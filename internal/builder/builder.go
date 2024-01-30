package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alexflint/go-filemutex"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type CIFuzzBuilder struct {
	Options

	projectDir string
	mutex      *filemutex.FileMutex
	isLocked   bool
}

type Options struct {
	Version   string
	TargetDir string
	GOOS      string
	GOARCH    string
	// build cifuzz with coverage instrumentation
	Coverage bool
}

func NewCIFuzzBuilder(opts Options) (*CIFuzzBuilder, error) {
	var err error

	// Validate options
	if opts.Version == "" {
		return nil, errors.New("No version is provided")
	}
	opts.TargetDir, err = validateTargetDir(opts.TargetDir)
	if err != nil {
		return nil, err
	}

	opts.TargetDir, err = filepath.Abs(opts.TargetDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	projectDir, err := FindProjectDir()
	if err != nil {
		return nil, err
	}

	i := &CIFuzzBuilder{
		Options:    opts,
		projectDir: projectDir,
	}

	i.mutex, err = filemutex.New(i.lockFile())
	if err != nil {
		// filemutex.New returns errors from syscall.Open without the
		// path, so we wrap it in the os.PathError same as os.Open does.
		return nil, errors.WithStack(&os.PathError{Op: "open", Path: i.lockFile(), Err: err})
	}

	err = i.createDirectoryLayout()
	if err != nil {
		i.Cleanup()
		return nil, err
	}

	log.Printf("Building cifuzz in %v", opts.TargetDir)

	return i, nil
}

func (i *CIFuzzBuilder) createDirectoryLayout() error {
	err := os.MkdirAll(i.binDir(), 0755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.MkdirAll(i.includeDir(), 0755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.MkdirAll(i.libDir(), 0755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.MkdirAll(i.shareDir(), 0755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.MkdirAll(i.srcDir(), 0755)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (i *CIFuzzBuilder) binDir() string {
	return filepath.Join(i.TargetDir, "bin")
}

func (i *CIFuzzBuilder) includeDir() string {
	return filepath.Join(i.TargetDir, "include")
}

func (i *CIFuzzBuilder) libDir() string {
	return filepath.Join(i.TargetDir, "lib")
}

func (i *CIFuzzBuilder) shareDir() string {
	return filepath.Join(i.TargetDir, "share")
}

func (i *CIFuzzBuilder) srcDir() string {
	return filepath.Join(i.TargetDir, "src")
}

func (i *CIFuzzBuilder) lockFile() string {
	return filepath.Join(i.projectDir, ".installer-lock")
}

func (i *CIFuzzBuilder) Cleanup() {
	fileutil.Cleanup(i.TargetDir)
	// Always remove the lock file, even if SKIP_CLEANUP is set, because
	// keeping it around is not useful for debugging purposes.
	_ = os.Remove(i.lockFile())
}

// Lock acquires a file lock to make sure that only one instance of the
// installer is executed at the same time. Note that this function does
// not provide thread-safety for using the same installer instance
// multiple times.
func (i *CIFuzzBuilder) Lock() error {
	if i.isLocked {
		return nil
	}
	err := i.mutex.Lock()
	if err != nil {
		return errors.WithStack(err)
	}
	i.isLocked = true
	return nil

}

// Unlock releases the file lock to allow other installer instances to
// run.
func (i *CIFuzzBuilder) Unlock() error {
	if !i.isLocked {
		return nil
	}
	err := i.mutex.Unlock()
	if err != nil {
		return errors.WithStack(err)
	}
	i.isLocked = false
	return nil
}

func (i *CIFuzzBuilder) BuildCIFuzzAndDeps() error {
	var err error

	err = i.Lock()
	if err != nil {
		return err
	}
	defer func() {
		err = i.Unlock()
		if err != nil {
			log.Printf("error: %v", err)
		}
	}()

	if runtime.GOOS == "linux" {
		err = i.BuildMinijail()
		if err != nil {
			return err
		}

		err = i.BuildProcessWrapper()
		if err != nil {
			return err
		}
	}

	err = i.BuildDumper()
	if err != nil {
		return err
	}

	err = i.BuildListFuzzTestsTool()
	if err != nil {
		return err
	}

	err = i.BuildCIFuzz()
	if err != nil {
		return err
	}

	err = i.CopyFiles()
	if err != nil {
		return err
	}

	return nil
}

func (i *CIFuzzBuilder) BuildMinijail() error {
	var err error

	err = i.Lock()
	if err != nil {
		return err
	}
	defer func() {
		err = i.Unlock()
		if err != nil {
			log.Printf("error: %v", err)
		}
	}()

	minijailDir := filepath.Join(i.projectDir, "third-party", "minijail")

	// Build minijail
	cmd := exec.Command("make", "CC_BINARY(minijail0)")
	cmd.Dir = minijailDir
	// The minijail Makefile changes the directory to $PWD, so we have
	// to set that.
	cmd.Env, err = envutil.Setenv(os.Environ(), "PWD", filepath.Join(i.projectDir, "third-party", "minijail"))
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	log.Printf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return errors.WithStack(err)
	}

	// Copy minijail binary
	src := filepath.Join(i.projectDir, "third-party", "minijail", "minijail0")
	dest := filepath.Join(i.binDir(), "minijail0")
	err = copy.Copy(src, dest)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (i *CIFuzzBuilder) BuildProcessWrapper() error {
	var err error
	err = i.Lock()
	if err != nil {
		return err
	}
	defer func() {
		err = i.Unlock()
		if err != nil {
			log.Printf("error: %v", err)
		}
	}()

	// Build process wrapper
	compiler := envutil.GetEnvWithPathSubstring(os.Environ(), "CC", "clang")
	if compiler == "" {
		compiler, err = exec.LookPath("clang")
		if err != nil {
			return errors.WithStack(err)
		}
	}
	dest := filepath.Join(i.libDir(), "process_wrapper")
	cmd := exec.Command(compiler, "-o", dest, "process_wrapper.c")
	cmd.Dir = filepath.Join(i.projectDir, "pkg", "minijail", "process_wrapper", "src")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	log.Printf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (i *CIFuzzBuilder) BuildDumper() error {
	var err error
	err = i.Lock()
	if err != nil {
		return err
	}
	defer func() {
		err = i.Unlock()
		if err != nil {
			log.Printf("error: %v", err)
		}
	}()

	// Build dumper
	compiler := envutil.GetEnvWithPathSubstring(os.Environ(), "CC", "clang")
	if compiler == "" {
		compiler, err = exec.LookPath("clang")
		if err != nil {
			return errors.WithStack(err)
		}
	}

	dest := filepath.Join(i.libDir(), "dumper.o")
	args := []string{"-c", "-o", dest, "dumper.c"}
	if runtime.GOOS != "windows" {
		args = append(args, "-fPIC")
	}

	cmd := exec.Command(compiler, args...)
	cmd.Dir = filepath.Join(i.projectDir, "tools", "dumper")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	log.Printf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (i *CIFuzzBuilder) BuildListFuzzTestsTool() error {
	var err error
	err = i.Lock()
	if err != nil {
		return err
	}
	defer func() {
		err = i.Unlock()
		if err != nil {
			log.Printf("error: %v", err)
		}
	}()

	listFuzzTestsDir := filepath.Join(i.projectDir, "tools", "list-fuzz-tests")

	mvn, err := runfiles.Finder.MavenPath()
	if err != nil {
		return err
	}

	args := []string{mvn, "package"}
	// Hide the progress output from Maven if stdout is not a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		args = append(args, "--batch-mode")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = listFuzzTestsDir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	log.Printf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return errors.WithStack(err)
	}

	err = copy.Copy(
		filepath.Join(listFuzzTestsDir, "target", "list-fuzz-tests.jar"),
		filepath.Join(i.shareDir(), "java", "list-fuzz-tests.jar"),
	)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (i *CIFuzzBuilder) BuildCIFuzz() error {
	var err error
	err = i.Lock()
	if err != nil {
		return err
	}
	defer func() {
		err = i.Unlock()
		if err != nil {
			log.Printf("error: %v", err)
		}
	}()

	// Add GOOS and GOARCH envs to support cross compilation
	buildEnv := os.Environ()
	buildEnv = append(buildEnv, []string{"GOOS=" + i.GOOS, "GOARCH=" + i.GOARCH}...)

	// Build cifuzz
	ldFlags := fmt.Sprintf("-ldflags=-X code-intelligence.com/cifuzz/internal/version.Version=%s", i.Version)
	cifuzz := filepath.Join("cmd", "cifuzz", "main.go")

	args := []string{
		"build",
		"-o",
		CIFuzzExecutablePath(i.binDir()),
		ldFlags,
	}

	if i.Coverage {
		args = append(args, "-cover")
	}

	args = append(args, cifuzz)

	cmd := exec.Command("go", args...)
	cmd.Dir = i.projectDir
	cmd.Env = buildEnv
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	log.Printf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// CopyFiles copies all files which don't need to built to the target directory.
func (i *CIFuzzBuilder) CopyFiles() error {
	var err error
	err = i.Lock()
	if err != nil {
		return err
	}
	defer func() {
		err = i.Unlock()
		if err != nil {
			log.Printf("error: %v", err)
		}
	}()

	opts := copy.Options{
		// Create deep copies of symlinks because the installer embeds
		// the target directory and the Go embed package doesn't support
		// symlinks
		OnSymlink: func(symlink string) copy.SymlinkAction {
			return copy.Deep
		},
	}

	// Copy the share directory
	err = copy.Copy(filepath.Join(i.projectDir, "share"), i.shareDir(), opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Copy the include directory
	err = copy.Copy(filepath.Join(i.projectDir, "include"), i.includeDir(), opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Copy C/C++ source files to the src directory
	err = copy.Copy(filepath.Join(i.projectDir, "tools", "dumper"), i.srcDir(), opts)
	if err != nil {
		return errors.WithStack(err)
	}
	err = copy.Copy(filepath.Join(i.projectDir, "tools", "launcher"), i.srcDir(), opts)
	if err != nil {
		return errors.WithStack(err)
	}
	err = copy.Copy(filepath.Join(i.projectDir, "tools", "replayer", "src"), i.srcDir(), opts)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func FindProjectDir() (string, error) {
	// Find the project root directory
	projectDir, err := os.Getwd()
	if err != nil {
		return "", errors.WithStack(err)
	}
	exists, err := fileutil.Exists(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return "", err
	}
	for !exists {
		if filepath.Dir(projectDir) == projectDir {
			return "", errors.Errorf("Couldn't find project root directory")
		}
		projectDir = filepath.Dir(projectDir)
		exists, err = fileutil.Exists(filepath.Join(projectDir, "go.mod"))
		if err != nil {
			return "", err
		}
	}
	return projectDir, nil
}

func CIFuzzExecutablePath(binDir string) string {
	path := filepath.Join(binDir, "cifuzz")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	return path
}

func validateTargetDir(targetDir string) (string, error) {
	var err error

	if strings.HasPrefix(targetDir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.WithStack(err)
		}
		targetDir = home + strings.TrimPrefix(targetDir, "~")
	}

	targetDir, err = filepath.Abs(targetDir)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return targetDir, nil
}
