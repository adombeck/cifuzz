package coverage

import (
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
)

type JacocoXMLReport struct {
	Name     string `xml:"name,attr"`
	Packages []struct {
		Name  string `xml:"name,attr"`
		Class []struct {
			Name           string `xml:"name,attr"`
			SourceFileName string `xml:"sourcefilename,attr"`
			Method         []struct {
				Name    string          `xml:"name,attr"`
				Line    int             `xml:"line,attr"`
				Counter []JacocoCounter `xml:"counter"`
			} `xml:"method"`
			Counter []JacocoCounter `xml:"counter"`
		} `xml:"class"`
		SourceFiles []struct {
			Name string `xml:"name,attr"`
			Line []struct {
				Nr                  int `xml:"nr,attr"`
				MissedInstructions  int `xml:"mi,attr"`
				CoveredInstructions int `xml:"ci,attr"`
				MissedBranches      int `xml:"mb,attr"`
				CoveredBranches     int `xml:"cb,attr"`
			} `xml:"line"`
			Counter []JacocoCounter `xml:"counter"`
		} `xml:"sourcefile"`
		Counter []JacocoCounter `xml:"counter"`
	} `xml:"package"`
	Counter []JacocoCounter `xml:"counter"`
}

type JacocoCounter struct {
	Type    string `xml:"type,attr"`
	Missed  int    `xml:"missed,attr"`
	Covered int    `xml:"covered,attr"`
}

func ParseJacocoXMLIntoLCOVReport(in io.Reader) (*LCOVReport, error) {
	lcovReport := &LCOVReport{}

	output, err := io.ReadAll(in)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read jacoco.xml report")
	}

	if len(output) == 0 {
		log.Debugf("Empty jacoco.xml, returning empty LCOV report")
		return lcovReport, nil
	}

	jacocoReport := &JacocoXMLReport{}
	err = xml.Unmarshal(output, jacocoReport)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse jacoco.xml report")
	}

	for _, pkg := range jacocoReport.Packages {
		for _, sourceFile := range pkg.SourceFiles {
			sf := SourceFile{}

			packagePath := filepath.Join(pkg.Name, sourceFile.Name)
			// Note: Sourcefile name needs to have full path so that the files
			// can be found when they are mapped by genhtml
			// TODO: handle cases where path is not default
			sf.Name = filepath.Join("src", "main", "java", packagePath)

			for _, line := range sourceFile.Line {
				// Line coverage
				executions := 0
				if line.CoveredInstructions > 0 {
					// If any instruction/statement in the line was covered it
					// means that it was executed at least once
					executions = 1
				}
				sf.LineInformation = append(sf.LineInformation, Line{
					Number:     line.Nr,
					Executions: executions,
				})

				// Branch coverage
				for i := 0; i < line.CoveredBranches; i++ {
					sf.BranchInformation = append(sf.BranchInformation, Branch{
						Line:       line.Nr,
						Executions: 1,
						Number:     i,
					})
				}
				for i := 0; i < line.MissedBranches; i++ {
					sf.BranchInformation = append(sf.BranchInformation, Branch{
						Line:       line.Nr,
						Executions: 0,
						Number:     i,
					})
				}
			}

			for _, class := range pkg.Class {
				// Only get information for the current sourcefile
				// Note: Using filepath.ToSlash() to make this check work with
				// Windows file paths
				if class.Name != filepath.ToSlash(strings.TrimSuffix(packagePath, ".java")) {
					continue
				}

				// Function coverage
				for _, method := range class.Method {
					f := Function{
						Name: method.Name,
						Line: method.Line,
					}
					sf.FunctionInformation = append(sf.FunctionInformation, f)

					executions := 0
					for _, counter := range method.Counter {
						if counter.Type == "METHOD" && counter.Covered > 0 {
							executions = 1
							break
						}
					}
					e := FunctionExecution{
						Name:       method.Name,
						Executions: executions,
					}
					sf.FunctionExecutions = append(sf.FunctionExecutions, e)
				}
			}

			// Accumulated coverage data
			for _, counter := range sourceFile.Counter {
				countJacoco(&sf.Overview, &counter)
			}

			lcovReport.SourceFiles = append(lcovReport.SourceFiles, &sf)
		}
	}

	return lcovReport, nil
}

// ParseJacocoXMLIntoSummary takes a jacoco xml report and turns it into
// the `Overview` struct. The parsing is as forgiving
// as possible. It will output debug/error logs instead of
// failing, with the goal to gather as much information as
// possible
func ParseJacocoXMLIntoSummary(in io.Reader) *Summary {
	coverageSummary := &Summary{
		Total: Overview{},
	}

	output, err := io.ReadAll(in)
	if err != nil {
		log.Errorf(err, "Unable to read jacoco.xml report: %v", err)
		return coverageSummary
	}

	if len(output) == 0 {
		log.Debugf("Empty jacoco.xml, returning empty coverage summary")
		return coverageSummary
	}

	report := &JacocoXMLReport{}
	err = xml.Unmarshal(output, report)
	if err != nil {
		log.Errorf(err, "Unable to parse jacoco.xml report: %v", err)
		return coverageSummary
	}

	var currentFile *FileCoverage
	for _, xmlPackage := range report.Packages {
		for _, sourcefile := range xmlPackage.SourceFiles {
			currentFile = &FileCoverage{
				Filename: fmt.Sprintf("%s/%s", xmlPackage.Name, sourcefile.Name),
				Coverage: Overview{},
			}
			for _, counter := range sourcefile.Counter {
				countJacoco(&coverageSummary.Total, &counter)
				countJacoco(&currentFile.Coverage, &counter)
			}
			coverageSummary.Files = append(coverageSummary.Files, currentFile)
		}
	}

	return coverageSummary
}

func countJacoco(c *Overview, counter *JacocoCounter) {
	switch counter.Type {
	case "LINE":
		c.LinesFound += counter.Covered + counter.Missed
		c.LinesHit += counter.Covered
	case "BRANCH":
		c.BranchesFound += counter.Covered + counter.Missed
		c.BranchesHit += counter.Covered
	case "METHOD":
		c.FunctionsFound += counter.Covered + counter.Missed
		c.FunctionsHit += counter.Covered
	}
}
