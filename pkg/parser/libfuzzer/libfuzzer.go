package libfuzzer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"

	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/java/sourcemap"
	"code-intelligence.com/cifuzz/pkg/minijail"
	"code-intelligence.com/cifuzz/pkg/parser/errorid"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/pkg/parser/sanitizer"
	"code-intelligence.com/cifuzz/pkg/report"
	"code-intelligence.com/cifuzz/util/regexutil"
)

var (
	nonEmptyCorpusPattern = regexp.MustCompile(
		`INFO: seed corpus: files: (?P<num_seeds>\d+) min: (?P<min_size>\w+) max: (?P<max_size>\w+) total: (?P<total_size>\w+) rss: (?P<peak_rss>\w+)`,
	)
	emptyCorpusPattern = regexp.MustCompile(
		`INFO: A corpus is not provided, starting from an empty corpus`,
	)

	libfuzzerTimeoutErrorPattern = regexp.MustCompile(
		`ALARM: working on the last Unit for (?P<timeout_seconds>\d+) seconds`,
	)
	libfuzzerErrorPattern = regexp.MustCompile(
		`==\d+== ERROR: libFuzzer: (?P<error_type>.+)`)

	jazzerSecurityIssuePattern = regexp.MustCompile(
		`== Java Exception:.*com.code_intelligence.jazzer.api.FuzzerSecurityIssue(?P<type>Low|Medium|High|Critical):\s*(?P<message>.*)$`)
	javaExceptionErrorPattern = regexp.MustCompile(
		`== Java Exception:\s*(?P<error_type>.*)`)
	javaAssertionErrorPattern = regexp.MustCompile(
		`== Java Assertion Error`)
	jazzerInputPathPatter = regexp.MustCompile(`INFO: using inputs from: (?P<input_path>.*)$`)

	jazzerJSExceptionErrorPattern = regexp.MustCompile(
		`==\d+== Uncaught Exception: (?P<message>.+)`,
	)
	jazzerJSCommandInjectionPattern = regexp.MustCompile(
		`==\d+== Command Injection`,
	)
	jazzerJSPathTraveralPattern = regexp.MustCompile(
		`==\d+== Path Traversal`,
	)
	jazzerJSPrototypePollutionPattern = regexp.MustCompile(
		`==\d+== Prototype Pollution`,
	)
	beginningOfJestReportPattern = regexp.MustCompile(
		`FAIL Jazzer\.js`,
	)

	// Examples for matching strings:
	// #2	INITED cov: 10 ft: 11 corp: 1/1b exec/s: 0 rss: 30Mb
	// #670	REDUCE cov: 13 ft: 15 corp: 4/5b lim: 8 exec/s: 0 rss: 31Mb L: 1/2 MS: 2 CopyPart-EraseBytes-
	statsPattern = regexp.MustCompile(
		`#(?P<total_execs>\d+)\s+(?P<status>\S*)\s+(cov:\s+(?P<edges>\d+)\s+)?ft:\s+(?P<features>\d+)\s+corp:\s+(?P<corpus_size>\d+)/.*exec/s:\s+(?P<executions_per_second>\d+)\s+`)
	testInputFilePattern = regexp.MustCompile(
		`Test unit written to\s*(?P<test_input_file>.*)`)
	slowInputPattern = regexp.MustCompile(
		`\s*Slowest unit: (?P<duration>\d+) s.*`)
	goPanicPattern = regexp.MustCompile(`^panic:\s+\S+`)
)

var errNotFound = errors.New("not found")

type parser struct {
	*Options

	FindingReported bool

	reportsCh chan *report.Report

	// Whether we parsed the message which indicates that libFuzzer
	// started the initialization
	initStarted bool
	// Whether we parsed the message which indicates that libFuzzer
	// finished the initialization
	initFinished bool

	// A finding that was found in the libfuzzer output but wasn't sent
	// yet, because we keep reading more output lines for some time and
	// attach them to the finding if they seem to belong to it
	pendingFinding                       *finding.Finding
	numMetricsLinesSinceFindingIsPending int

	foundBeginningOfJestReport bool

	lastNewFeatureTime time.Time // Timestamp representing the point when the last new feature was reported
	lastFeatures       int       // Last features reported by Libfuzzer
	lastNewEdgeTime    time.Time // Timestamp representing the point when the last new edge was reported
	lastEdges          int       // Last edges reported by Libfuzzer
}

type Options struct {
	SupportJazzer   bool
	SupportJazzerJS bool
	KeepColor       bool
	// The parser writes all parsed lines to StartupOutputWriter up to
	// the point where the fuzzer has completed initialization.
	StartupOutputWriter io.Writer
	// The directory to which paths in the stack trace are made relative to
	ProjectDir string
	SourceMap  *sourcemap.SourceMap
}

func NewLibfuzzerOutputParser(options *Options) *parser {
	if options == nil {
		options = &Options{}
	}
	return &parser{Options: options}
}

func (p *parser) Parse(ctx context.Context, input io.Reader, reportsCh chan *report.Report) error {
	p.reportsCh = reportsCh
	defer close(p.reportsCh)
	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		err := p.parseLine(ctx, scanner.Text())
		if err != nil {
			return err
		}
	}

	// The fuzzer output was closed, which means that the fuzzer exited.
	// If there is still a pending finding, send it now.
	err := p.finalizeAndSendPendingFindingIfAny(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (p *parser) sendReport(ctx context.Context, report *report.Report) error {
	select {
	case p.reportsCh <- report:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *parser) parseLine(ctx context.Context, line string) error {
	if !p.KeepColor {
		// Sanitizer reports can be colorized, but the ANSI escapes used
		// for colors should not be included in logs.
		line = pterm.RemoveColorFromString(line)
	}

	if !p.initStarted && !p.initFinished {
		if p.SupportJazzer || p.SupportJazzerJS {
			// Try to parse jazzer input path messages in order to
			// set generated and seed corpus paths for the report
			// handler.
			inputPath, found := parseAsInputPathMessage(line)
			if found {
				if strings.Contains(inputPath, filepath.Join(p.ProjectDir, ".cifuzz-corpus")) {
					return p.sendReport(ctx, &report.Report{
						GeneratedCorpus: inputPath,
					})
				} else if strings.Contains(inputPath, p.ProjectDir) {
					return p.sendReport(ctx, &report.Report{
						SeedCorpus: inputPath,
					})
				}
			}
		}
		// Try to parse the line as the libFuzzer message which tells us
		// how many seeds the corpus contains.
		// Note: We used to stop parsing lines until we saw the first
		// seed corpus message, but that caused no finding report to be
		// be sent when the fuzz target already crashes on the empty
		// input, because in that case libFuzzer doesn't print the seed
		// corpus message.
		numSeeds, err := parseAsSeedCorpusMessage(line)
		if err != nil {
			if !errors.Is(err, errNotFound) {
				return err
			}

			if p.StartupOutputWriter != nil {
				// Store all lines printed before the fuzzer has been initialized
				// so that they can be printed in case of a startup error (e.g.
				// a missing shared library dependency).
				_, err := p.StartupOutputWriter.Write(append([]byte(line), '\n'))
				if err != nil {
					return errors.WithStack(err)
				}
			}
		} else {
			p.initStarted = true

			if numSeeds == 0 {
				p.initFinished = true
			}

			// Report that the fuzzer is now initializing and how many seeds
			// are used for initialization
			return p.sendReport(ctx, &report.Report{
				Status:   report.RunStatusInitializing,
				NumSeeds: numSeeds,
			})
		}
	}

	metric := p.parseAsFuzzingMetric(line)
	if metric != nil {
		r := &report.Report{Metric: metric}
		if p.initFinished {
			r.Status = report.RunStatusRunning
		} else {
			r.Status = report.RunStatusInitializing
		}
		err := p.sendReport(ctx, r)
		if err != nil {
			return err
		}

		if p.pendingFinding != nil {
			p.numMetricsLinesSinceFindingIsPending += 1
		}
		if p.numMetricsLinesSinceFindingIsPending == 2 {
			// We saw two metric lines since the line that marked the
			// beginning of the error report. We assume that this means
			// that no more lines from the error report are following.
			// A single metrics line could be in between the lines of
			// the error report, because the metrics and error report
			// printed asynchronously, but the time between two metrics
			// lines are printed is long enough that it should be safe
			// to assume that the error report was printed out completely
			// by now. We therefore send the pending finding now.
			err = p.finalizeAndSendPendingFinding(ctx)
			if err != nil {
				return err
			}
		}

		return nil
	}

	finding := p.parseAsNewFinding(line)

	if finding != nil && !p.libFuzzerErrorFollowingGoPanic(finding) {
		// If there is still a pending finding, send it now, because
		// we'll treat all further output lines as belonging to the new
		// finding.
		err := p.finalizeAndSendPendingFindingIfAny(ctx)
		if err != nil {
			return err
		}

		// Store the finding but don't send it yet, because we keep
		// parsing more output lines which might belong to the error
		// report and contain relevant info.
		p.pendingFinding = finding

		return nil
	}

	if p.pendingFinding != nil {
		// The line is not a metrics line and doesn't mark a new finding,
		// so we append it to the pending finding (unless it's filtered)
		if !minijail.IsIgnoredLine(line) && !p.foundBeginningOfJestReport {
			p.pendingFinding.Logs = append(p.pendingFinding.Logs, line)
		}
	}

	// Check if the line contains the path to the test input file (which
	// we expect when we have a pending finding)
	testInputFilePath, ok := parseAsTestInputFilePath(line)
	if ok {
		testInput, err := os.ReadFile(testInputFilePath)
		if err != nil {
			return errors.WithStack(err)
		}

		// Attach the input data to the pending finding
		if p.pendingFinding == nil {
			return errors.Errorf("Unexpected test input line: %s", line)
		}
		p.pendingFinding.InputData = testInput
		p.pendingFinding.InputFile = testInputFilePath

		return nil
	}

	return nil
}

func (p *parser) parseAsNewFinding(line string) *finding.Finding {
	if p.SupportJazzer {
		finding := p.parseAsJazzerFinding(line)
		if finding != nil {
			return finding
		}
	}

	if p.SupportJazzerJS {
		finding := p.parseAsJazzerJSFinding(line)
		if finding != nil {
			return finding
		}
	}

	finding := p.parseAsGoFinding(line)
	if finding != nil {
		return finding
	}

	finding = p.parseAsLibfuzzerFinding(line)
	if finding != nil {
		// If JazzerJS is supported, the libfuzzer finding is part of a
		// JazzerJS finding and should not be treated as a new finding
		if p.SupportJazzerJS && p.pendingFinding != nil {
			return nil
		}
		return finding
	}

	finding = sanitizer.ParseAsFinding(line)
	if finding != nil {
		return finding
	}

	finding = parseAsSlowInput(line)
	if finding != nil {
		return finding
	}

	return nil
}

func parseAsTestInputFilePath(logLine string) (string, bool) {
	result, found := regexutil.FindNamedGroupsMatch(testInputFilePattern, logLine)
	if found {
		return result["test_input_file"], true
	}
	return "", false
}

func (p *parser) parseAsGoFinding(line string) *finding.Finding {
	if _, found := regexutil.FindNamedGroupsMatch(goPanicPattern, line); found {
		return &finding.Finding{
			Type:    finding.ErrorTypeCrash,
			Details: "Go Panic",
			Logs:    []string{line},
		}
	}
	return nil
}

func (p *parser) libFuzzerErrorFollowingGoPanic(report *finding.Finding) bool {
	return p.pendingFinding.GetDetails() == "Go Panic" && report.GetDetails() != "Go Panic"
}

func (p *parser) parseAsLibfuzzerFinding(line string) *finding.Finding {
	// For timeout errors, the first output line belonging to the error
	// report is *not* the "ERROR:" line, but the "ALARM:" line, so we
	// match that pattern first
	result, found := regexutil.FindNamedGroupsMatch(libfuzzerTimeoutErrorPattern, line)
	if found {
		return &finding.Finding{
			Type:    finding.ErrorTypeCrash, // aka Vulnerability
			Details: fmt.Sprintf("timeout after %s seconds", result["timeout_seconds"]),
			Logs:    []string{line},
		}
	}

	// All other libfuzzer errors start with the "ERROR:" line
	result, found = regexutil.FindNamedGroupsMatch(libfuzzerErrorPattern, line)
	if found {
		if strings.HasPrefix(result["error_type"], "timeout") {
			// This the "ERROR:" line of a timeout report. We already
			// created a finding for that.
			return nil
		}

		if strings.HasPrefix(result["error_type"], "out-of-memory") &&
			strings.HasPrefix(p.pendingFinding.GetDetails(), "out-of-memory") {
			// libFuzzer sometimes reports OOM errors twice, once
			// because a single malloc call exceeds the rss limit and
			// once because the rss limit was exceeded. We don't want
			// to create two findings in that case, so we ignore the
			// second error.
			return nil
		}

		return &finding.Finding{
			Type:    finding.ErrorTypeCrash, // aka Vulnerability
			Details: result["error_type"],
			Logs:    []string{line},
		}
	}

	return nil
}

func (p *parser) parseAsJazzerFinding(line string) *finding.Finding {
	matches, found := regexutil.FindNamedGroupsMatch(jazzerSecurityIssuePattern, line)
	if found {
		exceptionMessage := strings.TrimSpace(matches["message"])
		description := "Security Issue: " + exceptionMessage

		return &finding.Finding{
			Type:    finding.ErrorTypeCrash, // aka Vulnerability
			Details: description,
			Logs:    []string{line},
		}
	}

	_, found = regexutil.FindNamedGroupsMatch(javaAssertionErrorPattern, line)
	if found {
		return &finding.Finding{
			Type:    finding.ErrorTypeWarning, // aka Bug
			Details: "Java Assertion Error",
			Logs:    []string{line},
		}
	}

	matches, found = regexutil.FindNamedGroupsMatch(javaExceptionErrorPattern, line)
	if found {
		return &finding.Finding{
			Type:    finding.ErrorTypeWarning, // aka Bug
			Details: matches["error_type"],
			Logs:    []string{line},
		}
	}

	return nil
}

func (p *parser) parseAsJazzerJSFinding(line string) *finding.Finding {
	_, found := regexutil.FindNamedGroupsMatch(beginningOfJestReportPattern, line)
	if found {
		p.foundBeginningOfJestReport = true
		return nil
	}

	result, found := regexutil.FindNamedGroupsMatch(jazzerJSExceptionErrorPattern, line)
	if found {
		return &finding.Finding{
			Type:    finding.ErrorTypeWarning,
			Details: result["message"],
			Logs:    []string{line},
		}
	}

	_, found = regexutil.FindNamedGroupsMatch(jazzerJSCommandInjectionPattern, line)
	if found {
		return &finding.Finding{
			Type:    finding.ErrorTypeWarning,
			Details: "Command Injection",
			Logs:    []string{line},
		}
	}

	_, found = regexutil.FindNamedGroupsMatch(jazzerJSPathTraveralPattern, line)
	if found {
		return &finding.Finding{
			Type:    finding.ErrorTypeWarning,
			Details: "Path Traversal",
			Logs:    []string{line},
		}
	}

	_, found = regexutil.FindNamedGroupsMatch(jazzerJSPrototypePollutionPattern, line)
	if found {
		return &finding.Finding{
			Type:    finding.ErrorTypeWarning,
			Details: "Prototype Pollution",
			Logs:    []string{line},
		}
	}

	return nil
}

func (p *parser) parseAsFuzzingMetric(line string) *report.FuzzingMetric {
	if result, found := regexutil.FindNamedGroupsMatch(statsPattern, line); found {
		totalExecs, err := strconv.ParseUint(result["total_execs"], 10, 64)
		if err != nil {
			return nil
		}
		features, err := strconv.Atoi(result["features"])
		if err != nil {
			return nil
		}

		var edges int
		if result["edges"] == "" {
			edges = 0
		} else {
			edges, err = strconv.Atoi(result["edges"])
			if err != nil {
				return nil
			}
		}

		execsPerSec, err := strconv.Atoi(result["executions_per_second"])
		if err != nil {
			return nil
		}
		corpusSize, err := strconv.Atoi(result["corpus_size"])
		if err != nil {
			return nil
		}
		now := time.Now()
		var secondsSinceLastFeature uint64
		if !p.lastNewFeatureTime.IsZero() {
			secondsSinceLastFeature = uint64(now.Sub(p.lastNewFeatureTime).Truncate(time.Second).Seconds())
		}

		if features > p.lastFeatures {
			p.lastNewFeatureTime = now
			p.lastFeatures = features
			secondsSinceLastFeature = 0
		}
		var secondsSinceLastEdge uint64
		if !p.lastNewEdgeTime.IsZero() {
			secondsSinceLastEdge = uint64(now.Sub(p.lastNewEdgeTime).Truncate(time.Second).Seconds())
		}
		if edges > p.lastEdges {
			p.lastNewEdgeTime = now
			p.lastEdges = edges
			secondsSinceLastEdge = 0
		}

		if !p.initFinished && result["status"] == "INITED" {
			p.initFinished = true
		}

		return &report.FuzzingMetric{
			Timestamp:               now,
			ExecutionsPerSecond:     int32(execsPerSec),
			Features:                int32(features),
			Edges:                   int32(edges),
			CorpusSize:              int32(corpusSize),
			TotalExecutions:         totalExecs,
			SecondsSinceLastFeature: secondsSinceLastFeature,
			SecondsSinceLastEdge:    secondsSinceLastEdge,
		}
	}
	return nil
}

func parseAsSlowInput(log string) *finding.Finding {
	if res, ok := regexutil.FindNamedGroupsMatch(slowInputPattern, log); ok {
		return &finding.Finding{
			Type:    finding.ErrorTypeWarning,
			Details: fmt.Sprintf("Slow input detected. Processing time: %s s", res["duration"]),
			Logs:    []string{fmt.Sprintf("Slow input: %s seconds for processing", res["duration"])},
		}
	}
	return nil
}

func parseAsInputPathMessage(line string) (string, bool) {
	result, found := regexutil.FindNamedGroupsMatch(jazzerInputPathPatter, line)
	if !found {
		return "", false
	}
	return result["input_path"], true
}

func parseAsSeedCorpusMessage(line string) (numSeeds uint, err error) { //nolint:nonamedreturns
	numSeeds, err = parseAsNonEmptyCorpusMessage(line)
	if err == nil {
		return numSeeds, nil
	}
	if !errors.Is(err, errNotFound) {
		// Unexpected error
		return 0, err
	}
	found := parseAsEmptyCorpusMessage(line)
	if found {
		return 0, nil
	}
	return 0, errNotFound
}

func parseAsNonEmptyCorpusMessage(line string) (numSeeds uint, err error) { //nolint:nonamedreturns
	result, found := regexutil.FindNamedGroupsMatch(nonEmptyCorpusPattern, line)
	if !found {
		return 0, errNotFound
	}
	numSeedsUInt64, err := strconv.ParseUint(result["num_seeds"], 10, 0)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return uint(numSeedsUInt64), nil
}

func parseAsEmptyCorpusMessage(line string) bool {
	matches := emptyCorpusPattern.FindStringSubmatch(line)
	return matches != nil
}

func (p *parser) finalizeAndSendPendingFindingIfAny(ctx context.Context) error {
	if p.pendingFinding == nil {
		return nil
	}

	if p.pendingFinding.Details == "" {
		return nil
	}

	return p.finalizeAndSendPendingFinding(ctx)
}

func (p *parser) finalizeAndSendPendingFinding(ctx context.Context) error {
	var err error

	// Parse the stack trace
	parserOpts := &stacktrace.ParserOptions{
		ProjectDir:      p.ProjectDir,
		SourceMap:       p.SourceMap,
		SupportJazzer:   p.SupportJazzer,
		SupportJazzerJS: p.SupportJazzerJS,
	}
	parser, err := stacktrace.NewParser(parserOpts)
	if err != nil {
		return err
	}
	p.pendingFinding.StackTrace, err = parser.Parse(p.pendingFinding.Logs)
	if err != nil {
		return err
	}

	p.pendingFinding.MoreDetails = &finding.ErrorDetails{
		ID: errorid.ForFinding(p.pendingFinding),
	}

	err = p.sendFinding(ctx, p.pendingFinding)
	if err != nil {
		return err
	}
	p.pendingFinding = nil
	p.numMetricsLinesSinceFindingIsPending = 0
	return nil
}

func (p *parser) sendFinding(ctx context.Context, finding *finding.Finding) error {
	p.FindingReported = true

	return p.sendReport(ctx, &report.Report{
		Status:  report.RunStatusRunning,
		Finding: finding,
	})
}
