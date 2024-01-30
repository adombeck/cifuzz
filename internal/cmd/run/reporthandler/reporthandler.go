package reporthandler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gookit/color"
	"github.com/hokaccha/go-prettyjson"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler/metrics"
	"code-intelligence.com/cifuzz/internal/names"
	"code-intelligence.com/cifuzz/pkg/desktop"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/pkg/report"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type ReportHandlerOptions struct {
	ProjectDir           string
	GeneratedCorpusDir   string
	ManagedSeedCorpusDir string
	UserSeedCorpusDirs   []string
	JSONOutput           io.Writer
	PrinterOutput        io.Writer
	SkipSavingFinding    bool
}

type ReportHandler struct {
	*ReportHandlerOptions
	usingUpdatingPrinter bool

	printer      metrics.Printer
	startedAt    time.Time
	initStarted  bool
	initFinished bool

	LastMetrics  *report.FuzzingMetric
	FirstMetrics *report.FuzzingMetric
	ErrorDetails []*finding.ErrorDetails

	numSeedsAtInit uint

	FuzzTest string
	Findings []*finding.Finding
}

func NewReportHandler(fuzzTest string, options *ReportHandlerOptions) (*ReportHandler, error) {
	var err error
	h := &ReportHandler{
		ReportHandlerOptions: options,
		startedAt:            time.Now(),
		FuzzTest:             fuzzTest,
	}

	if options.JSONOutput == nil {
		h.JSONOutput = io.Discard
	}
	if options.PrinterOutput == nil {
		h.PrinterOutput = io.Discard
	}

	// Use an updating printer if the output stream is a TTY
	// and plain style is not enabled

	if file, ok := h.PrinterOutput.(*os.File); ok && term.IsTerminal(int(file.Fd())) && !log.PlainStyle() {
		h.printer, err = metrics.NewUpdatingPrinter(h.PrinterOutput)
		if err != nil {
			return nil, err
		}
		h.usingUpdatingPrinter = true
	} else {
		h.printer = metrics.NewLinePrinter(h.PrinterOutput)
	}

	return h, nil
}

func (h *ReportHandler) Handle(r *report.Report) error {
	var err error

	if r.SeedCorpus != "" || r.GeneratedCorpus != "" {
		// This report was only sent to update the seed corpus directory path
		// which we use later to count the number of seeds. We don't want to
		// print the report, so we return early.
		if r.SeedCorpus != "" {
			h.ManagedSeedCorpusDir = r.SeedCorpus
		}
		if r.GeneratedCorpus != "" {
			h.GeneratedCorpusDir = r.GeneratedCorpus
		}
		return nil
	}

	if r.Status == report.RunStatusInitializing && !h.initStarted {
		h.initStarted = true
		h.numSeedsAtInit = r.NumSeeds
		if r.NumSeeds == 0 {
			log.Info("Starting from an empty corpus")
			h.initFinished = true
		} else {
			log.Info("Initializing fuzzer with ", pterm.FgLightCyan.Sprintf("%d", r.NumSeeds), " seed inputs")
		}

		// Start the updating printer to show metrics while the fuzzer runs
		// the seeds
		h.printer.Start()
	}

	if r.Status == report.RunStatusRunning && !h.initFinished {
		log.Info("Successfully initialized fuzzer with seed inputs")
		h.initFinished = true

		// Ensure that the updating printer is started. It should already
		// have been started above during initialization, but we do it
		// again here in case no INITIALIZING report was received.
		// In case the printer is already started, this is a no-op.
		h.printer.Start()
	}

	if r.Metric != nil {
		h.LastMetrics = r.Metric
		if h.FirstMetrics == nil {
			h.FirstMetrics = r.Metric
		}
		h.printer.PrintMetrics(r.Metric)
	}

	if r.Finding != nil {
		// save finding
		h.Findings = append(h.Findings, r.Finding)

		if len(h.Findings) == 1 {
			h.PrintFindingInstruction()
		}

		err = h.handleFinding(r.Finding)
		if err != nil {
			return err
		}
	}

	if h.JSONOutput != io.Discard && h.usingUpdatingPrinter {
		// Clear the updating printer
		h.printer.(*metrics.UpdatingPrinter).Clear()
	}

	err = h.writeJSONReport(r)
	if err != nil {
		return err
	}

	return nil
}

func (h *ReportHandler) writeJSONReport(r *report.Report) error {
	var jsonString string
	var err error
	// Print with color if the output stream is a TTY
	if file, ok := h.JSONOutput.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		bytes, err := prettyjson.Marshal(r)
		if err != nil {
			return errors.WithStack(err)
		}
		jsonString = string(bytes)
	} else {
		jsonString, err = stringutil.ToJSONString(r)
		if err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(h.JSONOutput, jsonString)
	return nil
}

func (h *ReportHandler) handleFinding(f *finding.Finding) error {
	var err error

	f.CreatedAt = time.Now()

	// Generate a name for the finding. The name is chosen deterministically,
	// based on:
	// * Parts of the stack trace: The function name, source file name,
	//   line and column of those stack frames which are located in user
	//   or library code, i.e. everything above the call to
	//   LLVMFuzzerTestOneInputNoReturn or LLVMFuzzerTestOneInput.
	// * The crashing input.
	//
	// This automatically provides some very basic deduplication:
	// Crashes which were triggered by the same line in the user code
	// and with the same crashing input result in the same name, which
	// means that a previous finding of the same name gets overwritten.
	// So when executing the same fuzz test twice, we don't have
	// duplicate findings, because the same crashing input is used from
	// the seed corpus (unless the user deliberately removed it), which
	// results in the same crash and a finding of the same name.
	//
	// By including the crashing input, we also generate a new finding
	// in the scenario that, after a crash was found, the code was fixed
	// and therefore the old crashing input does not trigger the crash
	// anymore, but in a subsequent run the fuzzer finds a different
	// crashing input which causes the crash again. We do want to
	// produce a distinct new finding in that case.
	nameSeed := append(stacktrace.EncodeStackTrace(f.StackTrace), f.InputData...)
	f.Name = names.GetDeterministicName(nameSeed)

	if f.InputFile != "" && !h.SkipSavingFinding {
		if h.ManagedSeedCorpusDir == "" {
			// Handle the case that the seed corpus directory was not set. In
			// the case of Java fuzz tests, the seed corpus directory is
			// printed by Jazzer. We parse that output and send it to the
			// report handler via a report with an empty finding. If we did
			// not receive that report yet, we cannot copy the input file to
			// the seed corpus directory.
			return errors.New("finding before seed corpus directory was set")
		}
		err = f.CopyInputFileAndUpdateFinding(h.ProjectDir, h.ManagedSeedCorpusDir)
		if err != nil {
			return err
		}
	}

	f.FuzzTest = h.FuzzTest

	// Do not mutate f after this call.
	if !h.SkipSavingFinding {
		err = f.Save(h.ProjectDir)
		if err != nil {
			return err
		}
	}

	log.Finding(f.ShortDescriptionWithName())

	desktop.Notify("cifuzz finding", f.ShortDescriptionWithName())

	return nil
}

func (h *ReportHandler) PrintFindingInstruction() {
	log.Note(`
Use 'cifuzz finding <finding name>' for details on a finding.

`)
}

func (h *ReportHandler) PrintCrashingInputNote() {
	var crashingInputs []string

	for _, f := range h.Findings {
		if f.GetSeedPath() != "" {
			crashingInputs = append(crashingInputs, fileutil.PrettifyPath(f.GetSeedPath()))
		}
	}

	if len(crashingInputs) == 0 {
		return
	}

	log.Notef(`
Note: The reproducing inputs have been copied to the seed corpus at:

    %s

They will now be used as a seed input for all runs of the fuzz test,
including remote runs with artifacts created via 'cifuzz bundle' and
regression tests. For more information on regression tests, see:

    https://github.com/CodeIntelligenceTesting/cifuzz/blob/main/docs/Regression-Testing.md
`, strings.Join(crashingInputs, "\n    "))
}

func (h *ReportHandler) PrintFinalMetrics() error {
	// We don't want to print colors to stderr unless it's a TTY
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		color.Disable()
	}

	if h.usingUpdatingPrinter {
		// Stop the updating printer
		updatingPrinter := h.printer.(*metrics.UpdatingPrinter)
		err := updatingPrinter.Stop()
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		// Stopping the updating printer leaves an empty line, which
		// we actually want before the final metrics (because it looks
		// better), so in case we did not use an updating printer,
		// print an empty line anyway.
		log.Print("\n")
	}

	numCorpusEntries, err := h.countCorpusEntries()
	if err != nil {
		return err
	}

	duration := time.Since(h.startedAt)
	newCorpusEntries := numCorpusEntries - h.numSeedsAtInit

	// If the number of new corpus entries exceeds the total corpus entries, it
	// indicates an unexpected scenario where the total corpus entries are zero
	// (e.g., when running with `--engine-arg=-runs=10`) and cifuzz discovers new
	// seeds during subsequent runs. To avoid any issues related to unsigned
	// integers, we set the new corpus entries to 0 in such cases.
	if newCorpusEntries > numCorpusEntries {
		newCorpusEntries = 0
	}

	var averageExecsStr string

	if h.FirstMetrics == nil {
		averageExecsStr = metrics.NumberString("n/a")
	} else {
		var averageExecs uint64
		metricsDuration := h.LastMetrics.Timestamp.Sub(h.FirstMetrics.Timestamp)
		if metricsDuration.Milliseconds() == 0 {
			// The first and last metrics are either the same or were
			// printed too fast one after the other to calculate a
			// meaningful average, so we just use the exec/s from the
			// current metrics as the average.
			averageExecs = uint64(h.LastMetrics.ExecutionsPerSecond)
		} else {
			// We use milliseconds here to calculate a more accurate average
			execs := h.LastMetrics.TotalExecutions - h.FirstMetrics.TotalExecutions
			averageExecs = uint64(float64(execs) / (float64(metricsDuration.Milliseconds()) / 1000))
		}
		if averageExecs > 0 {
			averageExecsStr = metrics.NumberString("%d", averageExecs)
		} else {
			averageExecsStr = metrics.NumberString("n/a")
		}
	}

	// Round towards the next larger second to avoid that very short
	// runs show "Ran for 0s".
	durationStr := (duration.Truncate(time.Second) + time.Second).String()

	lines := []string{
		metrics.DescString("Execution time:\t") + metrics.NumberString(durationStr),
		metrics.DescString("Average exec/s:\t") + averageExecsStr,
		metrics.DescString("Findings:\t") + metrics.NumberString("%d", len(h.Findings)),
		metrics.DescString("Corpus entries:\t") + metrics.NumberString("%d", numCorpusEntries) +
			metrics.DescString(" (+%s)", metrics.NumberString("%d", newCorpusEntries)),
	}

	w := tabwriter.NewWriter(log.NewPTermWriter(os.Stderr), 0, 0, 1, ' ', 0)
	for _, line := range lines {
		_, err = fmt.Fprintln(w, line)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	err = w.Flush()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *ReportHandler) countCorpusEntries() (uint, error) {
	var numSeeds uint
	seedCorpusDirs := append(h.UserSeedCorpusDirs, h.ManagedSeedCorpusDir, h.GeneratedCorpusDir)

	for _, dir := range seedCorpusDirs {
		var seedsInDir uint
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return errors.WithStack(err)
			}
			// Don't count empty files, same as libFuzzer
			if info.Size() != 0 {
				seedsInDir += 1
			}
			return nil
		})
		// Don't fail if the seed corpus dir doesn't exist
		if os.IsNotExist(err) {
			return 0, nil
		}
		if err != nil {
			return 0, errors.WithStack(err)
		}
		numSeeds += seedsInDir
	}
	return numSeeds, nil
}
