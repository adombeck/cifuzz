package gradle

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build/java/gradle"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/coverage"
	"code-intelligence.com/cifuzz/pkg/log"
	parser "code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

const GradleReportTask = "cifuzzReport"

type GradleRunner interface {
	RunCommand(args []string) error
}

type GradleRunnerImpl struct {
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

	Parallel gradle.ParallelOptions
	Stderr   io.Writer

	GradleRunner GradleRunner
}

func (cov *CoverageGenerator) BuildFuzzTestForCoverage() error {
	testParam := fmt.Sprintf("-Pcifuzz.fuzztest=%s", cov.FuzzTest)
	if cov.TargetMethod != "" {
		testParam += fmt.Sprintf(".%s", cov.TargetMethod)
	}
	gradleArgs := []string{testParam}

	if cov.OutputPath == "" {
		buildDir, err := gradle.GetBuildDirectory(cov.ProjectDir)
		if err != nil {
			return err
		}
		cov.OutputPath = filepath.Join(buildDir, "reports", "jacoco", GradleReportTask)
	}

	// Make sure that directory exists, otherwise the command for --format=jacocoxml will fail
	err := os.MkdirAll(cov.OutputPath, 0700)
	if err != nil {
		return errors.WithStack(err)
	}

	gradleArgs = append(gradleArgs, GradleReportTask, fmt.Sprintf("-Pcifuzz.report.output=%s", cov.OutputPath))

	if cov.OutputFormat == coverage.FormatJacocoXML {
		gradleArgs = append(gradleArgs, fmt.Sprintf("-Pcifuzz.report.format=%s", coverage.FormatJacocoXML))
	}

	return cov.GradleRunner.RunCommand(gradleArgs)
}

func (cov *CoverageGenerator) GenerateCoverageReport() (string, error) {
	reportPath := filepath.Join(cov.OutputPath, "jacoco.xml")
	reportFile, err := os.Open(reportPath)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer reportFile.Close()
	parser.ParseJacocoXMLIntoSummary(reportFile).PrintTable(cov.Stderr)

	if cov.OutputFormat == coverage.FormatJacocoXML {
		return filepath.Join(cov.OutputPath, "jacoco.xml"), nil
	}

	return filepath.Join(cov.OutputPath, "html"), nil
}

func (runner *GradleRunnerImpl) RunCommand(args []string) error {
	// ensure a finder is set
	if runner.runfilesFinder == nil {
		runner.runfilesFinder = runfiles.Finder
	}

	gradleCmd, err := gradle.GetGradleCommand(runner.ProjectDir)
	if err != nil {
		return err
	}

	cmd := executil.Command(gradleCmd, args...)
	cmd.Dir = runner.ProjectDir
	cmd.Stdout = runner.BuildStdout
	cmd.Stderr = runner.BuildStderr
	log.Debugf("Running gradle command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))

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
