package coverage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
)

// TODO: do we need TN:<test name>? Only for merged reports?

type LCOVReport struct {
	SourceFiles []*SourceFile
}

type SourceFile struct {
	Name                string
	FunctionInformation []Function
	FunctionExecutions  []FunctionExecution
	LineInformation     []Line
	BranchInformation   []Branch
	Overview
}

type Function struct {
	Name string
	Line int
}

type FunctionExecution struct {
	Name       string
	Executions int
}

type Line struct {
	Number     int
	Executions int
}

type Branch struct {
	Line       int
	Block      int
	Number     int
	Executions int
}

type Overview struct {
	FunctionsFound int
	FunctionsHit   int
	LinesFound     int
	LinesHit       int
	BranchesFound  int
	BranchesHit    int
}

func (r *LCOVReport) WriteLCOVReportToFile(file string) error {
	if r.SourceFiles == nil || len(r.SourceFiles) == 0 {
		log.Debug("LCOV report is empty, no file created")
		return nil
	}

	if !strings.HasSuffix(file, ".lcov") {
		file += ".lcov"
		log.Debug("Missing extension '.lcov' was appended to path")
	}

	// Note: file needs read/write access to be used with genhtml later on
	f, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()

	for _, sf := range r.SourceFiles {
		// SF:<absolute path to the source file>
		s := fmt.Sprintf("SF:%s\n", sf.Name)

		// Function Coverage
		for _, f := range sf.FunctionInformation {
			// FN:<line number of function start>,<function name>
			s += fmt.Sprintf("FN:%d,%s\n", f.Line, f.Name)
		}
		for _, f := range sf.FunctionExecutions {
			// FNDA:<execution count>,<function name>
			s += fmt.Sprintf("FNDA:%d,%s\n", f.Executions, f.Name)
		}
		// FNF:<number of functions found>
		s += fmt.Sprintf("FNF:%d\n", sf.FunctionsFound)
		// FNH:<number of function hit>
		s += fmt.Sprintf("FNH:%d\n", sf.FunctionsHit)

		// Line Coverage
		for _, l := range sf.LineInformation {
			// DA:<line number>,<execution count>[,<checksum>]
			s += fmt.Sprintf("DA:%d,%d\n", l.Number, l.Executions)
		}
		// LF:<number of instrumented lines>
		s += fmt.Sprintf("LF:%d\n", sf.LinesFound)
		// LH:<number of lines with a non-zero execution count>
		s += fmt.Sprintf("LH:%d\n", sf.LinesHit)

		// Branch coverage
		for _, b := range sf.BranchInformation {
			if b.Executions == 0 {
				// BRDA:<line number>,<block number>,<branch number>,<taken>
				s += fmt.Sprintf("BRDA:%d,0,%d,-\n", b.Line, b.Number)
			} else {
				s += fmt.Sprintf("BRDA:%d,0,%d,%d\n", b.Line, b.Number, b.Executions)
			}
		}
		// BRF:<number of branches found>
		s += fmt.Sprintf("BRF:%d\n", sf.BranchesFound)
		// BRH:<number of branches hit>
		s += fmt.Sprintf("BRH:%d\n", sf.BranchesHit)

		// Necessary to signal end of sourcefile section
		s += fmt.Sprintf("end_of_record\n")

		_, err = f.WriteString(s)
		if err != nil {
			return errors.Wrapf(err, "Failed to write to file '%s'", file)
		}
	}

	log.Debugf("Successfully wrote lcov report to %s", file)
	return nil
}

func ParseLCOVFileIntoLCOVReport(in io.Reader) (*LCOVReport, error) {
	var err error
	report := &LCOVReport{}
	currentSourceFile := &SourceFile{}

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Text()
		invalidFormatErr := fmt.Errorf("'%s' is not a valid lcov format", line)
		failedParsing := fmt.Sprintf("Failed to parse line in lcov report: %s", line)

		if line == "end_of_record" {
			report.SourceFiles = append(report.SourceFiles, currentSourceFile)
			currentSourceFile = &SourceFile{}
			continue
		}

		prefix, v, found := strings.Cut(line, ":")
		if !found {
			return nil, invalidFormatErr
		}

		switch prefix {
		case "SF":
			currentSourceFile.Name = v

		case "LF":
			currentSourceFile.LinesFound, err = strconv.Atoi(v)
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}

		case "LH":
			currentSourceFile.LinesHit, err = strconv.Atoi(v)
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}

		case "FNF":
			currentSourceFile.FunctionsFound, err = strconv.Atoi(v)
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}

		case "FNH":
			currentSourceFile.FunctionsHit, err = strconv.Atoi(v)
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}

		case "BRF":
			currentSourceFile.BranchesFound, err = strconv.Atoi(v)
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}

		case "BRH":
			currentSourceFile.BranchesHit, err = strconv.Atoi(v)
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}

		case "FN":
			split := strings.Split(v, ",")
			if len(split) != 2 {
				return nil, invalidFormatErr
			}

			l, err := strconv.Atoi(split[0])
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}
			currentSourceFile.FunctionInformation = append(currentSourceFile.FunctionInformation, Function{
				Name: split[1],
				Line: l,
			})

		case "FNDA":
			split := strings.Split(v, ",")
			if len(split) != 2 {
				return nil, invalidFormatErr
			}

			e, err := strconv.Atoi(split[0])
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}
			currentSourceFile.FunctionExecutions = append(currentSourceFile.FunctionExecutions, FunctionExecution{
				Name:       split[1],
				Executions: e,
			})

		case "DA":
			split := strings.Split(v, ",")
			// Note: DA can have checksum as third value which we are ignoring
			if len(split) < 2 || len(split) > 3 {
				return nil, invalidFormatErr
			}

			l, err := strconv.Atoi(split[0])
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}
			e, err := strconv.Atoi(split[1])
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}
			currentSourceFile.LineInformation = append(currentSourceFile.LineInformation, Line{
				Number:     l,
				Executions: e,
			})

		case "BRDA":
			split := strings.Split(v, ",")
			if len(split) != 4 {
				return nil, invalidFormatErr
			}

			l, err := strconv.Atoi(split[0])
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}
			bl, err := strconv.Atoi(split[1])
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}
			br, err := strconv.Atoi(split[2])
			if err != nil {
				return nil, errors.Wrap(err, failedParsing)
			}
			e := 0
			if split[3] != "-" {
				e, err = strconv.Atoi(split[3])
				if err != nil {
					return nil, errors.Wrap(err, failedParsing)
				}
			}
			currentSourceFile.BranchInformation = append(currentSourceFile.BranchInformation, Branch{
				Line:       l,
				Block:      bl,
				Number:     br,
				Executions: e,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.WithStack(err)
	}

	return report, nil
}

// ParseLCOVReportIntoSummary takes a lcov report and turns it
// into the `Summary` struct. It will print the summary in verbose mode
// in JSON format if possible.
func ParseLCOVReportIntoSummary(in io.Reader) (*Summary, error) {
	summary := &Summary{
		Total: Overview{},
	}

	report, err := ParseLCOVFileIntoLCOVReport(in)
	if err != nil {
		return nil, err
	}

	for _, sf := range report.SourceFiles {
		currentFile := &FileCoverage{
			Filename: sf.Name,
			Coverage: sf.Overview,
		}
		summary.Files = append(summary.Files, currentFile)
	}

	for _, f := range summary.Files {
		summary.Total.LinesHit += f.Coverage.LinesHit
		summary.Total.LinesFound += f.Coverage.LinesFound
		summary.Total.BranchesFound += f.Coverage.BranchesFound
		summary.Total.BranchesHit += f.Coverage.BranchesHit
		summary.Total.FunctionsFound += f.Coverage.FunctionsFound
		summary.Total.FunctionsHit += f.Coverage.FunctionsHit
	}

	// This is not an essential step, so we don't fail on error
	out, err := json.MarshalIndent(summary, "", "    ")
	if err != nil {
		log.Errorf(errors.WithStack(err), "Unable to convert coverage summary to json: %v", err)
	} else {
		log.Debugf("Successfully created coverage summary: %s", string(out))
	}

	return summary, nil
}
