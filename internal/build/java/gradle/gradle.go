package gradle

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/fileutil"
)

const (
	PluginMissingErrorMsg string = "Failed to access CI Fuzz gradle plugin"
)

var (
	classpathRegex         = regexp.MustCompile("(?m)^cifuzz.test.classpath=(?P<classpath>.*)$")
	buildDirRegex          = regexp.MustCompile("(?m)^cifuzz.buildDir=(?P<buildDir>.*)$")
	rootDirRegex           = regexp.MustCompile("(?m)^cifuzz.rootDir=(?P<rootDir>.*)$")
	testSourceFoldersRegex = regexp.MustCompile("(?m)^cifuzz.test.source-folders=(?P<testSourceFolders>.*)$")
	mainSourceFoldersRegex = regexp.MustCompile("(?m)^cifuzz.main.source-folders=(?P<mainSourceFolders>.*)$")
)

func FindGradleWrapper(projectDir string) (string, error) {
	wrapper := "gradlew"
	if runtime.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}

	return fileutil.SearchFileBackwards(projectDir, wrapper)
}

type ParallelOptions struct {
	Enabled bool
	NumJobs uint
}

type BuilderOptions struct {
	ProjectDir string
	Parallel   ParallelOptions
	Stdout     io.Writer
	Stderr     io.Writer
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
}

func NewBuilder(opts *BuilderOptions) (*Builder, error) {
	err := opts.Validate()
	if err != nil {
		return nil, err
	}

	b := &Builder{BuilderOptions: opts}

	return b, err
}

func (b *Builder) Build() (*build.BuildResult, error) {
	version, err := b.GradlePluginVersion()
	if err != nil {
		return nil, err
	}
	log.Debugf("Found gradle plugin version: %s", version)

	deps, err := GetDependencies(b.ProjectDir)
	if err != nil {
		return nil, err
	}

	result := &build.BuildResult{
		// BuildDir is not used by Jazzer
		BuildDir:    "",
		RuntimeDeps: deps,
	}

	return result, nil
}

func (b *Builder) GradlePluginVersion() (string, error) {
	cmd, err := buildGradleCommand(b.ProjectDir, []string{"cifuzzPrintPluginVersion", "-q"})
	if err != nil {
		return "", err
	}
	log.Debugf("Command: %s", cmd.String())

	// Write cmd.Stderr to a buffer to catch if an error is caused by a
	// missing plugin. In that case we want to print a help message,
	// in all other cases we want to return the error and display the full
	// error output from the command.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "Task 'cifuzzPrintPluginVersion' not found") {
			return "", errors.New(PluginMissingErrorMsg)
		}
		_, writeErr := b.Stderr.Write(stderr.Bytes())
		if writeErr != nil {
			log.Errorf(errors.WithStack(writeErr), "Failed to write command output to stderr: %v", writeErr.Error())
		}
		return "", errors.WithStack(err)
	}

	return strings.TrimPrefix(string(output), "cifuzz.plugin.version="), nil
}

func GetDependencies(projectDir string) ([]string, error) {
	cmd, err := buildGradleCommand(projectDir, []string{"cifuzzPrintTestClasspath", "-q"})
	if err != nil {
		return nil, err
	}
	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	classpath := classpathRegex.FindStringSubmatch(string(output))
	deps := strings.Split(strings.TrimSpace(classpath[1]), string(os.PathListSeparator))

	return deps, nil
}

// GetGradleCommand returns the name of the gradle command.
// The gradle wrapper is preferred to use and gradle
// acts as a fallback command.
func GetGradleCommand(projectDir string) (string, error) {
	wrapper, err := FindGradleWrapper(projectDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if wrapper != "" {
		return wrapper, nil
	}

	gradleCmd, err := runfiles.Finder.GradlePath()
	if err != nil {
		return "", err
	}
	return gradleCmd, nil
}

func buildGradleCommand(projectDir string, args []string) (*exec.Cmd, error) {
	gradleCmd, err := GetGradleCommand(projectDir)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(gradleCmd, args...)
	cmd.Dir = projectDir

	return cmd, nil
}

func GetBuildDirectory(projectDir string) (string, error) {
	cmd, err := buildGradleCommand(projectDir, []string{"cifuzzPrintBuildDir", "-q"})
	if err != nil {
		return "", nil
	}

	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	if err != nil {
		return "", cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	result := buildDirRegex.FindStringSubmatch(string(output))
	if result == nil {
		return "", errors.New("Unable to parse gradle build directory from init script.")
	}
	buildDir := strings.TrimSpace(result[1])

	return buildDir, nil
}

func GetRootDirectory(projectDir string) (string, error) {
	cmd, err := buildGradleCommand(projectDir, []string{"cifuzzPrintRootDir", "-q"})
	if err != nil {
		return "", nil
	}

	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	if err != nil {
		return "", cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	result := rootDirRegex.FindStringSubmatch(string(output))
	if result == nil {
		return "", errors.New("Unable to parse gradle root directory from init script.")
	}
	rootDir := strings.TrimSpace(result[1])

	return rootDir, nil
}

func GetTestSourceSets(projectDir string) ([]string, error) {
	cmd, err := buildGradleCommand(projectDir, []string{"cifuzzPrintTestSourceFolders", "-q"})
	if err != nil {
		return nil, err
	}

	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	result := testSourceFoldersRegex.FindStringSubmatch(string(output))
	if result == nil {
		return nil, errors.New("Unable to parse gradle test sources.")
	}
	paths := strings.Split(strings.TrimSpace(result[1]), string(os.PathListSeparator))

	// only return valid paths
	var sourceSets []string
	for _, path := range paths {
		exists, err := fileutil.Exists(path)
		if err != nil {
			return nil, errors.WithMessagef(err, "Error checking if Gradle test source path %s exists", path)
		}
		if exists {
			sourceSets = append(sourceSets, path)
		}
	}

	log.Debugf("Found gradle test sources at: %s", sourceSets)
	return sourceSets, nil
}

func GetMainSourceSets(projectDir string) ([]string, error) {
	cmd, err := buildGradleCommand(projectDir, []string{"cifuzzPrintMainSourceFolders", "-q"})
	if err != nil {
		return nil, err
	}

	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	result := mainSourceFoldersRegex.FindStringSubmatch(string(output))
	if result == nil {
		return nil, errors.New("Unable to parse gradle main sources.")
	}
	paths := strings.Split(strings.TrimSpace(result[1]), string(os.PathListSeparator))

	// only return valid paths
	var sourceSets []string
	for _, path := range paths {
		exists, err := fileutil.Exists(path)
		if err != nil {
			return nil, errors.WithMessagef(err, "Error checking if Gradle main source path %s exists", path)
		}
		if exists {
			sourceSets = append(sourceSets, path)
		}
	}

	log.Debugf("Found gradle main sources at: %s", sourceSets)
	return sourceSets, nil
}
