package bundler

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/exp/maps"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/bazel"
	"code-intelligence.com/cifuzz/internal/build/cmake"
	"code-intelligence.com/cifuzz/internal/build/other"
	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

type configureVariant struct {
	Sanitizers []string
}

// System library dependencies that are so common that we shouldn't emit a warning for them - they will be contained in
// any reasonable Docker image.
var wellKnownSystemLibraries = map[string][]*regexp.Regexp{
	"linux": {
		versionedLibraryRegexp("ld-linux-x86-64.so"),
		versionedLibraryRegexp("libc.so"),
		versionedLibraryRegexp("libgcc_s.so"),
		versionedLibraryRegexp("libm.so"),
		versionedLibraryRegexp("libstdc++.so"),
	},
}

func versionedLibraryRegexp(unversionedBasename string) *regexp.Regexp {
	return regexp.MustCompile(".*/" + regexp.QuoteMeta(unversionedBasename) + "[.0-9]*")
}

type libfuzzerBundler struct {
	opts          *Opts
	archiveWriter archive.ArchiveWriter
}

func newLibfuzzerBundler(opts *Opts, archiveWriter archive.ArchiveWriter) *libfuzzerBundler {
	if opts.BuildStderr == nil {
		opts.BuildStderr = os.Stderr
	}
	if opts.BuildStdout == nil {
		opts.BuildStdout = os.Stdout
	}
	return &libfuzzerBundler{opts, archiveWriter}
}

func (b *libfuzzerBundler) bundle() ([]*archive.Fuzzer, error) {
	err := b.checkDependencies()
	if err != nil {
		return nil, err
	}

	buildResults, err := b.buildAllVariants()
	if err != nil {
		return nil, err
	}

	log.Info("Creating bundle...")

	// Add all fuzz test artifacts to the archive. There will be one "Fuzzer" metadata object for each pair of fuzz test
	// and Builder instance.
	var fuzzers []*archive.Fuzzer
	deduplicatedSystemDeps := make(map[string]struct{})
	for _, buildResult := range buildResults {
		fuzzTestFuzzers, systemDeps, err := b.assembleArtifacts(buildResult)
		if err != nil {
			return nil, err
		}
		fuzzers = append(fuzzers, fuzzTestFuzzers...)
		for _, systemDep := range systemDeps {
			deduplicatedSystemDeps[systemDep] = struct{}{}
		}
	}

	systemDeps := maps.Keys(deduplicatedSystemDeps)
	sort.Strings(systemDeps)
	if len(systemDeps) != 0 {
		log.Warnf(`The following system libraries are not part of the artifact and have to be provided by the Docker image %q:
      %s`, b.opts.DockerImage, strings.Join(systemDeps, "\n  "))
	}

	return fuzzers, nil
}

func (b *libfuzzerBundler) buildAllVariants() ([]*build.CBuildResult, error) {
	fuzzingVariant := configureVariant{
		// TODO: Do not hardcode these values.
		Sanitizers: []string{"address"},
	}
	// UBSan is not supported by MSVb.
	// TODO: Not needed anymore when sanitizers are configurable,
	//       then we do want to fail if the user explicitly asked for
	//       UBSan.
	if runtime.GOOS != "windows" {
		fuzzingVariant.Sanitizers = append(fuzzingVariant.Sanitizers, "undefined")
	}
	configureVariants := []configureVariant{fuzzingVariant}

	// Coverage builds are not supported by MSVb.
	if runtime.GOOS != "windows" {
		coverageVariant := configureVariant{
			Sanitizers: []string{"coverage"},
		}
		configureVariants = append(configureVariants, coverageVariant)
	}

	switch b.opts.BuildSystem {
	case config.BuildSystemBazel:
		return b.buildAllVariantsBazel(configureVariants)
	case config.BuildSystemCMake:
		return b.buildAllVariantsCMake(configureVariants)
	case config.BuildSystemOther:
		return b.buildAllVariantsOther(configureVariants)
	default:
		// We panic here instead of returning an error because it's a
		// programming error if this function was called with an
		// unsupported build system, that case should have been handled
		// in the Opts.Validate function.
		panic(fmt.Sprintf("Unsupported build system: %v", b.opts.BuildSystem))
	}
}

func (b *libfuzzerBundler) buildAllVariantsBazel(configureVariants []configureVariant) ([]*build.CBuildResult, error) {
	var allResults []*build.CBuildResult
	for _, variant := range configureVariants {
		builder, err := bazel.NewBuilder(&bazel.BuilderOptions{
			ProjectDir: b.opts.ProjectDir,
			Args:       b.opts.BuildSystemArgs,
			NumJobs:    b.opts.NumBuildJobs,
			Stdout:     b.opts.BuildStdout,
			Stderr:     b.opts.BuildStderr,
			TempDir:    b.opts.tempDir,
			Verbose:    viper.GetBool("verbose"),
		})
		if err != nil {
			return nil, err
		}

		b.printBuildingMsg(variant)

		if len(b.opts.FuzzTests) == 0 {
			// We panic here instead of returning an error because it's a
			// programming error if this function was called without any
			// fuzz tests, that case should have been handled in the
			// Opts.Validate function.
			panic("No fuzz tests specified")
		}

		results, err := builder.BuildForBundle(variant.Sanitizers, b.opts.FuzzTests)
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

func (b *libfuzzerBundler) buildAllVariantsCMake(configureVariants []configureVariant) ([]*build.CBuildResult, error) {
	var allResults []*build.CBuildResult
	for _, variant := range configureVariants {
		builder, err := cmake.NewBuilder(&cmake.BuilderOptions{
			ProjectDir: b.opts.ProjectDir,
			Args:       b.opts.BuildSystemArgs,
			Sanitizers: variant.Sanitizers,
			Parallel: cmake.ParallelOptions{
				Enabled: viper.IsSet("build-jobs"),
				NumJobs: b.opts.NumBuildJobs,
			},
			Stdout:          b.opts.BuildStdout,
			Stderr:          b.opts.BuildStderr,
			FindRuntimeDeps: true,
		})
		if err != nil {
			return nil, err
		}

		b.printBuildingMsg(variant)

		err = builder.Configure()
		if err != nil {
			return nil, err
		}

		var fuzzTests []string
		if len(b.opts.FuzzTests) == 0 {
			fuzzTests, err = builder.ListFuzzTests()
			if err != nil {
				return nil, err
			}
		} else {
			fuzzTests = b.opts.FuzzTests
		}

		// The fuzz tests passed to builder.Build must not contain
		// duplicates, which is ensured by builder.ListFuzzTests()
		// and the Opts.Validate() function.
		results, err := builder.Build(fuzzTests)
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

func (b *libfuzzerBundler) printBuildingMsg(variant configureVariant) {
	var typeDisplayString string
	if isCoverageBuild(variant.Sanitizers) {
		typeDisplayString = "coverage"
	} else {
		typeDisplayString = "fuzzing"
	}

	log.Infof("Building for %s...", typeDisplayString)
}

func (b *libfuzzerBundler) buildAllVariantsOther(configureVariants []configureVariant) ([]*build.CBuildResult, error) {
	if len(b.opts.BuildSystemArgs) > 0 {
		log.Warnf("Passing additional arguments is not supported for build system type \"other\".\n"+
			"These arguments are ignored: %s", strings.Join(b.opts.BuildSystemArgs, " "))
	}

	var results []*build.CBuildResult
	for _, variant := range configureVariants {
		builder, err := other.NewBuilder(&other.BuilderOptions{
			ProjectDir:   b.opts.ProjectDir,
			BuildCommand: b.opts.BuildCommand,
			CleanCommand: b.opts.CleanCommand,
			Sanitizers:   variant.Sanitizers,
			Stdout:       b.opts.BuildStdout,
			Stderr:       b.opts.BuildStderr,
		})
		if err != nil {
			return nil, err
		}

		b.printBuildingMsg(variant)

		if len(b.opts.FuzzTests) == 0 {
			// We panic here instead of returning an error because it's a
			// programming error if this function was called without any
			// fuzz tests, that case should have been handled in the
			// Opts.Validate function.
			panic("No fuzz tests specified")
		}

		if err := builder.Clean(); err != nil {
			return nil, err
		}

		for _, fuzzTest := range b.opts.FuzzTests {
			result, err := builder.Build(fuzzTest)
			if err != nil {
				return nil, err
			}

			// To avoid that subsequent builds overwrite the artifacts
			// from this build, we copy them to a temporary directory
			// and adjust the paths in the build.CBuildResult struct
			tempDir := filepath.Join(b.opts.tempDir, fuzzTestPrefix(result))
			err = b.copyArtifactsToTempdir(result, tempDir)
			if err != nil {
				return nil, err
			}

			results = append(results, result)
		}
	}

	return results, nil
}

func (b *libfuzzerBundler) copyArtifactsToTempdir(buildResult *build.CBuildResult, tempDir string) error {
	fuzzTestExecutableAbsPath := buildResult.Executable
	isBelow, err := fileutil.IsBelow(fuzzTestExecutableAbsPath, buildResult.BuildDir)
	if err != nil {
		return err
	}
	if isBelow {
		relPath, err := filepath.Rel(buildResult.BuildDir, fuzzTestExecutableAbsPath)
		if err != nil {
			return errors.WithStack(err)
		}
		newExecutablePath := filepath.Join(tempDir, relPath)
		err = copy.Copy(buildResult.Executable, newExecutablePath)
		if err != nil {
			return errors.WithStack(err)
		}
		buildResult.Executable = newExecutablePath
	}

	// Try to copy the regular files first before copying the corresponding symlinks.
	// Failing to do so results in errors that target of the symlink does not exist
	// in the temp directory.
	sort.Slice(buildResult.RuntimeDeps, func(i, j int) bool {
		return !fileutil.IsSymlink(buildResult.RuntimeDeps[i])
	})

	for i, dep := range buildResult.RuntimeDeps {
		isBelow, err = fileutil.IsBelow(dep, buildResult.BuildDir)
		if err != nil {
			return err
		}
		var topDir string
		if isBelow {
			topDir = buildResult.BuildDir
		} else {
			topDir = "/"
		}

		relPath, err := filepath.Rel(topDir, dep)
		if err != nil {
			return errors.WithStack(err)
		}
		newDepPath := filepath.Join(tempDir, relPath)

		// When dealing with symlinks, resolve the path so that we copy the actual file
		// to the temporary directory. This ensures that the dynamic dependencies resolved
		// by ldd and added into the bundle are valid files.
		resolvedPath, err := filepath.EvalSymlinks(dep)
		if err != nil {
			return errors.WithStack(err)
		}
		err = copy.Copy(resolvedPath, newDepPath)
		if err != nil {
			return errors.WithStack(err)
		}

		buildResult.RuntimeDeps[i] = newDepPath
	}
	buildResult.BuildDir = tempDir

	return nil
}

func (b *libfuzzerBundler) checkDependencies() error {
	var deps []dependencies.Key
	switch b.opts.BuildSystem {
	case config.BuildSystemCMake:
		deps = []dependencies.Key{dependencies.Clang, dependencies.CMake}
	case config.BuildSystemOther:
		deps = []dependencies.Key{dependencies.Clang}
	}
	err := dependencies.Check(deps, b.opts.ProjectDir)
	if err != nil {
		return err
	}
	return nil
}

//nolint:nonamedreturns
func (b *libfuzzerBundler) assembleArtifacts(buildResult *build.CBuildResult) (
	fuzzers []*archive.Fuzzer,
	systemDeps []string,
	err error,
) {
	log.Debugf("Assembling artifacts for %s", buildResult.Executable)

	fuzzTestExecutableAbsPath := buildResult.Executable

	// Add all build artifacts under a subdirectory of the fuzz test base path so that these files don't clash with
	// seeds and dictionaries.
	buildArtifactsPrefix := filepath.Join(fuzzTestPrefix(buildResult), "bin")

	// Add the fuzz test executable.
	ok, err := fileutil.IsBelow(fuzzTestExecutableAbsPath, buildResult.BuildDir)
	if err != nil {
		return
	}
	if !ok {
		err = errors.Errorf("fuzz test executable (%s) is not below build directory (%s)", fuzzTestExecutableAbsPath, buildResult.BuildDir)
		return
	}
	fuzzTestExecutableRelPath, err := filepath.Rel(buildResult.BuildDir, fuzzTestExecutableAbsPath)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	fuzzTestArchivePath := filepath.Join(buildArtifactsPrefix, fuzzTestExecutableRelPath)
	err = b.archiveWriter.WriteFile(fuzzTestArchivePath, fuzzTestExecutableAbsPath)
	if err != nil {
		return
	}

	// On macOS, debug information is collected in a separate .dSYM file. We bundle it in to get source locations
	// resolved in stack traces.
	fuzzTestDsymAbsPath := fuzzTestExecutableAbsPath + ".dSYM"
	dsymExists, err := fileutil.Exists(fuzzTestDsymAbsPath)
	if err != nil {
		err = errors.WithStack(err)
		return
	}
	if dsymExists {
		fuzzTestDsymArchivePath := fuzzTestArchivePath + ".dSYM"
		err = b.archiveWriter.WriteDir(fuzzTestDsymArchivePath, fuzzTestDsymAbsPath)
		if err != nil {
			return
		}
	}

	var libraryPaths []string
	// Add the runtime dependencies of the fuzz test executable.
	externalLibrariesPrefix := ""
depsLoop:
	for _, dep := range buildResult.RuntimeDeps {
		log.Debugf("Adding runtime dependency %s", dep)
		var isBelowBuildDir bool
		isBelowBuildDir, err = fileutil.IsBelow(dep, buildResult.BuildDir)
		if err != nil {
			return
		}
		if isBelowBuildDir {
			var buildDirRelPath string
			buildDirRelPath, err = filepath.Rel(buildResult.BuildDir, dep)
			if err != nil {
				err = errors.WithStack(err)
				return
			}

			if b.opts.BuildSystem == config.BuildSystemOther {
				libraryPath := filepath.Join(buildArtifactsPrefix, filepath.Dir(buildDirRelPath))
				if !sliceutil.Contains(libraryPaths, libraryPath) {
					libraryPaths = append(libraryPaths, libraryPath)
				}
			}

			var hash string
			hash, err = sha256sum(dep)
			if err != nil {
				return
			}
			casPath := filepath.Join("cas", hash[:2], hash[2:], filepath.Base(dep))
			if !b.archiveWriter.HasFileEntry(casPath) {
				err = b.archiveWriter.WriteFile(casPath, dep)
				if err != nil {
					return
				}
			}
			err = b.archiveWriter.WriteHardLink(casPath, filepath.Join(buildArtifactsPrefix, buildDirRelPath))
			if err != nil {
				return
			}
			continue
		}

		// The runtime dependency is not built as part of the current project. It will be of one of the following types:
		// 1. A standard system library that is available in all reasonable Docker images.
		// 2. A more uncommon system library that may require additional packages to be installed (e.g. X11), but still
		//    lives in a standard system library directory (e.g. /usr/lib). Such dependencies are expected to be
		//    provided by the Docker image used as the run environment.
		// 3. Any other external dependency (e.g. a CMake target imported from another CMake project with a separate
		//    build directory). These are not expected to be part of the Docker image and thus added to the archive
		//    in a special directory that is added to the library search path at runtime.

		// 1. is handled by ignoring these runtime dependencies.
		for _, wellKnownSystemLibrary := range wellKnownSystemLibraries[runtime.GOOS] {
			if wellKnownSystemLibrary.MatchString(dep) {
				log.Debugf("Runtime dependency %s is a standard system library and will not be added", dep)
				continue depsLoop
			}
		}

		// 2. is handled by returning a list of these libraries that is shown to the user as a warning about the
		// required contents of the Docker image specified as the run environment.
		if fileutil.IsSystemLibrary(dep) {
			systemDeps = append(systemDeps, dep)
			log.Debugf("Runtime dependency %s is a standard system library and will not be added", dep)
			continue depsLoop
		}

		// 3. is handled by staging the dependency in a special external library directory in the archive that is added
		// to the library search path in the run environment.
		// Note: Since all libraries are placed in a single directory, we have to ensure that basenames of external
		// libraries are unique. If they aren't, we report a conflict.
		externalLibrariesPrefix = filepath.Join(fuzzTestPrefix(buildResult), "external_libs")
		archivePath := filepath.Join(externalLibrariesPrefix, filepath.Base(dep))
		if b.archiveWriter.HasFileEntry(archivePath) {
			err = errors.Errorf(
				"fuzz test %q has conflicting runtime dependencies: %s and %s",
				buildResult.Name,
				dep,
				b.archiveWriter.GetSourcePath(archivePath),
			)
			return
		}
		err = b.archiveWriter.WriteFile(archivePath, dep)
		if err != nil {
			return
		}
	}

	if b.opts.Dictionary == "" {
		var exists bool
		exists, err = fileutil.Exists(buildResult.Dictionary)
		if err != nil {
			return
		}
		if exists {
			b.opts.Dictionary = buildResult.Dictionary
		}
	}
	// Add dictionary to archive
	var archiveDict string
	if b.opts.Dictionary != "" {
		log.Debugf("Adding dictionary %s", b.opts.Dictionary)
		archiveDict = filepath.Join(fuzzTestPrefix(buildResult), "dict")
		err = b.archiveWriter.WriteFile(archiveDict, b.opts.Dictionary)
		if err != nil {
			return
		}
	}

	// Add seeds from user-specified seed corpus dirs (if any) and the
	// default seed corpus (if it exists) to the seeds directory in the
	// archive
	seedCorpusDirs := b.opts.SeedCorpusDirs
	exists, err := fileutil.Exists(buildResult.SeedCorpus)
	if err != nil {
		return
	}
	if exists {
		log.Debugf("Adding user-provided seeds to seed corpus from %s", seedCorpusDirs)
		seedCorpusDirs = append([]string{buildResult.SeedCorpus}, seedCorpusDirs...)
	}
	var archiveSeedsDir string
	if len(seedCorpusDirs) > 0 {
		archiveSeedsDir = filepath.Join(fuzzTestPrefix(buildResult), "seeds")

		err = prepareSeeds(seedCorpusDirs, archiveSeedsDir, b.archiveWriter)
		if err != nil {
			return
		}
	}

	// Set NO_CIFUZZ=1 to avoid that remotely executed fuzz tests try
	// to start cifuzz
	env, err := envutil.Setenv(b.opts.Env, "NO_CIFUZZ", "1")
	if err != nil {
		return
	}

	baseFuzzerInfo := archive.Fuzzer{
		Target:     buildResult.Name,
		Path:       fuzzTestArchivePath,
		ProjectDir: buildResult.ProjectDir,
		Dictionary: archiveDict,
		Seeds:      archiveSeedsDir,
		EngineOptions: archive.EngineOptions{
			Env:   env,
			Flags: b.opts.EngineArgs,
		},
		MaxRunTime: uint(b.opts.Timeout.Seconds()),
	}

	if externalLibrariesPrefix != "" {
		libraryPaths = append(libraryPaths, externalLibrariesPrefix)
	}
	baseFuzzerInfo.LibraryPaths = libraryPaths

	if isCoverageBuild(buildResult.Sanitizers) {
		fuzzer := baseFuzzerInfo
		fuzzer.Engine = "LLVM_COV"
		// We use libFuzzer's crash-resistant merge mode. The first positional argument has to be an empty directory,
		// for which we use the working directory (empty at the beginning of a job as we include an empty work_dir in
		// the bundle). The second positional argument is the corpus directory passed in by the backend.
		// Since most libFuzzer options are not useful or potentially disruptive for coverage runs, we do not include
		// flags passed in via `--engine_args`.
		fuzzer.EngineOptions.Flags = []string{"-merge=1", "."}
		fuzzers = []*archive.Fuzzer{&fuzzer}
		// Coverage builds are separate from sanitizer builds, so we don't have any other fuzzers to add.
		return
	}

	for _, sanitizer := range buildResult.Sanitizers {
		if sanitizer == "undefined" {
			// The artifact archive spec does not support UBSan as a standalone sanitizer.
			continue
		}
		fuzzer := baseFuzzerInfo
		fuzzer.Engine = "LIBFUZZER"
		fuzzer.Sanitizer = strings.ToUpper(sanitizer)
		fuzzers = append(fuzzers, &fuzzer)
	}

	return
}

// fuzzTestPrefix returns the path in the resulting artifact archive under which fuzz test specific files should be
// added.
func fuzzTestPrefix(buildResult *build.CBuildResult) string {
	sanitizerSegment := strings.Join(buildResult.Sanitizers, "+")
	if sanitizerSegment == "" {
		sanitizerSegment = "none"
	}
	engine := "libfuzzer"
	if isCoverageBuild(buildResult.Sanitizers) {
		// The backend currently only passes the corpus directory (rather than the files contained in it) as
		// an argument to the coverage binary if it finds the substring "replayer/coverage" in the fuzz test archive
		// path.
		// FIXME: Remove this workaround as soon as the artifact spec provides a way to specify compatibility with
		//  directory arguments.
		engine = "replayer"
	}
	return filepath.Join(engine, sanitizerSegment, buildResult.Name)
}

func isCoverageBuild(sanitizers []string) bool {
	return len(sanitizers) == 1 && sanitizers[0] == "coverage"
}

func sha256sum(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer f.Close()

	h := sha256.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
