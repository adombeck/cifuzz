package runfiles

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type RunfilesFinder interface {
	BazelPath() (string, error)
	CIFuzzIncludePath() (string, error)
	ClangPath() (string, error)
	CMakePath() (string, error)
	CMakePresetsPath() (string, error)
	LLVMCovPath() (string, error)
	LLVMProfDataPath() (string, error)
	LLVMSymbolizerPath() (string, error)
	GenHTMLPath() (string, error)
	PerlPath() (string, error)
	Minijail0Path() (string, error)
	ProcessWrapperPath() (string, error)
	DumperPath() (string, error)
	ReplayerSourcePath() (string, error)
	ListFuzzTestsJarPath() (string, error)
	VisualStudioPath() (string, error)
	VSCodeTasksPath() (string, error)
	LogoPath() (string, error)
	MavenPath() (string, error)
	GradlePath() (string, error)
	JavaPath() (string, error)
	JavaHomePath() (string, error)
	NodePath() (string, error)
}

var Finder RunfilesFinder

func init() {
	// Set the default runfiles finder.
	//
	// If the environment variable CIFUZZ_INSTALL_ROOT is set, we use
	// that as the installation directory, else we assume that the
	// current executable lives in $INSTALL_DIR/bin, so we go up one
	// directory from there and use that as the installation directory.
	installDir, found := os.LookupEnv("CIFUZZ_INSTALL_ROOT")
	if !found || installDir == "" {
		executablePath, err := os.Executable()
		if err != nil {
			panic(errors.WithStack(err))
		}
		executablePath, err = filepath.EvalSymlinks(executablePath)
		if err != nil {
			panic(errors.WithStack(err))
		}

		installDir, err = filepath.Abs(filepath.Join(filepath.Dir(executablePath), ".."))
		if err != nil {
			panic(errors.WithStack(err))
		}
	}

	Finder = RunfilesFinderImpl{InstallDir: installDir}
}
