package bazel

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/archiveutil"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type BuilderOptions struct {
	ProjectDir string
	Args       []string
	NumJobs    uint
	Stdout     io.Writer
	Stderr     io.Writer
	TempDir    string
	Verbose    bool
}

func (opts *BuilderOptions) Validate() error {
	// Check that the project dir is set
	if opts.ProjectDir == "" {
		return errors.New("ProjectDir is not set")
	}

	// Check that the project dir exists and can be accessed
	_, err := os.Stat(opts.ProjectDir)
	if err != nil {
		return errors.WithStack(err)
	}

	// Check that the TempDir is set. This is not set by the user, so
	// we panic if it's not set
	if opts.TempDir == "" {
		panic("TempDir is not set")
	}

	return nil
}

type Builder struct {
	*BuilderOptions
}

func NewBuilder(opts *BuilderOptions) (*Builder, error) {
	err := opts.Validate()
	if err != nil {
		return nil, err
	}

	err = checkCIFuzzBazelRepoCommit()
	if err != nil {
		return nil, err
	}

	err = checkRulesFuzzingVersion()
	if err != nil {
		return nil, err
	}

	b := &Builder{BuilderOptions: opts}
	return b, nil
}

// BuildForRun builds the specified fuzz tests with bazel. It expects
// labels of targets of the cc_fuzz_test rule provided by rules_fuzzing:
// https://github.com/bazelbuild/rules_fuzzing/blob/master/docs/cc-fuzzing-rules.md#cc_fuzz_test
//
// TODO: Unfortunately, the cc_fuzz_test rule currently doesn't
// support combining sanitizers, so we can't build with both ASan
// and UBSan. Therefore, we only build with ASan and plan to upstream
// support for combining sanitizers.
func (b *Builder) BuildForRun(fuzzTests []string) ([]*build.BuildResult, error) {
	var err error

	var binLabels []string
	for i := range fuzzTests {
		// The cc_fuzz_test rule defines multiple bazel targets: If the
		// name is "foo", it defines the targets "foo", "foo_bin", and
		// others. We need to run the "foo_bin" target but want to
		// allow users to specify either "foo" or "foo_bin", so we check
		// if the fuzz test name  appended with "_bin" is a valid target
		// and use that in that case
		cmd := exec.Command("bazel", "query", fuzzTests[i]+"_bin")
		err := cmd.Run()
		if err == nil {
			binLabels = append(binLabels, fuzzTests[i]+"_bin")
		} else {
			fuzzTests[i] = strings.TrimSuffix(fuzzTests[i], "_bin")
			binLabels = append(binLabels, fuzzTests[i]+"_bin")
		}
	}

	// The BuildDir field of the build results is expected to be a
	// parent directory of all the artifacts, so that a single minijail
	// binding allows access to all artifacts in the sandbox.
	// When building via bazel, the "output_base" directory contains
	// all artifacts, so we use that as the BuildDir.
	cmd := exec.Command("bazel", "info", "output_base")
	out, err := cmd.Output()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	buildDir := strings.TrimSpace(string(out))
	fuzzScript := filepath.Join(b.TempDir, "fuzz.sh")

	// To avoid part of the loading and/or analysis phase to rerun, we
	// use the same flags for all bazel commands (except for those which
	// are not supported by all bazel commands we use).
	buildEnv, err := build.CommonBuildEnv()
	if err != nil {
		return nil, err
	}
	commonFlags := []string{
		"--repo_env=CC=" + envutil.Getenv(buildEnv, "CC"),
		"--repo_env=CXX=" + envutil.Getenv(buildEnv, "CXX"),
		// Don't use the LLVM from Xcode
		"--repo_env=BAZEL_USE_CPP_ONLY_TOOLCHAIN=1",
	}
	if b.NumJobs != 0 {
		commonFlags = append(commonFlags, "--jobs", fmt.Sprint(b.NumJobs))
	}

	// Flags which should only be used for bazel run because they are
	// not supported by the other bazel commands we use
	runFlags := []string{
		// Build with debug symbols
		"--copt", "-g",
		// Tell bazel to do an optimized build, which includes debug
		// symbols (in contrast to the default "fastbuild" compilation
		// mode which strips debug symbols).
		"--compilation_mode=opt",
		// Do optimizations which don't harm debugging
		"--copt", "-Og",
		// Enable asserts (disabled by --compilation_mode=opt).
		"--copt", "-UNDEBUG",
		// Disable source fortification, which is currently not supported
		// in combination with ASan, see https://github.com/google/sanitizers/issues/247
		"--copt", "-U_FORTIFY_SOURCE",
		// Build with libFuzzer
		"--@rules_fuzzing//fuzzing:cc_engine=@rules_fuzzing//fuzzing/engines:libfuzzer",
		"--@rules_fuzzing//fuzzing:cc_engine_instrumentation=libfuzzer",
		// Build with ASan and UBSan instrumentation
		"--@rules_fuzzing//fuzzing:cc_engine_sanitizer=asan-ubsan",
		// Link in our additional libFuzzer logic that dumps inputs for non-fatal crashes.
		"--@cifuzz//:__internal_has_libfuzzer",
		"--verbose_failures",
		"--script_path=" + fuzzScript,
	}

	if os.Getenv("BAZEL_SUBCOMMANDS") != "" {
		runFlags = append(runFlags, "--subcommands")
	}

	args := []string{"run"}
	args = append(args, commonFlags...)
	args = append(args, runFlags...)
	args = append(args, b.Args...)
	args = append(args, binLabels...)

	cmd = exec.Command("bazel", args...)
	cmd.Stdout = b.Stdout
	cmd.Stderr = b.Stderr
	if err != nil {
		return nil, err
	}
	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	// Assemble the build results
	var results []*build.BuildResult

	for _, fuzzTest := range fuzzTests {
		// Turn the fuzz test label into a valid path
		path, err := PathFromLabel(fuzzTest, commonFlags)
		if err != nil {
			return nil, err
		}
		seedCorpus := filepath.Join(b.ProjectDir, path+"_inputs")
		generatedCorpusBasename := "." + filepath.Base(path) + "_cifuzz_corpus"
		generatedCorpus := filepath.Join(b.ProjectDir, filepath.Dir(path), generatedCorpusBasename)

		result := &build.BuildResult{
			Executable:      fuzzScript,
			GeneratedCorpus: generatedCorpus,
			SeedCorpus:      seedCorpus,
			BuildDir:        buildDir,
		}
		results = append(results, result)
	}

	return results, nil
}

func (b *Builder) BuildForBundle(sanitizers []string, fuzzTests []string) ([]*build.CBuildResult, error) {
	var err error

	env, err := build.CommonBuildEnv()
	if err != nil {
		return nil, err
	}

	env, err = b.setLibFuzzerEnv(env)
	if err != nil {
		return nil, err
	}

	// To avoid part of the loading and/or analysis phase to rerun, we
	// use the same flags for all bazel commands (except for those which
	// are not supported by all bazel commands we use).
	commonFlags := []string{
		"--repo_env=CC=" + envutil.Getenv(env, "CC"),
		"--repo_env=CXX=" + envutil.Getenv(env, "CXX"),
		"--repo_env=FUZZING_CFLAGS=" + envutil.Getenv(env, "FUZZING_CFLAGS"),
		"--repo_env=FUZZING_CXXFLAGS=" + envutil.Getenv(env, "FUZZING_CXXFLAGS"),
		"--repo_env=LIB_FUZZING_ENGINE=" + envutil.Getenv(env, "LIB_FUZZING_ENGINE"),
		// Don't use the LLVM from Xcode
		"--repo_env=BAZEL_USE_CPP_ONLY_TOOLCHAIN=1",
		// rules_fuzzing only links in the UBSan C++ runtime when the
		// sanitizer is set to "undefined"
		"--repo_env=SANITIZER=undefined",
	}
	if b.NumJobs != 0 {
		commonFlags = append(commonFlags, "--jobs", fmt.Sprint(b.NumJobs))
	}

	// Flags which should only be used for bazel build
	buildAndCQueryFlags := []string{
		// Tell bazel to do an optimized build, which includes debug
		// symbols (in contrast to the default "fastbuild" compilation
		// mode which strips debug symbols).
		"--compilation_mode=opt",
		"--@rules_fuzzing//fuzzing:cc_engine=@rules_fuzzing_oss_fuzz//:oss_fuzz_engine",
		"--verbose_failures",
	}

	if os.Getenv("BAZEL_SUBCOMMANDS") != "" {
		buildAndCQueryFlags = append(buildAndCQueryFlags, "--subcommands")
	}

	// Add sanitizer-specific flags
	if len(sanitizers) == 1 && sanitizers[0] == "coverage" {
		llvmCov, err := runfiles.Finder.LLVMCovPath()
		if err != nil {
			return nil, err
		}
		llvmProfData, err := runfiles.Finder.LLVMProfDataPath()
		if err != nil {
			return nil, err
		}
		commonFlags = append(commonFlags,
			"--repo_env=BAZEL_USE_LLVM_NATIVE_COVERAGE=1",
			"--repo_env=GCOV="+llvmProfData,
			"--repo_env=BAZEL_LLVM_COV="+llvmCov,
		)
		buildAndCQueryFlags = append(buildAndCQueryFlags,
			"--instrument_test_targets",
			"--experimental_use_llvm_covmap",
			"--experimental_generate_llvm_lcov",
			"--collect_code_coverage",
		)
	} else {
		buildAndCQueryFlags = append(buildAndCQueryFlags,
			"--@rules_fuzzing//fuzzing:cc_engine_instrumentation=oss-fuzz")
		for _, sanitizer := range sanitizers {
			switch sanitizer {
			case "address", "undefined":
				// ASan and UBSan are already enabled above by the call
				// to b.setLibFuzzerEnv, which sets the respective flags
				// via the FUZZING_CFLAGS environment variable. These
				// variables are then picked up by the OSS-Fuzz engine
				// instrumentation.
			default:
				panic(fmt.Sprintf("Invalid sanitizer: %q", sanitizer))
			}
		}
	}

	args := []string{"build"}
	args = append(args, commonFlags...)
	args = append(args, b.Args...)
	args = append(args, buildAndCQueryFlags...)

	// We have to build the "*_oss_fuzz" target defined by the
	// cc_fuzz_test rule
	var labels []string
	for _, fuzzTestLabel := range fuzzTests {
		labels = append(labels, fuzzTestLabel+"_oss_fuzz")
	}
	args = append(args, labels...)

	cmd := exec.Command("bazel", args...)
	cmd.Stdout = b.Stdout
	cmd.Stderr = b.Stderr
	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	// Assemble the build results
	var results []*build.CBuildResult

	for _, fuzzTest := range fuzzTests {
		// Get the path to the archive created by the build
		args := []string{"cquery", "--output=starlark", "--starlark:expr=target.files.to_list()[0].path"}
		args = append(args, commonFlags...)
		args = append(args, buildAndCQueryFlags...)
		args = append(args, fuzzTest+"_oss_fuzz")
		cmd = exec.Command("bazel", args...)
		out, err := cmd.Output()
		if err != nil {
			return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
		}
		ossFuzzArchive := strings.TrimSpace(string(out))

		// Extract the archive
		extractedDir, err := os.MkdirTemp(b.TempDir, "extracted-")
		if err != nil {
			return nil, errors.WithStack(err)
		}
		err = archiveutil.UntarFile(ossFuzzArchive, extractedDir)
		if err != nil {
			return nil, errors.WithMessagef(err, "Failed to extract archive %s to %s", ossFuzzArchive, extractedDir)
		}

		path, err := PathFromLabel(fuzzTest, commonFlags)
		if err != nil {
			return nil, err
		}
		executable := filepath.Join(extractedDir, filepath.Base(path))

		// Extract the seed corpus
		ossFuzzSeedCorpus := executable + "_seed_corpus.zip"
		extractedCorpus := executable + "_seed_corpus"
		err = archiveutil.Unzip(ossFuzzSeedCorpus, extractedCorpus)
		if err != nil {
			return nil, err
		}

		// Find the runtime dependencies. The bundler will include them
		// in the bundle because below we set the BuildDir field of the
		// build.CBuildResult to extractedCorpus, which contains all the
		// runtime dependencies, causing the bundler to treat them all
		// as created by the build and therefore including them in the
		// bundle.
		var runtimeDeps []string
		runfilesDir := executable + ".runfiles"
		exists, err := fileutil.Exists(runfilesDir)
		if err != nil {
			return nil, err
		}
		if exists {
			err = filepath.WalkDir(runfilesDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return errors.WithStack(err)
				}
				if d.IsDir() {
					return nil
				}
				runtimeDeps = append(runtimeDeps, path)
				return nil
			})
			if err != nil {
				return nil, errors.WithMessagef(err, "Failed to walk through runfiles directory %s", runfilesDir)
			}
		}

		result := &build.CBuildResult{
			Name: path,
			// Bazel builds files with PWD=/proc/self/cwd
			ProjectDir: "/proc/self/cwd",
			Sanitizers: sanitizers,
			BuildResult: &build.BuildResult{
				Executable:  executable,
				SeedCorpus:  extractedCorpus,
				BuildDir:    extractedDir,
				RuntimeDeps: runtimeDeps,
			},
		}
		results = append(results, result)
	}

	return results, nil
}

func (b *Builder) setLibFuzzerEnv(env []string) ([]string, error) {
	var err error

	// Set FUZZING_CFLAGS and FUZZING_CXXFLAGS.
	cflags := build.LibFuzzerCFlags()
	env, err = envutil.Setenv(env, "FUZZING_CFLAGS", strings.Join(cflags, " "))
	if err != nil {
		return nil, err
	}
	env, err = envutil.Setenv(env, "FUZZING_CXXFLAGS", strings.Join(cflags, " "))
	if err != nil {
		return nil, err
	}

	// Set LIB_FUZZING_ENGINE, which is added as a linkopt to the fuzz
	// test itself.
	env, err = envutil.Setenv(env, "LIB_FUZZING_ENGINE", "-fsanitize=fuzzer")
	if err != nil {
		return nil, err
	}

	return env, nil
}

// PathFromLabel turns a bazel label into a valid path, which can for
// example be used to create the fuzz test's corpus directory.
// Flags which should be passed to the `bazel query` command can be
// passed via the flags argument (to avoid bazel discarding the analysis
// cache).
func PathFromLabel(label string, flags []string) (string, error) {
	// Get a canonical form of label via `bazel query`
	args := append([]string{"query"}, flags...)
	args = append(args, label)
	cmd := exec.Command("bazel", args...)
	log.Debugf("Command: %s", cmd.String())
	out, err := cmd.Output()
	if err != nil {
		return "", cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	canonicalLabel := strings.TrimSpace(string(out))

	// Transform the label into a valid path below the directory which
	// contains the BUILD file, which:
	// * Doesn't contain any leading '//'
	// * Has any ':' and '/' replaced with the path separator (':' is
	//   not allowed in filenames on Windows)
	res := strings.TrimPrefix(canonicalLabel, "//")
	res = strings.ReplaceAll(res, ":", "/")
	res = strings.ReplaceAll(res, "/", string(filepath.Separator))

	return res, nil
}

// Parses formatted bazel query --output=build output such as:
//
//	git_repository(
//	  name = "cifuzz",
//	  remote = "https://github.com/CodeIntelligenceTesting/cifuzz-bazel",
//	  commit = "ccb0bb7f27864626f668cca6d6e87776e6f87bd",
//	)
//
// For backwards compatibility, this regex also matches a branch that
// was used in cifuzz v0.9.0 and earlier. The branch will never be equal
// to a commit hash.
var cifuzzCommitRegex = regexp.MustCompile(`(?m)^\s*(?:commit|branch)\s*=\s*"([^"]*)"`)

var rulesFuzzingSHA256Regex = regexp.MustCompile(`(?m)^\s*sha256\s*=\s*"([^"]*)"`)

func checkCIFuzzBazelRepoCommit() error {
	cmd := exec.Command("bazel", "query", "--output=build", "//external:cifuzz")
	out, err := cmd.Output()
	if err != nil {
		// If the reason for the error is that the cifuzz repository is
		// missing, produce a more helpful error message.
		if strings.Contains(err.Error(), "target 'cifuzz' not declared in package") {
			return cmdutils.WrapExecError(errors.Errorf(`The "cifuzz" repository is not defined in the WORKSPACE file,
run 'cifuzz init' to see setup instructions.`), cmd)
		}
		return cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	matches := cifuzzCommitRegex.FindSubmatch(out)
	if matches == nil {
		return cmdutils.WrapExecError(errors.Errorf(
			`Failed to parse the definition of the "cifuzz" repository in the WORKSPACE file,
run 'cifuzz init' to see setup instructions.
bazel query output:
%s`, string(out)), cmd)
	}

	cifuzzRepoCommit := string(matches[1])
	if cifuzzRepoCommit != dependencies.CIFuzzBazelCommit {
		return cmdutils.WrapExecError(errors.Errorf(
			`Please update the commit specified for the "cifuzz" repository in the WORKSPACE file.
Required: %[1]s
Current : %[2]s`,
			fmt.Sprintf(`commit = %q`, dependencies.CIFuzzBazelCommit),
			strings.TrimSpace(string(matches[0]))), cmd)
	}

	return nil
}

func checkRulesFuzzingVersion() error {
	cmd := exec.Command("bazel", "query", "--output=build", "//external:rules_fuzzing")
	out, err := cmd.Output()
	if err != nil {
		// If the reason for the error is that the cifuzz repository is
		// missing, produce a more helpful error message.
		if strings.Contains(err.Error(), "target 'cifuzz' not declared in package") {
			return cmdutils.WrapExecError(errors.Errorf(
				`The "rules_fuzzing" repository is not defined in the WORKSPACE file,
run 'cifuzz init' to see setup instructions.`),
				cmd)
		}
		return cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	matches := rulesFuzzingSHA256Regex.FindSubmatch(out)
	if len(matches) == 0 || string(matches[1]) != dependencies.RulesFuzzingSHA256 {
		return cmdutils.WrapExecError(errors.Errorf(
			`Please update the definition of the "rules_fuzzing" repository and its
dependencies in the WORKSPACE file to:

    %s

		`, dependencies.RulesFuzzingWORKSPACEContent), cmd)
	}

	return nil
}
