package cmdutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/regexutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

var jazzerFuzzTestRegex = regexp.MustCompile(`@FuzzTest|\sfuzzerTestOneInput\s*\(`)

func JazzerSeedCorpus(targetClass string, projectDir string) string {
	seedCorpus := targetClass + "Inputs"
	path := strings.Split(seedCorpus, ".")
	path = append([]string{"src", "test", "resources"}, path...)

	return filepath.Join(projectDir, filepath.Join(path...))
}

// GetTargetMethodsFromJVMFuzzTestFile returns a list of target methods from
// a given fuzz test file.
func GetTargetMethodsFromJVMFuzzTestFile(path string) ([]string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var targetMethods []string

	// Regular expression pattern to match @FuzzTest and @FuzzTest() annotations
	fuzzTestRegex := regexp.MustCompile(`@FuzzTest(\((?P<parameter>.[^\)]*)\))*\s+(?P<prefix>\w*\s)*(?P<targetName>\w+)\s*\(`)
	matches, _ := regexutil.FindAllNamedGroupsMatches(fuzzTestRegex, string(bytes))

	// Extract the function targetName from each match and append it to the
	// targetMethods slice
	for _, match := range matches {
		targetMethods = append(targetMethods, match["targetName"])
	}

	// Check if the file contains a fuzzerTestOneInput method
	// and append it to the targetMethods slice if it does
	fuzzerTestOneInputRegex := regexp.MustCompile(`\sfuzzerTestOneInput\s*\(`)
	if len(fuzzerTestOneInputRegex.FindAllStringSubmatch(string(bytes), -1)) > 0 {
		targetMethods = append(targetMethods, "fuzzerTestOneInput")
	}

	return targetMethods, nil
}

// ConstructJVMFuzzTestIdentifier constructs a fully qualified class name for a
// given fuzz test file from the directory the file is in and the file name.
func ConstructJVMFuzzTestIdentifier(path, testDir string) (string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", errors.WithStack(err)
	}

	match := jazzerFuzzTestRegex.MatchString(string(bytes))
	if match {

		classFilePath, err := filepath.Rel(testDir, path)
		if err != nil {
			return "", errors.WithStack(err)
		}

		className := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

		fuzzTestIdentifier := filepath.Join(
			filepath.Dir(classFilePath),
			className,
		)
		fuzzTestIdentifier = strings.ReplaceAll(fuzzTestIdentifier, string(os.PathSeparator), ".")
		// remove language specific paths from identifier for example src/test/(java|kotlin)
		fuzzTestIdentifier = strings.TrimPrefix(fuzzTestIdentifier, "java.")
		fuzzTestIdentifier = strings.TrimPrefix(fuzzTestIdentifier, "kotlin.")

		return fuzzTestIdentifier, nil
	}

	return "", nil
}

// ListJVMFuzzTestsByRegex returns a list of all fuzz tests inside
// the given directories.
// The returned list contains the fully qualified class name of the fuzz test.
// to filter files based on the fqcn you can use the prefix filter parameter
func ListJVMFuzzTestsByRegex(testDirs []string, prefixFilter string) ([]string, error) {
	var fuzzTests []string
	for _, testDir := range testDirs {
		exists, err := fileutil.Exists(testDir)
		if err != nil {
			return nil, err
		}
		// skip non-existing directories
		if !exists {
			continue
		}

		// use zglob to support globbing in windows
		matches, err := zglob.Glob(filepath.Join(testDir, "**", "*.{java,kt}"))
		if err != nil {
			return nil, errors.WithStack(err)
		}

		for _, match := range matches {
			// Get the target methods from the fuzz test file
			methods, err := GetTargetMethodsFromJVMFuzzTestFile(match)
			if err != nil {
				return nil, err
			}

			// add the fuzz test identifier to the fuzzTests slice
			for _, method := range methods {
				fuzzTestIdentifier, err := ConstructJVMFuzzTestIdentifier(match, testDir)
				if err != nil {
					return nil, err
				}

				fuzzTestIdentifier = fuzzTestIdentifier + "::" + method
				if fuzzTestIdentifier != "" && (prefixFilter == "" || strings.HasPrefix(fuzzTestIdentifier, prefixFilter)) {
					// add the method name to the identifier
					fuzzTests = append(fuzzTests, fuzzTestIdentifier)
				}
			}
		}
	}
	return fuzzTests, nil
}

// SeparateTargetClassAndMethod splits up the given fuzz test into target class
// and method if it follows the pattern <class>::<method>. If it doesn't follow
// the pattern, it will return the given string and an empty string.
func SeparateTargetClassAndMethod(fuzzTest string) (string, string) {
	if !strings.Contains(fuzzTest, "::") {
		return fuzzTest, ""
	}

	split := strings.Split(fuzzTest, "::")
	return split[0], split[1]
}

// ListJVMFuzzTests gathers all fuzz tests using the list-fuzz-tests tool.
func ListJVMFuzzTests(classNames []string, runtimeDeps []string) ([]string, error) {
	listFuzzTestsJar, err := runfiles.Finder.ListFuzzTestsJarPath()
	if err != nil {
		return nil, err
	}
	classPath := strings.Join(append(runtimeDeps, listFuzzTestsJar), string(os.PathListSeparator))

	args := []string{
		"-cp",
		classPath,
		"com.code_intelligence.cifuzz.helper.ListFuzzTests",
	}
	if len(classNames) > 0 {
		args = append(args, strings.Join(classNames, " "))
	}

	javaBin, err := runfiles.Finder.JavaPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(javaBin, args...)
	cmd.Stderr = os.Stderr
	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	if err != nil {
		return nil, WrapExecError(errors.WithStack(err), cmd)
	}

	fuzzTests := strings.Split(strings.TrimSpace(string(output)), "\n")

	return fuzzTests, nil
}

// ValidateJVMFuzzTest checks if the given fuzz test is valid.
// If no target method is specified, it will be added.
func ValidateJVMFuzzTest(fuzzTest string, targetMethod *string, deps []string) error {
	allValidFuzzTests, err := ListJVMFuzzTests(nil, deps)
	if err != nil {
		return err
	}

	var fuzzTestsInTargetClass []string
	for _, validFuzzTest := range allValidFuzzTests {
		targetClass, _ := SeparateTargetClassAndMethod(validFuzzTest)
		if targetClass == fuzzTest {
			fuzzTestsInTargetClass = append(fuzzTestsInTargetClass, validFuzzTest)
		}
	}

	if len(fuzzTestsInTargetClass) == 0 {
		return WrapIncorrectUsageError(errors.Errorf("No valid fuzz tests found in %s", fuzzTest))
	}

	if *targetMethod == "" {
		if len(fuzzTestsInTargetClass) > 1 {
			return WrapIncorrectUsageError(errors.Errorf("Multiple fuzz tests found in %s", fuzzTest))
		}
		_, *targetMethod = SeparateTargetClassAndMethod(fuzzTestsInTargetClass[0])
	}

	if !sliceutil.Contains(fuzzTestsInTargetClass, fmt.Sprintf("%s::%s", fuzzTest, *targetMethod)) {
		return WrapIncorrectUsageError(errors.Errorf("%s::%s is not a valid fuzz test", fuzzTest, *targetMethod))
	}

	return nil
}
