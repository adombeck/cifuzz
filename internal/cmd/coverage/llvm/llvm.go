package llvm

import (
	"bytes"
	"context"
	"debug/macho"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/cmake"
	"code-intelligence.com/cifuzz/internal/build/other"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/binary"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/minijail"
	"code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	fuzzer_runner "code-intelligence.com/cifuzz/pkg/runner"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type CoverageGenerator struct {
	OutputFormat    string
	OutputPath      string
	BuildSystem     string
	BuildCommand    string
	BuildSystemArgs []string
	CleanCommand    string
	NumBuildJobs    uint
	SeedCorpusDirs  []string
	UseSandbox      bool
	FuzzTest        string
	ProjectDir      string
	Stderr          io.Writer
	BuildStdout     io.Writer
	BuildStderr     io.Writer

	coverageBinary string
	libraryDirs    []string
	runtimeDeps    []string
	tmpDir         string
	outputDir      string
	runfilesFinder runfiles.RunfilesFinder
}

func (cov *CoverageGenerator) BuildFuzzTestForCoverage() error {
	// ensure a finder is set
	if cov.runfilesFinder == nil {
		cov.runfilesFinder = runfiles.Finder
	}

	var err error
	cov.tmpDir, err = os.MkdirTemp("", "llvm-coverage-")
	if err != nil {
		return errors.WithStack(err)
	}
	cov.outputDir = filepath.Join(cov.tmpDir, "output")
	err = os.Mkdir(cov.outputDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	err = cov.build()
	if err != nil {
		return err
	}

	return nil
}

func (cov *CoverageGenerator) GenerateCoverageReport() (string, error) {
	log.Infof("Running %s on corpus", pterm.Style{pterm.Reset, pterm.FgLightBlue}.Sprint(cov.FuzzTest))
	log.Debugf("Executable: %s", cov.coverageBinary)

	ctx := context.Background()
	defer fileutil.Cleanup(cov.tmpDir)

	err := cov.run(ctx)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && cov.UseSandbox {
			return "", cmdutils.WrapCouldBeSandboxError(err)
		}
		return "", err
	}

	reportPath, err := cov.report(ctx)
	if err != nil {
		return "", err
	}
	return reportPath, nil
}

func (cov *CoverageGenerator) GenerateCoverageReportInFuzzContainer(ctx context.Context, coverageBinary string,
	outputPath string, libraryDirs []string) error {

	log.Infof("Creating coverage report for %s", pterm.Style{pterm.Reset, pterm.FgLightBlue}.Sprint(coverageBinary))

	var err error
	cov.coverageBinary = coverageBinary
	cov.libraryDirs = libraryDirs

	// ensure a finder is set
	if cov.runfilesFinder == nil {
		cov.runfilesFinder = runfiles.Finder
	}

	cov.tmpDir, err = os.MkdirTemp("", "llvm-coverage-")
	if err != nil {
		return errors.WithStack(err)
	}
	cov.outputDir = filepath.Join(cov.tmpDir, "output")
	err = os.Mkdir(cov.outputDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	exists, err := fileutil.Exists("cas")
	if err != nil {
		return err
	}
	if exists {
		// Add all files in the "cas" directory to the runtime deps slice
		err = filepath.Walk("cas", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.WithStack(err)
			}
			if info.IsDir() {
				return nil
			}
			cov.runtimeDeps = append(cov.runtimeDeps, path)
			return nil
		})
		// filepath.Walk returns an error created by us so it already has a
		// stack trace and we don't want to add another one here
		// nolint: wrapcheck
		if err != nil {
			return err
		}
	}

	err = cov.run(ctx)
	if err != nil {
		return err
	}

	err = cov.indexRawProfile(ctx)
	if err != nil {
		return err
	}

	lcovReportSummary, err := cov.lcovReportSummary(ctx)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(outputPath), 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.WriteFile(outputPath, []byte(lcovReportSummary), 0o644)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (cov *CoverageGenerator) build() error {
	var buildResult *build.CBuildResult
	switch cov.BuildSystem {
	case config.BuildSystemCMake:
		builder, err := cmake.NewBuilder(&cmake.BuilderOptions{
			ProjectDir: cov.ProjectDir,
			Args:       cov.BuildSystemArgs,
			Sanitizers: []string{"coverage"},
			Parallel: cmake.ParallelOptions{
				Enabled: viper.IsSet("build-jobs"),
				NumJobs: uint(cov.NumBuildJobs),
			},
			Stdout: cov.BuildStdout,
			Stderr: cov.BuildStderr,
			// We want the runtime deps in the build result because we
			// pass them to the llvm-cov command.
			FindRuntimeDeps: true,
		})
		if err != nil {
			return err
		}
		err = builder.Configure()
		if err != nil {
			return err
		}
		buildResults, err := builder.Build([]string{cov.FuzzTest})
		if err != nil {
			return err
		}
		buildResult = buildResults[0]
	case config.BuildSystemOther:
		if runtime.GOOS == "windows" {
			return errors.New("CMake is the only supported build system on Windows")
		}
		builder, err := other.NewBuilder(&other.BuilderOptions{
			ProjectDir:     cov.ProjectDir,
			BuildCommand:   cov.BuildCommand,
			CleanCommand:   cov.CleanCommand,
			Sanitizers:     []string{"coverage"},
			RunfilesFinder: cov.runfilesFinder,
			Stdout:         cov.BuildStdout,
			Stderr:         cov.BuildStderr,
		})
		if err != nil {
			return err
		}

		if err := builder.Clean(); err != nil {
			return err
		}

		buildResult, err = builder.Build(cov.FuzzTest)
		if err != nil {
			return err
		}
	default:
		return errors.New("unknown build system")
	}

	cov.coverageBinary = buildResult.Executable
	cov.runtimeDeps = buildResult.RuntimeDeps

	// Use the seed corpus directory and generated corpus directory if
	// they exist.
	for _, path := range []string{buildResult.SeedCorpus, buildResult.GeneratedCorpus} {
		exists, err := fileutil.Exists(path)
		if err != nil {
			return err
		}
		if exists {
			cov.SeedCorpusDirs = append(cov.SeedCorpusDirs, path)
		}
	}

	return nil
}

func (cov *CoverageGenerator) run(ctx context.Context) error {
	var err error

	corpusDirs := cov.SeedCorpusDirs

	// Ensure that symlinks are resolved to be able to add minijail
	// bindings for the corpus dirs.
	for i, dir := range corpusDirs {
		corpusDirs[i], err = filepath.EvalSymlinks(dir)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	conModeSupport := binary.SupportsLlvmProfileContinuousMode(cov.coverageBinary)
	var env []string
	env, err = envutil.Setenv(env, "LLVM_PROFILE_FILE", cov.rawProfilePattern(conModeSupport))
	if err != nil {
		return err
	}
	env, err = envutil.Setenv(env, "NO_CIFUZZ", "1")
	if err != nil {
		return err
	}
	if len(cov.libraryDirs) > 0 {
		env, err = fuzzer_runner.SetLDLibraryPath(env, cov.libraryDirs)
		if err != nil {
			return err
		}
	}

	dirWithEmptyFile := filepath.Join(cov.outputDir, "empty-file-corpus")
	err = os.Mkdir(dirWithEmptyFile, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = fileutil.Touch(filepath.Join(dirWithEmptyFile, "empty_file"))
	if err != nil {
		return err
	}

	emptyDir := filepath.Join(cov.outputDir, "merge-target")
	err = os.Mkdir(emptyDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	artifactsDir := filepath.Join(cov.outputDir, "merge-artifacts")
	err = os.Mkdir(artifactsDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	// libFuzzer emits crashing inputs in merge mode, but these aren't useful as we only run on already known inputs.
	// Since there is no way to disable this behavior in libFuzzer, we instead emit artifacts into a dedicated temporary
	// directory that is thrown away after the coverage run.
	args := []string{"-artifact_prefix=" + artifactsDir + "/"}

	// libFuzzer's merge mode never runs the empty input, whereas regular fuzzing runs and the replayer always try the
	// empty input first. To achieve consistent behavior, manually run the empty input, ignoring any crashes. runFuzzer
	// always logs any error we encounter.
	// This line is responsible for empty inputs being skipped:
	// https://github.com/llvm/llvm-project/blob/c7c0ce7d9ebdc0a49313bc77e14d1e856794f2e0/compiler-rt/lib/fuzzer/FuzzerIO.cpp#L127
	_ = cov.runFuzzer(ctx, append(args, "-runs=0"), []string{dirWithEmptyFile}, env)

	// We use libFuzzer's crash-resistant merge mode to merge all corpus directories into an empty directory, which
	// makes libFuzzer go over all inputs in a subprocess that is restarted in case it crashes. With LLVM's continuous
	// mode (see rawProfilePattern) and since the LLVM coverage information is automatically appended to the existing
	// .profraw file, we collect complete coverage information even if the target crashes on an input in the corpus.
	return cov.runFuzzer(ctx, append(args, "-merge=1"), append([]string{emptyDir}, corpusDirs...), env)
}

func (cov *CoverageGenerator) runFuzzer(ctx context.Context, preCorpusArgs []string,
	corpusDirs []string, env []string) error {

	var err error
	args := []string{cov.coverageBinary}
	args = append(args, preCorpusArgs...)
	args = append(args, corpusDirs...)

	if cov.UseSandbox {
		bindings := []*minijail.Binding{
			// The fuzz target must be accessible
			{Source: cov.coverageBinary},
		}

		for _, dir := range corpusDirs {
			bindings = append(bindings, &minijail.Binding{Source: dir})
		}

		// Set up Minijail
		mj, err := minijail.NewMinijail(&minijail.Options{
			Args:      args,
			Bindings:  bindings,
			OutputDir: cov.outputDir,
		})
		if err != nil {
			return err
		}
		defer mj.Cleanup()

		// Use the command which runs the fuzz test via minijail
		args = mj.Args
	}

	cmd := executil.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env, err = envutil.Copy(os.Environ(), env)
	if err != nil {
		return err
	}

	errStream := &bytes.Buffer{}
	if viper.GetBool("verbose") {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else if cov.UseSandbox {
		cmd.Stderr = minijail.NewOutputFilter(errStream)
	} else {
		cmd.Stderr = errStream
	}

	log.Debugf("Command: %s", envutil.QuotedCommandWithEnv(cmd.Args, env))
	err = cmd.Run()
	if err != nil {
		// Add stderr output of the fuzzer to provide users with
		// the context of this error even without verbose mode.
		if !viper.GetBool("verbose") {
			err = fmt.Errorf("%w\n%s", err, errStream.String())
		}
		return cmdutils.WrapExecError(errors.WithStack(err), cmd.Cmd)
	}
	return err
}

func (cov *CoverageGenerator) report(ctx context.Context) (string, error) {
	err := cov.indexRawProfile(ctx)
	if err != nil {
		return "", err
	}

	lcovReportSummary, err := cov.lcovReportSummary(ctx)
	if err != nil {
		return "", err
	}
	reportReader := strings.NewReader(lcovReportSummary)
	summary, err := coverage.ParseLCOVReportIntoSummary(reportReader)
	if err != nil {
		return "", err
	}
	summary.PrintTable(cov.Stderr)

	reportPath := ""
	switch cov.OutputFormat {
	case "html":
		reportPath, err = cov.generateHTMLReport(ctx)
		if err != nil {
			return "", err
		}

	case "lcov":
		reportPath, err = cov.generateLcovReport(ctx)
		if err != nil {
			return "", err
		}
	}

	return reportPath, nil
}

func (cov *CoverageGenerator) indexRawProfile(ctx context.Context) error {
	rawProfileFiles, err := cov.rawProfileFiles()
	if err != nil {
		return err
	}
	if len(rawProfileFiles) == 0 {
		// The rawProfilePattern parameter only governs whether we add "%c",
		// which doesn't affect the actual raw profile location.
		return errors.Errorf("%s did not generate .profraw files at %s", cov.coverageBinary, cov.rawProfilePattern(false))
	}

	llvmProfData, err := cov.runfilesFinder.LLVMProfDataPath()
	if err != nil {
		return err
	}

	args := append([]string{"merge", "-sparse", "-o", cov.indexedProfilePath()}, rawProfileFiles...)
	cmd := exec.CommandContext(ctx, llvmProfData, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Debugf("Command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))
	err = cmd.Run()
	if err != nil {
		return cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	return nil
}

func (cov *CoverageGenerator) rawProfilePattern(supportsContinuousMode bool) string {
	// Use "%m" instead of a fixed path to support coverage of shared
	// libraries: Each executable or library generates its own profile
	// file, all of which we have to merge in the end. By using "%m",
	// the profile is written to a unique file for each executable and
	// shared library.
	// Use "%c", if supported, which expands out to nothing, to enable the
	// continuous mode in which the .profraw is mmaped and thus kept in sync with
	// the counters in the instrumented code even when it crashes.
	// https://clang.llvm.org/docs/SourceBasedCodeCoverage.html#running-the-instrumented-program
	basePattern := "%m.profraw"
	if supportsContinuousMode {
		basePattern = "%c" + basePattern
	}
	return filepath.Join(cov.outputDir, basePattern)
}

func (cov *CoverageGenerator) generateHTMLReport(ctx context.Context) (string, error) {
	args := []string{"export", "-format=lcov"}
	ignoreCIFuzzIncludesArgs, err := cov.getIgnoreCIFuzzIncludesArgs()
	if err != nil {
		return "", err
	}
	args = append(args, ignoreCIFuzzIncludesArgs...)
	report, err := cov.runLlvmCov(ctx, args)
	if err != nil {
		return "", err
	}
	// Write lcov report to temp dir
	reportDir, err := os.MkdirTemp("", "coverage-")
	if err != nil {
		return "", errors.WithStack(err)
	}
	lcovReport := filepath.Join(reportDir, "coverage.lcov")
	err = os.WriteFile(lcovReport, []byte(report), 0o644)
	if err != nil {
		return "", errors.WithStack(err)
	}

	if cov.OutputPath == "" {
		// If no output path is specified, we create the output in a
		// temporary directory.
		outputDir, err := os.MkdirTemp("", "coverage-")
		if err != nil {
			return "", errors.WithStack(err)
		}
		cov.OutputPath = filepath.Join(outputDir, cov.executableName())
	}

	// Create an HTML report via genhtml
	genHTML, err := runfiles.Finder.GenHTMLPath()
	if err != nil {
		return "", err
	}
	args = []string{"--output", cov.OutputPath, lcovReport}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// genHTML is a perl script, which has to be started like
		// "perl /path/to/genhtml args..." on Windows
		args = append([]string{genHTML}, args...)
		perl, err := runfiles.Finder.PerlPath()
		if err != nil {
			return "", err
		}
		cmd = exec.Command(perl, args...)
	} else {
		cmd = exec.Command(genHTML, args...)
	}

	cmd.Dir = cov.ProjectDir
	cmd.Stderr = os.Stderr
	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return cov.OutputPath, nil
}

func (cov *CoverageGenerator) runLlvmCov(ctx context.Context, args []string) (string, error) {
	llvmCov, err := cov.runfilesFinder.LLVMCovPath()
	if err != nil {
		return "", err
	}

	// Add all runtime dependencies of the fuzz test to the binaries
	// processed by llvm-cov to include them in the coverage report
	args = append(args, "-instr-profile="+cov.indexedProfilePath())
	args = append(args, cov.coverageBinary)
	if archArg, err := cov.archFlagIfNeeded(cov.coverageBinary); err != nil {
		return "", err
	} else if archArg != "" {
		args = append(args, archArg)
	}
	for _, path := range cov.runtimeDeps {
		args = append(args, "-object="+path)
		if archArg, err := cov.archFlagIfNeeded(path); err != nil {
			return "", err
		} else if archArg != "" {
			args = append(args, archArg)
		}
	}

	cmd := exec.CommandContext(ctx, llvmCov, args...)
	cmd.Stderr = os.Stderr
	log.Debugf("Command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))
	output, err := cmd.Output()
	if err != nil {
		return "", cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	return string(output), nil
}

func (cov *CoverageGenerator) generateLcovReport(ctx context.Context) (string, error) {
	args := []string{"export", "-format=lcov"}
	ignoreCIFuzzIncludesArgs, err := cov.getIgnoreCIFuzzIncludesArgs()
	if err != nil {
		return "", err
	}
	args = append(args, ignoreCIFuzzIncludesArgs...)
	report, err := cov.runLlvmCov(ctx, args)
	if err != nil {
		return "", err
	}

	outputPath := cov.OutputPath
	if cov.OutputPath == "" {
		// If no output path is specified, we create the output in the
		// current working directory. We don't create it in a temporary
		// directory like we do for HTML reports, because we can't open
		// the lcov report in a browser, so the command is only useful
		// if the lcov report is accessible after it was created.
		outputPath = cov.executableName() + ".coverage.lcov"
	}

	err = os.WriteFile(outputPath, []byte(report), 0o644)
	if err != nil {
		return "", errors.WithStack(err)
	}

	log.Debugf("Created lcov trace file: %s", outputPath)
	return outputPath, nil
}

func (cov *CoverageGenerator) lcovReportSummary(ctx context.Context) (string, error) {
	args := []string{"export", "-format=lcov", "-summary-only"}
	ignoreCIFuzzIncludesArgs, err := cov.getIgnoreCIFuzzIncludesArgs()
	if err != nil {
		return "", err
	}
	args = append(args, ignoreCIFuzzIncludesArgs...)
	output, err := cov.runLlvmCov(ctx, args)
	if err != nil {
		return "", err
	}

	return output, nil
}

func (cov *CoverageGenerator) getIgnoreCIFuzzIncludesArgs() ([]string, error) {
	cifuzzIncludePath, err := cov.runfilesFinder.CIFuzzIncludePath()
	if err != nil {
		return nil, err
	}
	return []string{"-ignore-filename-regex=" + regexp.QuoteMeta(cifuzzIncludePath) + "/.*"}, nil
}

func (cov *CoverageGenerator) rawProfileFiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(cov.outputDir, "*.profraw"))
	return files, errors.WithStack(err)
}

func (cov *CoverageGenerator) indexedProfilePath() string {
	return filepath.Join(cov.tmpDir, filepath.Base(cov.coverageBinary)+".profdata")
}

func (cov *CoverageGenerator) executableName() string {
	executable := cov.coverageBinary
	// Remove .exe file extension on Windows
	if runtime.GOOS == "windows" {
		executable = strings.TrimSuffix(executable, filepath.Ext(executable))
	}
	return filepath.Base(executable)
}

// Returns an llvm-cov -arch flag indicating the preferred architecture of the given object on macOS, where objects can
// be "universal", that is, contain versions for multiple architectures.
func (cov *CoverageGenerator) archFlagIfNeeded(object string) (string, error) {
	if runtime.GOOS != "darwin" {
		// Only macOS uses universal binaries that bundle multiple architectures.
		return "", nil
	}
	var cifuzzCPU macho.Cpu
	if runtime.GOARCH == "amd64" {
		cifuzzCPU = macho.CpuAmd64
	} else {
		cifuzzCPU = macho.CpuArm64
	}
	fatFile, fatErr := macho.OpenFat(object)
	if fatErr == nil {
		defer fatFile.Close()
		var fallbackCPU macho.Cpu
		for _, arch := range fatFile.Arches {
			// Give preference to the architecture matching that of the cifuzz binary.
			if arch.Cpu == cifuzzCPU {
				return cov.cpuToArchFlag(arch.Cpu)
			}
			if arch.Cpu == macho.CpuAmd64 || arch.Cpu == macho.CpuArm64 {
				fallbackCPU = arch.Cpu
			}
		}
		return cov.cpuToArchFlag(fallbackCPU)
	}
	file, err := macho.Open(object)
	if err == nil {
		defer file.Close()
		return cov.cpuToArchFlag(file.Cpu)
	}
	return "", errors.Errorf("failed to parse Mach-O file %q: %q (as universal binary), %q", object, fatErr, err)
}

func (cov *CoverageGenerator) cpuToArchFlag(cpu macho.Cpu) (string, error) {
	switch cpu {
	case macho.CpuArm64:
		return "-arch=arm64", nil
	case macho.CpuAmd64:
		return "-arch=x86_64", nil
	default:
		return "", errors.Errorf("unsupported architecture: %s", cpu.String())
	}
}
