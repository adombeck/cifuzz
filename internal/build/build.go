package build

import (
	"os"
	"runtime"

	"github.com/Masterminds/semver"

	"code-intelligence.com/cifuzz/util/envutil"
)

// BuildResult contains fields which are needed to run the fuzz test
type BuildResult struct {
	// Canonical path of the fuzz test executable
	Executable string
	// Canonical path of the fuzz test's generated corpus directory
	GeneratedCorpus string
	// Canonical path of the fuzz test's default seed corpus directory
	SeedCorpus string
	// Canonical path of the fuzz test's default dictionary
	Dictionary string
	// Canonical path of the build directory
	BuildDir string
	// The canonical paths of the fuzz test's runtime dependencies
	RuntimeDeps []string
}

// CBuildResult contains the fields needed to run or bundle a C project which has been built
type CBuildResult struct {
	*BuildResult
	// A name which uniquely identifies the fuzz test and is a valid path
	Name string
	// The sanitizers with which the fuzz test was built
	Sanitizers []string
	// Canonical path of the directory to which source file paths should
	// be made relative
	ProjectDir string
}

// JavaBuildResult contains the fields needed to run or bundle a Java (or other JVM language) project which has been built
type JavaBuildResult struct {
	*BuildResult
}

func CommonBuildEnv() ([]string, error) {
	var err error
	env := os.Environ()

	// Set CIFUZZ=1 to allow the build system to figure out that it was
	// started by cifuzz.
	env, err = envutil.Setenv(env, "CIFUZZ", "1")
	if err != nil {
		return nil, err
	}

	// On Windows, our preferred compiler is clang-cl, which can't easily be run
	// from an arbitrary terminal as it requires about a dozen environment
	// variables to be set correctly. Thus, we assume users to run cifuzz from
	// a developer command prompt anyway and thus don't need to set the
	// compiler explicitly.
	if runtime.GOOS != "windows" {
		// Set the C/C++ compiler to clang/clang++ (if not already set),
		// which is needed to build a  binary with fuzzing instrumentation
		// gcc doesn't have -fsanitize=fuzzer.
		if val := envutil.GetEnvWithPathSubstring(env, "CC", "clang"); val == "" {
			env, err = envutil.Setenv(env, "CC", "clang")
			if err != nil {
				return nil, err
			}
		}
		if val := envutil.GetEnvWithPathSubstring(env, "CXX", "clang++"); val == "" {
			env, err = envutil.Setenv(env, "CXX", "clang++")
			if err != nil {
				return nil, err
			}
		}
	}

	// We don't want to fail if ASan is set up incorrectly for tools
	// built and executed during the build or they contain leaks.
	env, err = envutil.Setenv(env, "ASAN_OPTIONS", "detect_leaks=0:verify_asan_link_order=0")
	if err != nil {
		return nil, err
	}

	return env, nil
}

var commonCFlags = []string{
	// Keep debug symbols
	"-g",
	// Do optimizations which don't harm debugging
	"-Og",
	// To get good stack frames for better debugging
	"-fno-omit-frame-pointer",
	// Conventional macro to conditionally compile out fuzzer road blocks
	// See https://llvm.org/docs/LibFuzzer.html#fuzzer-friendly-build-mode
	"-DFUZZING_BUILD_MODE_UNSAFE_FOR_PRODUCTION",
	// Ensure that asserts are enabled regardless of compilation mode (e.g. explicit -DNDEBUG).
	"-UNDEBUG",
}

func LibFuzzerCFlags() []string {
	// These flags must not contain spaces, because the environment
	// variables that are set to these flags are space separated.
	// Note: Keep in sync with share/cmake/cifuzz-functions.cmake
	return append(commonCFlags, []string{
		// ----- Flags used to build with libFuzzer -----
		// Compile with edge coverage and compare instrumentation. We
		// use fuzzer-no-link here instead of -fsanitize=fuzzer because
		// CFLAGS are often also passed to the linker, which would cause
		// errors if the build includes tools which have a main function.
		"-fsanitize=fuzzer-no-link",

		// ----- Flags used to build with ASan -----
		// Build with instrumentation for ASan and UBSan and link in
		// their runtime
		"-fsanitize=address,undefined",
		// To support recovering from ASan findings
		"-fsanitize-recover=address",
		// Use additional error detectors for use-after-scope bugs
		// TODO: Evaluate the slow down caused by this flag
		// TODO: Check if there are other additional error detectors
		//       which we want to use
		"-fsanitize-address-use-after-scope",
		// Disable source fortification, which is currently not supported
		// in combination with ASan, see https://github.com/google/sanitizers/issues/247
		"-U_FORTIFY_SOURCE",
	}...)
}

func CoverageCFlags(clangVersion *semver.Version) []string {
	cflags := append(commonCFlags, []string{
		// ----- Flags used to build with code coverage -----
		// Generate instrumented code to collect execution counts
		"-fprofile-instr-generate",
		// Generate coverage mapping to enable code coverage analysis
		"-fcoverage-mapping",
		// Disable source fortification to ensure that coverage builds
		// reach all code reached by ASan builds.
		"-U_FORTIFY_SOURCE",
	}...)

	if runtime.GOOS != "darwin" && clangVersion != nil {
		// LLVM's continuous mode requires compile-time support on non-macOS
		// platforms. This support is unstable in Clang 13 and lower, so we
		// only enable it on 14+.
		if clangVersion.Compare(semver.MustParse("14.0.0")) >= 0 {
			cflags = append(cflags, "-mllvm", "-runtime-counter-relocation")
		}
	}
	return cflags
}
