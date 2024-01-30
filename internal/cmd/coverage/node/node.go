package node

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/coverage"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/options"
	parser "code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type CoverageGenerator struct {
	OutputFormat    string
	OutputPath      string
	TestPathPattern string
	TestNamePattern string
	ProjectDir      string

	Stderr      io.Writer
	BuildStdout io.Writer
	BuildStderr io.Writer
}

func (cov *CoverageGenerator) BuildFuzzTestForCoverage() error {
	return nil
}

func (cov *CoverageGenerator) GenerateCoverageReport() (string, error) {
	// check if the specified path and name patterns have at least one match
	err := cov.validateFuzzTest()
	if err != nil {
		return "", err
	}

	if cov.OutputPath == "" {
		// default location if no output path is specified
		cov.OutputPath = filepath.Join(cov.ProjectDir, ".cifuzz-build", "coverage")
	} else {
		err := os.MkdirAll(cov.OutputPath, 0700)
		if err != nil {
			return "", errors.WithStack(err)
		}
	}

	args := []string{"jest", "--coverage"}
	args = append(args, options.JazzerJSTestPathPatternFlag(cov.TestPathPattern))
	args = append(args, options.JazzerJSTestNamePatternFlag(cov.TestNamePattern))
	args = append(args, options.JazzerJSCoverageDirectoryFlag(cov.OutputPath))
	// the lcov coverage reporter generates both the lcov.info and an html report
	args = append(args, options.JazzerJSCoverageReportersFlag(coverage.FormatLCOV))

	err = cov.runNPXCommand(args, cov.BuildStdout, cov.BuildStderr)
	if err != nil {
		return "", err
	}

	// generate the summary table
	reportPath := filepath.Join(cov.OutputPath, "lcov.info")
	reportFile, err := os.Open(reportPath)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer reportFile.Close()
	summary, err := parser.ParseLCOVReportIntoSummary(reportFile)
	if err != nil {
		return "", err
	}
	summary.PrintTable(cov.Stderr)

	// the index.html file is located in the subfolder lcov-report
	if cov.OutputFormat == "html" {
		reportPath = filepath.Join(cov.OutputPath, "lcov-report")
	}

	return reportPath, nil
}

func (cov *CoverageGenerator) validateFuzzTest() error {
	// list all fuzz tests with the specified path and name patterns
	args := []string{"jest", "--listTests"}
	args = append(args, options.JazzerJSTestPathPatternFlag(cov.TestPathPattern))
	args = append(args, options.JazzerJSTestNamePatternFlag(cov.TestNamePattern))

	stdout := new(bytes.Buffer)
	err := cov.runNPXCommand(args, stdout, stdout)
	if err != nil {
		return err
	}
	output, err := io.ReadAll(stdout)
	if err != nil {
		return errors.WithStack(err)
	}

	// check if response is empty
	if strings.TrimSpace(string(output)) == "" {
		return errors.New("No fuzz test found")
	}

	return nil
}

func (cov *CoverageGenerator) runNPXCommand(args []string, stdout, stderr io.Writer) error {
	cmd := executil.Command("npx", args...)
	cmd.Dir = cov.ProjectDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// terminate progress group if receiving exit signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	go func() {
		<-sigs
		err := cmd.TerminateProcessGroup()
		if err != nil {
			log.Error(err)
		}
	}()

	log.Debugf("Running npx command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))
	err := cmd.Run()
	if err != nil {
		// The jest test runner returns exit code 1 if not all tests
		// passed. This is expected behavior and should not be
		// treated as an error.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			log.Debugf("Jest test runner exited with exit code 1")
			return nil
		}
		return errors.WithStack(err)
	}

	return nil
}
