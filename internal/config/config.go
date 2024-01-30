package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

const (
	BuildSystemBazel  string = "bazel"
	BuildSystemCMake  string = "cmake"
	BuildSystemNodeJS string = "nodejs"
	BuildSystemMaven  string = "maven"
	BuildSystemGradle string = "gradle"
	BuildSystemOther  string = "other"
)

var buildSystemTypes = []string{
	BuildSystemBazel,
	BuildSystemCMake,
	BuildSystemNodeJS,
	BuildSystemMaven,
	BuildSystemGradle,
	BuildSystemOther,
}

var supportedBuildSystems = map[string][]string{
	"linux": buildSystemTypes,
	"darwin": {
		BuildSystemCMake,
		BuildSystemNodeJS,
		BuildSystemMaven,
		BuildSystemGradle,
		BuildSystemOther,
	},
	"windows": {
		BuildSystemCMake,
		BuildSystemNodeJS,
		BuildSystemMaven,
		BuildSystemGradle,
	},
}

const ProjectConfigFile = "cifuzz.yaml"

const AllowUnsupportedPlatformsEnv = "CIFUZZ_ALLOW_UNSUPPORTED_PLATFORMS"

//go:embed cifuzz.yaml.tmpl
var projectConfigTemplate string

// CreateProjectConfig creates a new project config in the given directory
func CreateProjectConfig(configDir string, server string, project string) (string, error) {
	// try to open the target file, returns error if already exists
	configpath := filepath.Join(configDir, ProjectConfigFile)
	f, err := os.OpenFile(configpath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return configpath, errors.WithStack(err)
		}
		return "", errors.WithStack(err)
	}

	// if the user is using the default server,
	// we don't want to save it in the config file
	if strings.Contains(server, "app.code-intelligence.com") {
		server = ""
	}

	// setup config struct with (default) values
	config := struct {
		LastUpdated string
		Server      string
		Project     string
	}{
		time.Now().Format("2006-01-02"),
		server,
		project,
	}

	// parse the template and write it to config file
	t, err := template.New("project_config").Parse(projectConfigTemplate)
	if err != nil {
		return "", errors.WithStack(err)
	}

	err = t.Execute(f, config)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return configpath, nil
}

func FindAndParseProjectConfig(opts interface{}) error {
	var configDir string
	var err error

	// If a config dir is set in the options, use that
	v := reflect.ValueOf(opts).Elem().FieldByName("ConfigDir")
	if v.IsValid() && v.String() != "" {
		configDir = v.String()
	} else {
		configDir, err = FindConfigDir()
		if err != nil {
			return errors.WithMessage(err, "Failed to determine config directory for cifuzz.yaml")
		}
	}

	err = ParseProjectConfig(configDir, opts)
	if err != nil {
		return errors.WithMessage(err, "Failed to parse cifuzz.yaml")
	}

	return nil
}

func ParseProjectConfig(configDir string, opts interface{}) error {
	configpath := filepath.Join(configDir, ProjectConfigFile)
	viper.SetConfigFile(configpath)

	// Set defaults
	useSandboxDefault := runtime.GOOS == "linux"
	viper.SetDefault("sandbox", useSandboxDefault)

	err := viper.ReadInConfig()
	if err != nil {
		return errors.WithStack(err)
	}

	// viper.Unmarshal doesn't return an error if the timeout value is
	// missing a unit, so we check that manually
	if viper.GetString("timeout") != "" {
		_, err = time.ParseDuration(viper.GetString("timeout"))
		if err != nil {
			return errors.Wrap(err, "error decoding 'timeout'")
		}
	}

	err = viper.Unmarshal(opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// If the build system was not set by the user, try to determine it
	// automatically.
	v := reflect.ValueOf(opts).Elem().FieldByName("BuildSystem")
	if v.IsValid() && v.String() == "" {
		buildSystem, err := DetermineBuildSystem(configDir)
		if err != nil {
			return err
		}
		v.SetString(buildSystem)
	}

	// If the project dir was not set by the user, set it to the config dir
	v = reflect.ValueOf(opts).Elem().FieldByName("ProjectDir")
	if v.IsValid() && v.String() == "" {
		v.SetString(configDir)
	}

	return nil
}

func ValidateBuildSystem(buildSystem string) error {
	if os.Getenv(AllowUnsupportedPlatformsEnv) != "" {
		log.Infof("%s is set. Be aware that this skips all OS/build system checks and can cause unforeseen results.", AllowUnsupportedPlatformsEnv)
		return nil
	}

	if !stringutil.Contains(buildSystemTypes, buildSystem) {
		return errors.Errorf("cifuzz currently does not support \"%s\"", buildSystem)
	}

	if !stringutil.Contains(supportedBuildSystems[runtime.GOOS], buildSystem) {
		return errors.Errorf(NotSupportedErrorMessage(buildSystem, runtime.GOOS))
	}

	return nil
}

func DetermineBuildSystem(projectDir string) (string, error) {
	buildSystemIdentifier := map[string][]string{
		BuildSystemBazel:  {"WORKSPACE", "WORKSPACE.bazel"},
		BuildSystemCMake:  {"CMakeLists.txt"},
		BuildSystemNodeJS: {"package.json", "package-lock.json", "yarn.lock", "node_modules/"},
		BuildSystemMaven:  {"pom.xml"},
		BuildSystemGradle: {"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"},
	}

	for buildSystem, files := range buildSystemIdentifier {
		for _, f := range files {
			isBuildSystem, err := fileutil.Exists(filepath.Join(projectDir, f))
			if err != nil {
				return "", err
			}

			if isBuildSystem {
				return buildSystem, nil
			}
		}
	}

	return BuildSystemOther, nil
}

func IsGradleMultiProject(projectDir string) (bool, error) {
	matches, err := zglob.Glob(filepath.Join(projectDir, "settings.{gradle,gradle.kts}"))
	if err != nil {
		return false, errors.WithStack(err)
	}
	if len(matches) == 0 {
		return false, nil
	}
	return true, nil
}

func DetermineGradleBuildLanguage(projectDir string) (GradleBuildLanguage, error) {
	kts, err := fileutil.Exists(filepath.Join(projectDir, "build.gradle.kts"))
	if err != nil {
		return "", err
	}
	if kts {
		return GradleKotlin, nil
	}

	return GradleGroovy, nil
}

func TestTypeFileNameExtension(testType FuzzTestType) (string, bool) {
	fileNameExtension := map[FuzzTestType]string{
		Java:   ".java",
		Kotlin: ".kt",
	}

	extension, found := fileNameExtension[testType]
	if !found {
		return "", false
	}
	return extension, true
}

func FindConfigDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", errors.WithStack(err)
	}
	configFileExists, err := fileutil.Exists(filepath.Join(dir, ProjectConfigFile))
	if err != nil {
		return "", err
	}
	for !configFileExists {
		if dir == filepath.Dir(dir) {
			return "", fmt.Errorf("not a cifuzz project (or any of the parent directories): %s %w", ProjectConfigFile, os.ErrNotExist)
		}
		dir = filepath.Dir(dir)
		configFileExists, err = fileutil.Exists(filepath.Join(dir, ProjectConfigFile))
		if err != nil {
			return "", err
		}
	}

	return dir, nil
}

func EnsureProjectEntry(configContent string, project string) string {
	// check if there is already a project entry (with or without a comment)
	re := regexp.MustCompile(`(?m)^#*\s*project:.*$`)
	// if there  is not, append it
	if !re.MatchString(configContent) {
		return fmt.Sprintf("%s\nproject: %s\n", configContent, project)
	}
	// if there is, set it
	return re.ReplaceAllString(configContent, fmt.Sprintf(`project: %s`, project))
}

func NotSupportedErrorMessage(tool string, platform string) string {
	prettyString := func(text string) string {
		switch text {
		case "maven", "gradle", "bazel", "linux", "windows":
			return cases.Title(language.Und).String(text)
		case "other":
			return "other build systems"
		case "cmake":
			return "CMake"
		case "nodejs":
			return "NodeJS"
		case "nodets":
			return "NodeTS"
		case "darwin":
			return "macOS"
		case "bundle", "coverage", "remote run", "run":
			return fmt.Sprintf("'%s'", text)
		default:
			return text
		}
	}

	return fmt.Sprintf(
		`cifuzz currently does not support %s with %s.
If you are interested in using this feature, please contact us via cifuzz@code-intelligence.com.`,
		prettyString(tool), prettyString(platform))
}

func AllowUnsupportedPlatforms() bool {
	return os.Getenv(AllowUnsupportedPlatformsEnv) != ""
}
