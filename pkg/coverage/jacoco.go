package coverage

import (
	"encoding/xml"
	"fmt"
	"os"

	"code-intelligence.com/cifuzz/pkg/log"
)

type jacocoCounter struct {
	Text    string `xml:",chardata"`
	Type    string `xml:"type,attr"`
	Missed  int    `xml:"missed,attr"`
	Covered int    `xml:"covered,attr"`
}

type jacocoReport struct {
	XMLName     xml.Name `xml:"report"`
	Text        string   `xml:",chardata"`
	Name        string   `xml:"name,attr"`
	Sessioninfo []struct {
		Text  string `xml:",chardata"`
		ID    string `xml:"id,attr"`
		Start string `xml:"start,attr"`
		Dump  string `xml:"dump,attr"`
	} `xml:"sessioninfo"`
	Packages []struct {
		Text    string `xml:",chardata"`
		Name    string `xml:"name,attr"`
		Classes []struct {
			Text           string `xml:",chardata"`
			Name           string `xml:"name,attr"`
			Sourcefilename string `xml:"sourcefilename,attr"`
			Method         []struct {
				Text    string          `xml:",chardata"`
				Name    string          `xml:"name,attr"`
				Desc    string          `xml:"desc,attr"`
				Line    string          `xml:"line,attr"`
				Counter []jacocoCounter `xml:"counter"`
			} `xml:"method"`
			Counter []jacocoCounter `xml:"counter"`
		} `xml:"class"`
		Sourcefiles []struct {
			Text string `xml:",chardata"`
			Name string `xml:"name,attr"`
			Line []struct {
				Text string `xml:",chardata"`
				Nr   string `xml:"nr,attr"`
				Mi   string `xml:"mi,attr"`
				Ci   string `xml:"ci,attr"`
				Mb   string `xml:"mb,attr"`
				Cb   string `xml:"cb,attr"`
			} `xml:"line"`
			Counter []jacocoCounter `xml:"counter"`
		} `xml:"sourcefile"`
		Counter []jacocoCounter `xml:"counter"`
	} `xml:"package"`
	Counter []jacocoCounter `xml:"counter"`
}

func countJacoco(c *Coverage, counter *jacocoCounter) {
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

// ParseJacocoXML takes a jacoco xml report and turns it into
// the `CoverageSummary` struct. The parsing is as forgiving
// as possible. It will output debug/error logs instead of
// failing, with the goal to gather as much information as
// possible
func ParseJacocoXML(reportPath string) *CoverageSummary {
	summary := &CoverageSummary{
		Total: &Coverage{},
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		log.Debugf("Unable to open jacoco xml report: %s", reportPath)
		return summary
	}
	report := &jacocoReport{}
	err = xml.Unmarshal([]byte(data), report)
	if err != nil {
		log.Debugf("Unable to parse jacoco xml report: %s", reportPath)
		return summary
	}

	var currentFile *FileCoverage
	for _, xmlPackage := range report.Packages {
		for _, sourcefile := range xmlPackage.Sourcefiles {
			currentFile = &FileCoverage{
				Filename: fmt.Sprintf("%s/%s", xmlPackage.Name, sourcefile.Name),
				Coverage: &Coverage{},
			}
			for _, counter := range sourcefile.Counter {
				countJacoco(summary.Total, &counter)
				if currentFile != nil {
					countJacoco(currentFile.Coverage, &counter)
				}
			}
			summary.Files = append(summary.Files, currentFile)
		}
	}

	return summary
}
