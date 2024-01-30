package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"

	"code-intelligence.com/cifuzz/pkg/log"
)

func main() {

	flags := pflag.NewFlagSet("updater", pflag.ExitOnError)
	deps := flags.String("dependency", "", "which dependency to update eg. gradle-plugin, jazzer, jazzerjs")
	version := flags.String("version", "", "target version to update to, for example 1.2.3")

	if err := flags.Parse(os.Args); err != nil {
		log.Error(errors.WithStack(err))
		os.Exit(1)
	}

	_, err := semver.NewVersion(*version)
	if err != nil {
		log.Error(errors.WithStack(err))
		os.Exit(1)
	}

	switch *deps {
	case "gradle-plugin":
		re := regexp.MustCompile(`("com.code-intelligence.cifuzz"\)? version ")(?P<version>\d+.\d+.\d+.*)(")`)
		paths := []string{
			"examples/gradle/build.gradle",
			"examples/gradle-kotlin/build.gradle.kts",
			"examples/gradle-multi/testsuite/build.gradle.kts",
			"pkg/messaging/instructions/gradle",
			"pkg/messaging/instructions/gradlekotlin",
			"internal/bundler/testdata/jazzer/gradle/multi-custom/testsuite/build.gradle.kts",
			"test/projects/gradle/app/build.gradle.kts",
			"test/projects/gradle/testsuite/build.gradle.kts",
		}
		for _, path := range paths {
			updateFile(path, *version, re)
		}
	case "jazzer":
		re := regexp.MustCompile(`(<artifactId>jazzer-junit<\/artifactId>\s*<version>)(?P<version>\d+.\d+.\d+.*)(<\/version>)`)
		paths := []string{
			"examples/maven/pom.xml",
			"integration-tests/errors/java/testdata/pom.xml",
			"integration-tests/errors/java/testdata-sql-ldap/pom.xml",
			"integration-tests/java-maven-spring/testdata/pom.xml",
			"internal/bundler/testdata/jazzer/maven/pom.xml",
			"pkg/messaging/instructions/maven",
			"test/projects/maven/tests/pom.xml",
			"test/projects/maven/util/pom.xml",
			"tools/list-fuzz-tests/pom.xml",
		}
		for _, path := range paths {
			updateFile(path, *version, re)
		}
	case "jazzerjs":
		updateJazzerNpm("examples/nodejs", *version)
		updateJazzerNpm("examples/nodejs-typescript", *version)

		re := regexp.MustCompile(`(@jazzer\.js\/jest-runner@)(?P<version>\d+.\d+.\d+)`)
		updateFile("pkg/messaging/instructions/nodejs", *version, re)
		updateFile("pkg/messaging/instructions/nodets", *version, re)

		re = regexp.MustCompile(`("@jazzer\.js\/jest-runner": "\^)(?P<version>\d+.\d+.\d+)(")`)
		updateFile("integration-tests/errors/nodejs/testdata/package.json", *version, re)
	default:
		log.Error(errors.New("unsupported dependency selected"))
		os.Exit(1)
	}
}

func updateJazzerNpm(path string, version string) {
	cmd := exec.Command("npm", "install", "--save-dev", fmt.Sprintf("@jazzer.js/jest-runner@%s", version))
	cmd.Dir = path
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}

func updateFile(path string, version string, re *regexp.Regexp) {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	buildFile := string(content)

	s := re.ReplaceAllString(buildFile, fmt.Sprintf(`${1}%s${3}`, version))

	err = os.WriteFile(path, []byte(s), 0x644)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	fmt.Printf("updated %s to %s\n", path, version)
}
