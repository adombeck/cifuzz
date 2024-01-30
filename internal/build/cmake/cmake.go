package cmake

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/ldd"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

// The CMake configuration (also called "build type") to use for fuzzing runs.
// See enable_fuzz_testing in tools/cmake/modules/cifuzz-functions.cmake for the rationale for using this
// build type.
const cmakeBuildConfiguration = "RelWithDebInfo"

// System library dependencies, which should not be considered as runtime dependencies
var wellKnownSystemLibraries = map[string][]*regexp.Regexp{
	"windows": {
		regexp.MustCompile("^api-ms"),
		regexp.MustCompile("^ext-ms"),
	},
}

type ParallelOptions struct {
	Enabled bool
	NumJobs uint
}

type BuilderOptions struct {
	ProjectDir string
	Args       []string
	Sanitizers []string
	Parallel   ParallelOptions
	Stdout     io.Writer
	Stderr     io.Writer
	BuildOnly  bool

	FindRuntimeDeps bool
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
	return nil
}

type Builder struct {
	*BuilderOptions
	env []string
}

func NewBuilder(opts *BuilderOptions) (*Builder, error) {
	err := opts.Validate()
	if err != nil {
		return nil, err
	}

	b := &Builder{BuilderOptions: opts}

	// Ensure that the build directory exists.
	buildDir, err := b.BuildDir()
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(buildDir, 0755)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	b.env, err = build.CommonBuildEnv()
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Builder) Opts() *BuilderOptions {
	return b.BuilderOptions
}

func (b *Builder) BuildDir() (string, error) {
	// Note: Invoking CMake on the same build directory with different cache
	// variables is a no-op. For this reason, we have to encode all choices made
	// for the cache variables below in the path to the build directory.
	// Currently, this includes the fuzzing engine, the choice of sanitizers
	// and optional user arguments
	sanitizersSegment := strings.Join(b.Sanitizers, "+")
	if sanitizersSegment == "" {
		sanitizersSegment = "none"
	}

	buildDir := sanitizersSegment

	if len(b.Args) > 0 {
		// Add the hash of all user arguments to the build dir name in order to
		// create different build directories for different combinations of arguments
		hash := sha256.New()
		for _, arg := range b.Args {
			// Prepend the length of each argument in order to differentiate
			// between arguments like {"foo", "bar"} and {"foobar"}
			err := binary.Write(hash, binary.BigEndian, uint32(len(arg)))
			if err != nil {
				return "", errors.WithStack(err)
			}
			err = binary.Write(hash, binary.BigEndian, []byte(arg))
			if err != nil {
				return "", errors.WithStack(err)
			}
		}
		// Use only the first 8 characters in order to prevent errors on
		// Windows, which cannot handle long file paths.
		hashString := base32.StdEncoding.EncodeToString(hash.Sum(nil))[:8]
		buildDir = fmt.Sprintf("%s-%s", sanitizersSegment, hashString)
	}

	buildDir = filepath.Join(b.ProjectDir, ".cifuzz-build", "libfuzzer", buildDir)

	return buildDir, nil
}

// Configure calls cmake to "Generate a project buildsystem" (that's the
// phrasing used by the CMake man page).
// Note: This is usually a no-op after the directory has been created once,
// even if cache variables change. However, if a previous invocation of this
// command failed during CMake generation and the command is run again, the
// build step would only result in a very unhelpful error message about
// missing Makefiles. By reinvoking CMake's configuration explicitly here,
// we either get a helpful error message or the build step will succeed if
// the user fixed the issue in the meantime.
func (b *Builder) Configure() error {
	buildDir, err := b.BuildDir()
	if err != nil {
		return err
	}

	cacheArgs := []string{
		"-DCIFUZZ_ENGINE=libfuzzer",
		"-DCIFUZZ_SANITIZERS=" + strings.Join(b.Sanitizers, ";"),
		"-DCIFUZZ_TESTING:BOOL=ON",
	}
	if runtime.GOOS != "windows" {
		// CMAKE_BUILD_TYPE is ignored when building with MSBuild.
		// The config only has to be specified in the build step with
		// --config cmakeBuildConfiguration.
		cacheArgs = append(cacheArgs, "-DCMAKE_BUILD_TYPE="+cmakeBuildConfiguration)
		// Use relative paths in RPATH/RUNPATH so that binaries from the
		// build directory can find their shared libraries even when
		// packaged into an artifact.
		// On Windows, where there is no RPATH, there are two ways the user or
		// we can handle this:
		// 1. Use the TARGET_RUNTIME_DLLS generator expression introduced in
		//    CMake 3.21 to copy all DLLs into the directory of the executable
		//    in a post-build action.
		// 2. Add all library directories to PATH.
		cacheArgs = append(cacheArgs, "-DCMAKE_BUILD_RPATH_USE_ORIGIN:BOOL=ON")
	} else {
		// "-T ClangCL" is needed in order to use clang-cl instead of MSVC
		cacheArgs = append(cacheArgs, "-T ClangCL")
	}

	args := cacheArgs
	args = append(args, b.Args...)
	args = append(args, b.ProjectDir)

	cmd := exec.Command("cmake", args...)
	cmd.Stdout = b.Stdout
	cmd.Stderr = b.Stderr
	cmd.Env = b.env
	cmd.Dir = buildDir
	log.Debugf("Working directory: %s", cmd.Dir)
	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	return nil
}

// Build builds the specified fuzz tests with CMake. The fuzz tests must
// not contain duplicates.
func (b *Builder) Build(fuzzTests []string) ([]*build.CBuildResult, error) {
	buildDir, err := b.BuildDir()
	if err != nil {
		return nil, err
	}

	flags := append([]string{
		"--build", buildDir,
		"--config", cmakeBuildConfiguration,
		"--target"}, fuzzTests...)

	if b.Parallel.Enabled {
		flags = append(flags, "--parallel")
		if b.Parallel.NumJobs != 0 {
			flags = append(flags, fmt.Sprint(b.Parallel.NumJobs))
		}
	}

	cmd := exec.Command("cmake", flags...)
	cmd.Stdout = b.Stdout
	cmd.Stderr = b.Stderr
	cmd.Env = b.env
	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	if b.BuildOnly {
		return nil, nil
	}

	var results []*build.CBuildResult
	for _, fuzzTest := range fuzzTests {
		executable, err := b.findFuzzTestExecutable(fuzzTest)
		if err != nil {
			return nil, err
		}
		seedCorpus, err := b.findFuzzTestSeedCorpus(fuzzTest)
		if err != nil {
			return nil, err
		}
		dict, err := b.findFuzzTestDictionary(fuzzTest)
		if err != nil {
			return nil, err
		}

		var runtimeDeps []string
		if b.FindRuntimeDeps {
			// TODO if we have another solution for windows/darwin we should remove
			// the getRuntimeDeps and the related code in cifuzz-functions.cmake
			if runtime.GOOS == "linux" {
				runtimeDeps, err = ldd.NonSystemSharedLibraries(executable)
			} else {
				runtimeDeps, err = b.getRuntimeDeps(fuzzTest)
			}
			if err != nil {
				return nil, err
			}
		}

		generatedCorpus := filepath.Join(b.ProjectDir, ".cifuzz-corpus", fuzzTest)
		result := &build.CBuildResult{
			Name:       fuzzTest,
			ProjectDir: b.ProjectDir,
			Sanitizers: b.Sanitizers,
			BuildResult: &build.BuildResult{
				Executable:      executable,
				GeneratedCorpus: generatedCorpus,
				SeedCorpus:      seedCorpus,
				Dictionary:      dict,
				BuildDir:        buildDir,
				RuntimeDeps:     runtimeDeps,
			},
		}
		results = append(results, result)
	}

	return results, nil
}

// findFuzzTestExecutable uses the info files emitted by the CMake integration
// in the configure step to look up the canonical path of a fuzz test's
// executable.
func (b *Builder) findFuzzTestExecutable(fuzzTest string) (string, error) {
	return b.readInfoFileAsPath(fuzzTest, "executable")
}

// findFuzzTestSeedCorpus uses the info files emitted by the CMake integration
// in the configure step to look up the canonical path of a fuzz test's
// seed corpus directory.
func (b *Builder) findFuzzTestSeedCorpus(fuzzTest string) (string, error) {
	return b.readInfoFileAsPath(fuzzTest, "seed_corpus")
}

// findFuzzTestDict uses the info files emitted by the CMake integration
// in the configure step to look up the canonical path of a fuzz test's
// dictionary.
func (b *Builder) findFuzzTestDictionary(fuzzTest string) (string, error) {
	return b.readInfoFileAsPath(fuzzTest, "dict")
}

// ListFuzzTests lists all fuzz tests defined in the CMake project after
// Configure has been run.
func (b *Builder) ListFuzzTests() ([]string, error) {
	fuzzTestsDir, err := b.fuzzTestsInfoDir()
	if err != nil {
		return nil, err
	}
	fuzzTestEntries, err := os.ReadDir(fuzzTestsDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var fuzzTests []string
	for _, entry := range fuzzTestEntries {
		fuzzTests = append(fuzzTests, entry.Name())
	}
	fuzzTests = sliceutil.RemoveDuplicates(fuzzTests)
	return fuzzTests, nil
}

// getRuntimeDeps returns the canonical paths of all (transitive) runtime
// dependencies of the given fuzz test. It prints a warning if any dependency
// couldn't be resolved or resolves to more than one file.
func (b *Builder) getRuntimeDeps(fuzzTest string) ([]string, error) {
	buildDir, err := b.BuildDir()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"cmake",
		"--install",
		buildDir,
		"--config", cmakeBuildConfiguration,
		"--component", "cifuzz_internal_deps_"+fuzzTest,
	)
	log.Debugf("Command: %s", cmd.String())
	stdout, err := cmd.Output()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	var resolvedDeps []string
	var unresolvedDeps []string
	var conflictingDeps []string
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := scanner.Text()
		// Typical lines in the output of the install command look like this:
		//
		// <arbitrary CMake output>
		// -- CIFUZZ RESOLVED /usr/lib/system.so
		// -- CIFUZZ RESOLVED /home/user/git/project/build/lib/bar.so
		// -- CIFUZZ UNRESOLVED not_found.so

		// Skip over CMake output.
		if !strings.HasPrefix(line, "-- CIFUZZ ") {
			continue
		}
		statusAndDep := strings.TrimPrefix(line, "-- CIFUZZ ")
		endOfStatus := strings.Index(statusAndDep, " ")
		if endOfStatus == -1 {
			return nil, errors.Errorf("invalid runtime dep line: %s", line)
		}
		status := statusAndDep[:endOfStatus]
		dep := statusAndDep[endOfStatus+1:]

		// Filter well known system libraries
		if isSystemLibrary(dep) {
			continue
		}

		switch status {
		case "UNRESOLVED":
			unresolvedDeps = append(unresolvedDeps, dep)
		case "CONFLICTING":
			conflictingDeps = append(conflictingDeps, dep)
		case "RESOLVED":
			resolvedDeps = append(resolvedDeps, dep)
		default:
			return nil, errors.Errorf("invalid status '%s' in runtime dep line: %s", status, line)
		}
	}

	if len(unresolvedDeps) > 0 || len(conflictingDeps) > 0 {
		var warning strings.Builder
		if len(unresolvedDeps) > 0 {
			warning.WriteString(
				fmt.Sprintf("The following shared library dependencies of %s could not be resolved:\n", fuzzTest))
			for _, unresolvedDep := range unresolvedDeps {
				warning.WriteString(fmt.Sprintf("  %s\n", unresolvedDep))
			}
		}
		if len(conflictingDeps) > 0 {
			warning.WriteString(
				fmt.Sprintf("The following shared library dependencies of %s could not be resolved unambiguously:\n", fuzzTest))
			for _, conflictingDep := range conflictingDeps {
				warning.WriteString(fmt.Sprintf("  %s\n", conflictingDep))
			}
		}
		warning.WriteString("The archive may be incomplete.\n")
		log.Warn(warning.String())
	}

	return resolvedDeps, nil
}

// readInfoFileAsPath returns the contents of the CMake-generated info file of type kind for the given fuzz test,
// interpreted as a path. All symlinks are followed.
func (b *Builder) readInfoFileAsPath(fuzzTest string, kind string) (string, error) {

	fuzzTestsInfoDir, err := b.fuzzTestsInfoDir()
	if err != nil {
		return "", err
	}
	infoFile := filepath.Join(fuzzTestsInfoDir, fuzzTest, kind)
	content, err := os.ReadFile(infoFile)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(content), nil
}

func (b *Builder) fuzzTestsInfoDir() (string, error) {
	buildDir, err := b.BuildDir()
	if err != nil {
		return "", err
	}
	// The path to the info file for single-configuration CMake generators (e.g. Makefiles).
	fuzzTestsDir := filepath.Join(buildDir, ".cifuzz", "fuzz_tests")
	log.Debugf("Searching for test info file in %s", fuzzTestsDir)
	if fileutil.IsDir(fuzzTestsDir) {
		return fuzzTestsDir, nil
	}
	// The path to the info file for multi-configuration CMake generators (e.g. MSBuild).
	fuzzTestsDir = filepath.Join(buildDir, cmakeBuildConfiguration, ".cifuzz", "fuzz_tests")
	log.Debugf("Searching for test info file in %s", fuzzTestsDir)
	if fileutil.IsDir(fuzzTestsDir) {
		return fuzzTestsDir, nil
	}
	log.Warn("Did not find test info file")
	return "", errors.WithStack(os.ErrNotExist)
}

func isSystemLibrary(dep string) bool {
	for _, wellKnownSystemLibrary := range wellKnownSystemLibraries[runtime.GOOS] {
		if wellKnownSystemLibrary.MatchString(dep) {
			return true
		}
	}

	return false
}
