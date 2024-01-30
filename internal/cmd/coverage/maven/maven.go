package maven

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build/java/maven"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/coverage"
	"code-intelligence.com/cifuzz/pkg/log"
	parser "code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type MavenRunner interface {
	RunCommand(args []string) error
}

type MavenRunnerImpl struct {
	ProjectDir  string
	BuildStdout io.Writer
	BuildStderr io.Writer

	runfilesFinder runfiles.RunfilesFinder
}

type CoverageGenerator struct {
	OutputFormat string
	OutputPath   string
	FuzzTest     string
	TargetMethod string
	ProjectDir   string

	Parallel maven.ParallelOptions
	Stderr   io.Writer

	MavenRunner MavenRunner
}

func (cov *CoverageGenerator) BuildFuzzTestForCoverage() error {
	// Maven tests fail if fuzz tests fail, so we ignore the error here,
	// so we can still generate the coverage report
	mavenTestArgs := []string{"-Dmaven.test.failure.ignore=true"}

	// Flags for jazzer
	mavenTestArgs = append(mavenTestArgs, "-Djazzer.hooks=false")

	// Flags for cifuzz
	testParam := fmt.Sprintf("-Dtest=%s", cov.FuzzTest)
	if cov.TargetMethod != "" {
		testParam += fmt.Sprintf("#%s", cov.TargetMethod)
	}
	mavenTestArgs = append(mavenTestArgs,
		"-Pcifuzz",
		testParam,
		"test")

	if cov.Parallel.Enabled {
		mavenTestArgs = append(mavenTestArgs, "-T")
		if cov.Parallel.NumJobs != 0 {
			mavenTestArgs = append(mavenTestArgs, fmt.Sprint(cov.Parallel.NumJobs))
		} else {
			// Use one thread per cpu core
			mavenTestArgs = append(mavenTestArgs, "1C")
		}
	}
	err := cov.MavenRunner.RunCommand(mavenTestArgs)
	if err != nil {
		return err
	}

	if cov.OutputPath == "" {
		// We are using the .cifuzz-build directory
		// because the build directory is unknown at this point
		cov.OutputPath = filepath.Join(cov.ProjectDir, ".cifuzz-build", "report")
	}
	mavenReportArgs := []string{
		"-Pcifuzz",
		"jacoco:report",
		fmt.Sprintf("-Dcifuzz.report.output=%s", cov.OutputPath),
	}

	if cov.OutputFormat == coverage.FormatJacocoXML {
		mavenReportArgs = append(mavenReportArgs, "-Dcifuzz.report.format=XML")
	} else {
		mavenReportArgs = append(mavenReportArgs, "-Dcifuzz.report.format=XML,HTML")
	}

	return cov.MavenRunner.RunCommand(mavenReportArgs)
}

func (cov *CoverageGenerator) GenerateCoverageReport() (string, error) {
	reportPath := filepath.Join(cov.OutputPath, "jacoco.xml")
	reportFile, err := os.Open(reportPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Catch case where no jacoco.xml was produced, most likely caused by a faulty jacoco configuration
			notExistErr := fmt.Errorf(`JaCoCo did not create a coverage report (jacoco.xml).
Please check if you configured JaCoCo correctly in your project.

If you use the maven-surefire-plugin: 
Be aware that adding additional arguments in the plugin configuration in your 
pom.xml can disrupt the JaCoCo execution and have to be prefixed with '@{argline}'.

<argLine>@{argLine} -your -extra -arguments</argLine>
`)
			return "", notExistErr
		}
		return "", errors.WithStack(err)
	}
	defer reportFile.Close()
	parser.ParseJacocoXMLIntoSummary(reportFile).PrintTable(cov.Stderr)

	if cov.OutputFormat == coverage.FormatJacocoXML {
		return filepath.Join(cov.OutputPath, "jacoco.xml"), nil
	}

	return cov.OutputPath, nil
}

func (runner *MavenRunnerImpl) RunCommand(args []string) error {
	// ensure a finder is set
	if runner.runfilesFinder == nil {
		runner.runfilesFinder = runfiles.Finder
	}

	mavenCmd, err := runner.runfilesFinder.MavenPath()
	if err != nil {
		return err
	}

	cmdArgs := []string{mavenCmd}
	cmdArgs = append(cmdArgs, args...)

	cmd := executil.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = runner.ProjectDir
	cmd.Stdout = runner.BuildStdout
	cmd.Stderr = runner.BuildStderr
	log.Debugf("Running maven command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	go func() {
		<-sigs
		err = cmd.TerminateProcessGroup()
		if err != nil {
			log.Error(err)
		}
	}()

	err = cmd.Run()
	if err != nil {
		return cmdutils.WrapExecError(errors.WithStack(err), cmd.Cmd)
	}
	return nil
}
