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
	"code-intelligence.com/cifuzz/pkg/options"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/regexutil"
)

func ListNodeFuzzTestsByRegex(projectDir string, prefixFilter string) ([]string, error) {
	// use zglob to support globbing in windows
	fuzzTestFiles, err := zglob.Glob(filepath.Join(projectDir, "**", "*.fuzz.*"))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var fuzzTests []string
	for _, testFile := range fuzzTestFiles {
		methods, err := getTargetMethodsFromNodeTestFile(testFile)
		if err != nil {
			return nil, err
		}

		fuzzTest := filepath.Base(testFile)
		if strings.HasSuffix(fuzzTest, ".ts") {
			fuzzTest = strings.TrimSuffix(fuzzTest, ".fuzz.ts")
		} else {
			fuzzTest = strings.TrimSuffix(fuzzTest, ".fuzz.js")
		}
		if len(methods) == 1 {
			if prefixFilter == "" || strings.HasPrefix(testFile, prefixFilter) {
				fuzzTests = append(fuzzTests, fuzzTest)
			}
			continue
		}

		for _, method := range methods {
			fuzzTestIdentifier := fuzzTest + ":" + fmt.Sprintf("%q", method)
			if prefixFilter == "" || strings.HasPrefix(fuzzTestIdentifier, prefixFilter) {
				fuzzTests = append(fuzzTests, fuzzTestIdentifier)
			}
		}
	}

	return fuzzTests, nil
}

func ValidateNodeFuzzTest(projectDir string, testPathPattern string, testNamePattern string) error {
	var env []string
	// enable "list fuzz tests" mode for jazzer.js
	env, err := envutil.Setenv(env, "JAZZER_LIST_FUZZTEST_NAMES", "1")
	if err != nil {
		return err
	}
	// pass test name pattern to jazzer.js
	env, err = envutil.Setenv(env, "JAZZER_LIST_FUZZTEST_NAMES_PATTERN", testNamePattern)
	if err != nil {
		return err
	}

	args := []string{"jest"}
	// pass test path pattern to jest
	args = append(args, options.JazzerJSTestPathPatternFlag(testPathPattern))
	// use a test name pattern, which is not matched by any fuzz test
	args = append(args, options.JazzerJSTestNamePatternFlag("list-fuzz-tests"))
	// disable jest reporters to prevent unnecessary output
	args = append(args, options.JazzerJSReportersFlag(""))

	cmd := exec.Command("npx", args...)
	cmd.Dir = projectDir
	cmd.Env, err = envutil.Copy(os.Environ(), env)
	if err != nil {
		return err
	}
	log.Debugf("Command: %s", envutil.QuotedCommandWithEnv(cmd.Args, env))

	bytes, err := cmd.Output()
	output := strings.TrimSpace(string(bytes))
	if err != nil {
		// in case of invalid testPathPattern jest exists with exit code 1
		// and outputs an error message containing "No tests found"
		if strings.Contains(output, "No tests found") {
			return WrapIncorrectUsageError(
				errors.Errorf("No valid fuzz test found for %s:%s", testPathPattern, testNamePattern),
			)
		}
		return WrapExecError(errors.WithStack(err), cmd)
	}

	if len(output) == 0 {
		return WrapIncorrectUsageError(
			errors.Errorf("No valid fuzz test found for %s:%s", testPathPattern, testNamePattern),
		)
	}

	fuzzTests := strings.Split(output, "\n")
	if len(fuzzTests) > 1 {
		return WrapIncorrectUsageError(
			errors.Errorf("Multiple fuzz tests found for %s:%s", testPathPattern, testNamePattern),
		)
	}

	return nil
}

func getTargetMethodsFromNodeTestFile(path string) ([]string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var targetMethods []string
	fuzzTestRegex := regexp.MustCompile(`\.fuzz\s*\(\s*"(?P<fuzzTest>(\w|\s|\d)+)`)
	matches, _ := regexutil.FindAllNamedGroupsMatches(fuzzTestRegex, string(bytes))
	for _, match := range matches {
		targetMethods = append(targetMethods, match["fuzzTest"])
	}

	return targetMethods, nil
}
