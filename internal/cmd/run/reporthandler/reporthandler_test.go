package reporthandler

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/gookit/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler/metrics"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/report"
)

var (
	logOutput io.ReadWriter
)

func TestMain(m *testing.M) {
	// Disable color for this test to allow comparing strings without
	// having to add color to them
	color.Disable()

	logOutput = bytes.NewBuffer([]byte{})
	log.Output = logOutput

	m.Run()
}

func TestReportHandler_EmptyCorpus(t *testing.T) {
	testDir := testutil.ChdirToTempDir(t, "report-handler-test-")
	h, err := NewReportHandler("", &ReportHandlerOptions{ProjectDir: testDir})
	require.NoError(t, err)

	initStartedReport := &report.Report{
		Status:   report.RunStatusInitializing,
		NumSeeds: 0,
	}
	err = h.Handle(initStartedReport)
	require.NoError(t, err)
	checkOutput(t, logOutput, "Starting from an empty corpus")
	require.True(t, h.initFinished)
}

func TestReportHandler_NonEmptyCorpus(t *testing.T) {
	testDir := testutil.ChdirToTempDir(t, "report-handler-test-")
	h, err := NewReportHandler("", &ReportHandlerOptions{ProjectDir: testDir})
	require.NoError(t, err)

	initStartedReport := &report.Report{
		Status:   report.RunStatusInitializing,
		NumSeeds: 1,
	}
	err = h.Handle(initStartedReport)
	require.NoError(t, err)
	checkOutput(t, logOutput, "Initializing fuzzer with")

	initFinishedReport := &report.Report{Status: report.RunStatusRunning}
	err = h.Handle(initFinishedReport)
	require.NoError(t, err)
	checkOutput(t, logOutput, "Successfully initialized fuzzer")
}

func TestReportHandler_Metrics(t *testing.T) {
	testDir := testutil.ChdirToTempDir(t, "report-handler-test-")
	h, err := NewReportHandler("", &ReportHandlerOptions{ProjectDir: testDir})
	require.NoError(t, err)

	printerOut := bytes.NewBuffer([]byte{})
	h.printer.(*metrics.LinePrinter).BasicTextPrinter.Writer = printerOut

	metricsReport := &report.Report{
		Status: report.RunStatusRunning,
		Metric: &report.FuzzingMetric{
			Timestamp:           time.Now(),
			ExecutionsPerSecond: 1234,
			Features:            12,
		},
	}
	err = h.Handle(metricsReport)
	require.NoError(t, err)
	checkOutput(t, printerOut, metrics.MetricsToString(metricsReport.Metric))
}

func TestReportHandler_Finding(t *testing.T) {
	testDir := testutil.ChdirToTempDir(t, "report-handler-test-")
	h, err := NewReportHandler("", &ReportHandlerOptions{ProjectDir: testDir, ManagedSeedCorpusDir: "seed_corpus"})
	require.NoError(t, err)

	// create an input file
	testfile := "crash_123_test"
	err = os.WriteFile(testfile, []byte("TEST"), 0o644)
	require.NoError(t, err)

	findingReport := &report.Report{
		Status: report.RunStatusRunning,
		Finding: &finding.Finding{
			InputFile: testfile,
		},
	}
	err = h.Handle(findingReport)
	require.NoError(t, err)

	expectedOutputs := []string{findingReport.Finding.Name}
	checkOutput(t, logOutput, expectedOutputs...)
}

func TestReportHandler_CorpusDirs(t *testing.T) {
	h, err := NewReportHandler("", &ReportHandlerOptions{})
	require.NoError(t, err)

	seedCorpusDir := "/seed/corpus/dir"
	seedCorpusReport := &report.Report{
		SeedCorpus: seedCorpusDir,
	}
	generatedCorpusDir := "/generated/corpus/dir"
	generatedCorpusReport := &report.Report{
		GeneratedCorpus: generatedCorpusDir,
	}

	err = h.Handle(seedCorpusReport)
	require.NoError(t, err)
	assert.Equal(t, seedCorpusDir, h.ManagedSeedCorpusDir)

	err = h.Handle(generatedCorpusReport)
	require.NoError(t, err)
	assert.Equal(t, generatedCorpusDir, h.GeneratedCorpusDir)
}

func TestReportHandler_PrintJSON(t *testing.T) {
	testDir := testutil.ChdirToTempDir(t, "report-handler-test-")
	jsonOut := bytes.NewBuffer([]byte{})
	h, err := NewReportHandler("", &ReportHandlerOptions{
		ProjectDir: testDir,
		JSONOutput: jsonOut,
	})
	require.NoError(t, err)

	findingLogs := []string{"Oops", "The program crashed"}
	findingReport := &report.Report{
		Status: report.RunStatusRunning,
		Finding: &finding.Finding{
			Logs: findingLogs,
		},
	}
	err = h.Handle(findingReport)
	require.NoError(t, err)
	checkOutput(t, jsonOut, findingLogs...)
}

func TestReportHandler_GenerateName(t *testing.T) {
	testDir := testutil.ChdirToTempDir(t, "report-handler-test-")
	h, err := NewReportHandler("", &ReportHandlerOptions{ProjectDir: testDir})
	require.NoError(t, err)

	findingLogs := []string{"Oops", "The program crashed"}
	findingReport := &report.Report{
		Status: report.RunStatusRunning,
		Finding: &finding.Finding{
			Logs:      findingLogs,
			InputData: []byte("123"),
		},
	}
	err = h.Handle(findingReport)
	require.NoError(t, err)
	assert.Equal(t, "adventurous_pangolin", findingReport.Finding.Name)
}

func checkOutput(t *testing.T, r io.Reader, s ...string) {
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	for _, str := range s {
		require.Contains(t, string(output), str)
	}
}
