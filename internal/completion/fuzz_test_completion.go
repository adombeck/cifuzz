package completion

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/regexutil"
)

// This regex is based on the bazel bash completion script, see:
// https://github.com/bazelbuild/bazel/blob/021c2a053780d697899cbcbd76a032c72cd5cbbb/scripts/bazel-complete-template.bash#L173
var bazelFuzzTestTargetPattern = regexp.MustCompile(`cc_fuzz_test *\([^)]* {0,1}name *= *['"](?P<name>[a-zA-Z0-9_.+=,@~-]*)['"][^)]*\)`)

// ValidFuzzTests can be used as a cobra ValidArgsFunction that completes fuzz test names.
func ValidFuzzTests(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Change the directory if the `--directory` flag was set
	err := cmdutils.Chdir()
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	// Read the project config to figure out the build system
	conf := struct {
		BuildSystem string `mapstructure:"build-system"`
		ProjectDir  string `mapstructure:"project-dir"`
	}{}
	err = config.FindAndParseProjectConfig(&conf)
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	switch conf.BuildSystem {
	case config.BuildSystemBazel:
		return validBazelFuzzTests(toComplete)
	case config.BuildSystemCMake:
		return validCMakeFuzzTests(conf.ProjectDir)
	case config.BuildSystemMaven, config.BuildSystemGradle:
		return validJVMFuzzTests(conf.ProjectDir, toComplete)
	case config.BuildSystemNodeJS:
		return validNodeFuzzTests(conf.ProjectDir, toComplete)

	case config.BuildSystemOther:
		// For other build systems, the <fuzz test> argument must be
		// the path to the fuzz test executable, so we use file
		// completion here (which is only useful if the executable has
		// been built before, but that's still better than no completion
		// support)
		return nil, cobra.ShellCompDirectiveDefault
	default:
		err := errors.Errorf("Unsupported build system \"%s\"", conf.BuildSystem)
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}
}

func validBazelFuzzTests(toComplete string) ([]string, cobra.ShellCompDirective) {
	if strings.HasPrefix(toComplete, "//") {
		return absoluteBazelFuzzTestLabels(toComplete)
	} else {
		return relativeBazelFuzzTestLabels(toComplete)
	}
}

func absoluteBazelFuzzTestLabels(toComplete string) ([]string, cobra.ShellCompDirective) {
	var res []string

	workSpace, err := getWorkspacePath()
	if err != nil {
		log.Error(errors.WithMessage(err, "Failed to get path to bazel workspace"))
		return nil, cobra.ShellCompDirectiveError
	}
	buildFiles, err := findBazelBuildFiles(toComplete, workSpace)
	if err != nil {
		log.Error(errors.WithMessage(err, "Failed to find bazel build files"))
		return nil, cobra.ShellCompDirectiveError
	}

	for _, buildFile := range buildFiles {
		// Construct the absolute target label
		var labelPrefix string
		absPackageName := filepath.Dir(buildFile)
		if absPackageName == "." {
			labelPrefix = "//:"
		} else {
			labelPrefix = "//" + absPackageName + ":"
		}

		targetNames, err := findTargetsInBuildFile(buildFile)
		if err != nil {
			// Command completion is best-effort: Do not fail on errors
			log.Error(errors.WithMessagef(err, "Failed to find absolute targets in bazel build file %s", buildFile))
			continue
		}
		for _, name := range targetNames {
			res = append(res, labelPrefix+name)
		}
	}

	return res, cobra.ShellCompDirectiveNoFileComp
}

func relativeBazelFuzzTestLabels(toComplete string) ([]string, cobra.ShellCompDirective) {
	var res []string

	workDir, err := os.Getwd()
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to get path to current working directory"))
		return nil, cobra.ShellCompDirectiveError
	}
	buildFiles, err := findBazelBuildFiles(toComplete, workDir)
	if err != nil {
		log.Error(errors.WithMessage(err, "Failed to find bazel build files"))
		return nil, cobra.ShellCompDirectiveError
	}

	for _, buildFile := range buildFiles {
		targetNames, err := findTargetsInBuildFile(buildFile)
		if err != nil {
			// Command completion is best-effort: Do not fail on errors
			log.Error(errors.WithMessagef(err, "Failed to find relative targets in bazel build file %s", buildFile))
			continue
		}

		for _, name := range targetNames {
			// Construct the relative target label (that's the term used
			// by bazel for the target identifier, see
			// https://bazel.build/concepts/labels)
			var relLabel string
			relPackageName := filepath.Dir(buildFile)
			if relPackageName == "." {
				relLabel = name
			} else {
				relLabel = relPackageName + ":" + name
			}
			res = append(res, relLabel)
		}
	}

	return res, cobra.ShellCompDirectiveNoFileComp
}

func validCMakeFuzzTests(projectDir string) ([]string, cobra.ShellCompDirective) {
	matches, err := zglob.Glob(projectDir + "/.cifuzz-build/**/.cifuzz/fuzz_tests/*")
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}
	var res []string
	for _, match := range matches {
		res = append(res, filepath.Base(match))
	}
	return res, cobra.ShellCompDirectiveNoFileComp
}

// validJVMFuzzTests returns a list of valid JVM fuzz test identifiers
// (i.e. the fully qualified class name of the fuzz test)
func validJVMFuzzTests(projectDir string, toComplete string) ([]string, cobra.ShellCompDirective) {
	testDirs := []string{
		filepath.Join(projectDir, "src", "test"),
	}
	fuzzTests, err := cmdutils.ListJVMFuzzTestsByRegex(testDirs, toComplete)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return fuzzTests, cobra.ShellCompDirectiveNoFileComp
}

// validNodeFuzzTests returns a list of valid Node.js fuzz test identifiers
// (i.e. the fully qualified class name of the fuzz test)
func validNodeFuzzTests(projectDir string, toComplete string) ([]string, cobra.ShellCompDirective) {
	fuzzTests, err := cmdutils.ListNodeFuzzTestsByRegex(projectDir, toComplete)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return fuzzTests, cobra.ShellCompDirectiveNoFileComp
}

// findBazelBuildFiles returns the paths to all BUILD.bazel and BUILD files
// found in the given directory.
func findBazelBuildFiles(toComplete string, dir string) ([]string, error) {
	var buildFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}

		path, err = filepath.Rel(dir, path)
		if err != nil {
			return errors.WithStack(err)
		}

		if d.IsDir() && path != "." {
			// Skip walking the directory if it doesn't start with the
			// toComplete string
			if !strings.HasPrefix(path, strings.TrimPrefix(toComplete, "//")) {
				return fs.SkipDir
			}
			return nil
		}

		baseName := filepath.Base(path)
		if baseName == "BUILD.bazel" || baseName == "BUILD" {
			buildFiles = append(buildFiles, path)
		}
		return nil
	})
	return buildFiles, errors.WithStack(err)
}

// findTargetsInBuildFile returns all "cc_fuzz_test" targets in a given build file.
func findTargetsInBuildFile(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Read build file and remove comments and newlines, which is
	// the same the bazel bash completion script does, see:
	// https://github.com/bazelbuild/bazel/blob/021c2a053780d697899cbcbd76a032c72cd5cbbb/scripts/bazel-complete-template.bash#L166-L167
	var text string
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "#") {
			text += " " + line
		}
	}

	if !strings.Contains(text, "cc_fuzz_test") {
		return nil, nil
	}

	targetNames, found := regexutil.FindNamedGroupsMatch(bazelFuzzTestTargetPattern, text)
	if !found {
		return nil, nil
	}

	return targetNames, nil
}

// getWorkSpacePath returns the directory that includes the WORKSPACE file
// which should be the root of a bazel project.
func getWorkspacePath() (string, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return "", errors.WithStack(err)
	}

	exists, err := fileutil.Exists(filepath.Join(workDir, "WORKSPACE"))
	if err != nil {
		return "", err
	}
	if exists {
		// current working directory is already the working space root path
		return workDir, nil
	}

	for !exists {
		parentDir := filepath.Join(workDir, "..")
		exists, err = fileutil.Exists(filepath.Join(parentDir, "WORKSPACE"))
		if err != nil {
			return "", err
		}
		if exists {
			return parentDir, nil
		}
		workDir = parentDir
	}

	return "", errors.New("not able to determine the workspace")
}
