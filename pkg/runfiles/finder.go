package runfiles

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type RunfilesFinderImpl struct {
	InstallDir string
}

func (f RunfilesFinderImpl) BazelPath() (string, error) {
	path, err := exec.LookPath("bazel")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) CIFuzzIncludePath() (string, error) {
	return f.findFollowSymlinks("include")
}

func (f RunfilesFinderImpl) ClangPath() (string, error) {
	path, err := f.llvmToolPath("clang")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) CMakePath() (string, error) {
	path, err := exec.LookPath("cmake")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) CMakePresetsPath() (string, error) {
	return f.findFollowSymlinks("share/integration/CMakePresets.json")
}

func (f RunfilesFinderImpl) LLVMCovPath() (string, error) {
	path, err := f.llvmToolPath("llvm-cov")
	return path, err
}

func (f RunfilesFinderImpl) LLVMProfDataPath() (string, error) {
	path, err := f.llvmToolPath("llvm-profdata")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) LLVMSymbolizerPath() (string, error) {
	path, err := f.llvmToolPath("llvm-symbolizer")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) GenHTMLPath() (string, error) {
	if runtime.GOOS == "windows" {
		path := os.Getenv("PATH")
		for _, dir := range filepath.SplitList(path) {
			path := filepath.Join(dir, "genhtml")
			exists, err := fileutil.Exists(path)
			if err != nil {
				return "", err
			}
			if exists {
				return path, nil
			}
		}

		return "", errors.New("genhtml not found in %PATH%")
	}

	path, err := exec.LookPath("genhtml")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) PerlPath() (string, error) {
	path, err := exec.LookPath("perl")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) JavaPath() (string, error) {
	javaHome, err := f.JavaHomePath()
	if err != nil {
		return "", err
	}
	javaBin := filepath.Join(javaHome, "bin", "java")
	if runtime.GOOS == "windows" {
		javaBin = filepath.Join(javaHome, "bin", "java.exe")
	}

	return javaBin, nil
}

func (f RunfilesFinderImpl) MavenPath() (string, error) {
	path, err := exec.LookPath("mvn")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) GradlePath() (string, error) {
	path, err := exec.LookPath("gradle")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) NodePath() (string, error) {
	path, err := exec.LookPath("node")
	return path, errors.WithStack(err)
}

func (f RunfilesFinderImpl) Minijail0Path() (string, error) {
	return f.findFollowSymlinks("bin/minijail0")
}

func (f RunfilesFinderImpl) ProcessWrapperPath() (string, error) {
	return f.findFollowSymlinks("lib/process_wrapper")
}

func (f RunfilesFinderImpl) DumperPath() (string, error) {
	return f.findFollowSymlinks("lib/dumper.o")
}

func (f RunfilesFinderImpl) ReplayerSourcePath() (string, error) {
	return f.findFollowSymlinks("src/replayer.c")
}

func (f RunfilesFinderImpl) ListFuzzTestsJarPath() (string, error) {
	return f.findFollowSymlinks("share/java/list-fuzz-tests.jar")
}

func (f RunfilesFinderImpl) VisualStudioPath() (string, error) {
	path, found := os.LookupEnv("VSINSTALLDIR")
	if !found {
		log.Warn(`Please make sure that you run this command from a Developer Command Prompt for VS 2022.
Otherwise Visual Studio will not be found.`)
		return "", errors.New("Visual Studio not found")
	}
	return path, nil
}

func (f RunfilesFinderImpl) VSCodeTasksPath() (string, error) {
	return f.findFollowSymlinks("share/integration/tasks.json")
}

func (f RunfilesFinderImpl) LogoPath() (string, error) {
	return f.findFollowSymlinks("share/logo.png")
}

// JavaHomePath returns the absolute path to the base directory of the
// default system JDK/JRE. It first looks up JAVA_HOME and then falls back to
// using the java binary in the PATH.
func (f RunfilesFinderImpl) JavaHomePath() (string, error) {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" {
		return javaHome, nil
	}

	if runtime.GOOS == "darwin" {
		// On some macOS installations, an executable 'java_home' exists
		// which prints the JAVA_HOME of the default installation to stdout
		var outbuf bytes.Buffer
		cmd := exec.Command("/usr/libexec/java_home")
		cmd.Stdout = &outbuf
		err := cmd.Run()
		if err == nil {
			return strings.TrimSpace(outbuf.String()), nil
		}
	}

	javaSymlink, err := exec.LookPath("java")
	if err != nil {
		return "", errors.WithStack(err)
	}
	// The java binary in the PATH, e.g. at /usr/bin/java, is typically a
	// symlink pointing to the actual java binary in the bin subdirectory of the
	// JAVA_HOME.
	javaBinary, err := filepath.EvalSymlinks(javaSymlink)
	if err != nil {
		return "", errors.WithStack(err)
	}
	absoluteJavaBinary, err := filepath.Abs(javaBinary)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return filepath.Dir(filepath.Dir(absoluteJavaBinary)), nil
}

func (f RunfilesFinderImpl) findFollowSymlinks(relativePath string) (string, error) {
	absolutePath := filepath.Join(f.InstallDir, relativePath)

	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", errors.Wrapf(err, "path: %s", absolutePath)
	}
	_, err = os.Stat(resolvedPath)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return resolvedPath, nil
}

func (f RunfilesFinderImpl) llvmToolPath(name string) (string, error) {
	if runtime.GOOS == "windows" {
		visualStudioPath, err := f.VisualStudioPath()
		if err != nil {
			return "", err
		}

		path := os.Getenv("PATH")
		for _, dir := range filepath.SplitList(path) {
			// Only look for llvm tools in the visual studio path
			if !strings.HasPrefix(dir, visualStudioPath) {
				continue
			}

			path = filepath.Join(dir, name+".exe")
			exists, err := fileutil.Exists(path)
			if err != nil {
				return "", err
			}
			if exists {
				return path, nil
			}
		}

		return "", errors.Errorf("%s not found in %%PATH%%", name)
	}

	var err error
	var path string
	env := os.Environ()

	clangPath := envutil.GetEnvWithPathSubstring(env, "CC", "clang")
	if clangPath == "" {
		clangPath = envutil.GetEnvWithPathSubstring(env, "CXX", "clang++")
	}
	if clangPath != "" {
		path = filepath.Join(filepath.Dir(clangPath), name)
		found, err := fileutil.Exists(path)
		if err != nil {
			return "", err
		}
		if found {
			return path, nil
		}
	}

	path, err = exec.LookPath(name)
	return path, errors.WithStack(err)
}
