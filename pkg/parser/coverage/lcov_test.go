package coverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
)

func TestWriteLCOVReportToFile(t *testing.T) {
	report := LCOVReport{
		SourceFiles: []*SourceFile{{
			Name: "com/example/ExploreMe.java",
			FunctionInformation: []Function{{
				Name: "exploreMe",
				Line: 2,
			}},
			FunctionExecutions: []FunctionExecution{{
				Name:       "exploreMe",
				Executions: 1,
			}},
			LineInformation: []Line{
				{Number: 3, Executions: 1},
				{Number: 4, Executions: 0},
				{Number: 5, Executions: 1},
				{Number: 6, Executions: 1},
			},
			BranchInformation: []Branch{{
				Line:       5,
				Block:      0,
				Number:     0,
				Executions: 1},
				{
					Line:       5,
					Block:      0,
					Number:     1,
					Executions: 0,
				}},
			Overview: Overview{
				FunctionsFound: 1,
				FunctionsHit:   1,
				LinesFound:     4,
				LinesHit:       3,
				BranchesFound:  2,
				BranchesHit:    1,
			},
		}},
	}

	tempDir := testutil.MkdirTemp(t, "", "lcov-test")
	lcovPath := filepath.Join(tempDir, "report.lcov")
	err := report.WriteLCOVReportToFile(lcovPath)
	require.NoError(t, err)
	assert.FileExists(t, lcovPath)

	expectedLCOV := `SF:com/example/ExploreMe.java
FN:2,exploreMe
FNDA:1,exploreMe
FNF:1
FNH:1
DA:3,1
DA:4,0
DA:5,1
DA:6,1
LF:4
LH:3
BRDA:5,0,0,1
BRDA:5,0,1,-
BRF:2
BRH:1
end_of_record
`
	data, err := os.ReadFile(lcovPath)
	require.NoError(t, err)
	assert.Equal(t, expectedLCOV, string(data), "written report to file does not match expected report")
}

func TestWriteLCOVReportToFile_EmptyReport(t *testing.T) {
	report := LCOVReport{}

	tempDir := testutil.MkdirTemp(t, "", "lcov-test")
	lcovPath := filepath.Join(tempDir, "report.lcov")
	err := report.WriteLCOVReportToFile(lcovPath)
	require.NoError(t, err)
	assert.NoFileExists(t, lcovPath, "lcov file should not exist")
}

func TestParseLCOVFileIntoLCOVReport(t *testing.T) {
	lcovFile := `SF:com/example/ExploreMe.java
FN:2,exploreMe
FNDA:1,exploreMe
FNF:1
FNH:1
DA:3,1
DA:4,0
DA:5,1
DA:6,1
LF:4
LH:3
BRDA:5,0,0,1
BRDA:5,0,1,-
BRF:2
BRH:1
end_of_record
SF:com/example/ExploreMe2.java
end_of_record`

	report, err := ParseLCOVFileIntoLCOVReport(strings.NewReader(lcovFile))
	require.NoError(t, err)

	require.Len(t, report.SourceFiles, 2, "incorrect number of source file")
	assert.Equal(t, "com/example/ExploreMe.java", report.SourceFiles[0].Name, "incorrect name of source file (0)")
	assert.Equal(t, "com/example/ExploreMe2.java", report.SourceFiles[1].Name, "incorrect name of source file (1)")

	require.Len(t, report.SourceFiles[0].FunctionInformation, 1, "incorrect number of FunctionInformation")
	require.Len(t, report.SourceFiles[0].FunctionInformation, report.SourceFiles[0].FunctionsFound, "incorrect number of functions found")
	assert.Equal(t, 1, report.SourceFiles[0].FunctionsHit, "incorrect number of functions hit")
	assert.Equal(t, "exploreMe", report.SourceFiles[0].FunctionInformation[0].Name, "incorrect function name (information)")
	assert.Equal(t, 2, report.SourceFiles[0].FunctionInformation[0].Line, "incorrect function line")

	require.Len(t, report.SourceFiles[0].FunctionExecutions, 1, "incorrect number of FunctionExecutions")
	assert.Equal(t, "exploreMe", report.SourceFiles[0].FunctionExecutions[0].Name, "incorrect function name (executions)")
	assert.Equal(t, 1, report.SourceFiles[0].FunctionExecutions[0].Executions, "incorrect function executions")

	require.Len(t, report.SourceFiles[0].LineInformation, 4, "incorrect number of LineInformation")
	require.Len(t, report.SourceFiles[0].LineInformation, report.SourceFiles[0].LinesFound, "incorrect number of lines found")
	assert.Equal(t, 3, report.SourceFiles[0].LinesHit, "incorrect number of lines hit")
	assert.Equal(t, 3, report.SourceFiles[0].LineInformation[0].Number, "incorrect line number (0)")
	assert.Equal(t, 1, report.SourceFiles[0].LineInformation[0].Executions, "incorrect line executions (0)")
	assert.Equal(t, 4, report.SourceFiles[0].LineInformation[1].Number, "incorrect line number (1)")
	assert.Equal(t, 0, report.SourceFiles[0].LineInformation[1].Executions, "incorrect line executions (1)")
	assert.Equal(t, 5, report.SourceFiles[0].LineInformation[2].Number, "incorrect line number (2)")
	assert.Equal(t, 1, report.SourceFiles[0].LineInformation[2].Executions, "incorrect line executions (2)")
	assert.Equal(t, 6, report.SourceFiles[0].LineInformation[3].Number, "incorrect line number (3)")
	assert.Equal(t, 1, report.SourceFiles[0].LineInformation[3].Executions, "incorrect line executions (3)")

	require.Len(t, report.SourceFiles[0].BranchInformation, 2, "incorrect number of BranchInformation")
	require.Len(t, report.SourceFiles[0].BranchInformation, report.SourceFiles[0].BranchesFound, "incorrect number of branches found")
	assert.Equal(t, 1, report.SourceFiles[0].BranchesHit, "incorrect number of branches hit")
	assert.Equal(t, 5, report.SourceFiles[0].BranchInformation[0].Line, "incorrect branch line (0)")
	assert.Equal(t, 0, report.SourceFiles[0].BranchInformation[0].Block, "incorrect branch block (0)")
	assert.Equal(t, 0, report.SourceFiles[0].BranchInformation[0].Number, "incorrect branch number (0)")
	assert.Equal(t, 1, report.SourceFiles[0].BranchInformation[0].Executions, "incorrect branch executions (0)")
	assert.Equal(t, 5, report.SourceFiles[0].BranchInformation[1].Line, "incorrect branch line (1)")
	assert.Equal(t, 0, report.SourceFiles[0].BranchInformation[1].Block, "incorrect branch block (1)")
	assert.Equal(t, 1, report.SourceFiles[0].BranchInformation[1].Number, "incorrect branch number (1)")
	assert.Equal(t, 0, report.SourceFiles[0].BranchInformation[1].Executions, "incorrect branch executions (1)")
}

func TestParseLCOVFileIntoLCOVReport_EmptyFile(t *testing.T) {
	lcovFile := ""
	report, err := ParseLCOVFileIntoLCOVReport(strings.NewReader(lcovFile))
	require.NoError(t, err)
	assert.Len(t, report.SourceFiles, 0, "report should not have found any source files")
}

func TestParseLCOVFileIntoLCOVReport_InvalidFormat(t *testing.T) {
	lcovFile := `SF:com/example/ExploreMe.java
123
end_of_record`

	report, err := ParseLCOVFileIntoLCOVReport(strings.NewReader(lcovFile))
	require.Error(t, err)
	assert.Empty(t, report)
}

func TestParseLCOVFileIntoLCOVReport_ParsingFailure(t *testing.T) {
	lcovFile := `SF:com/example/ExploreMe.java
FN:2a,exploreMe
end_of_record`

	report, err := ParseLCOVFileIntoLCOVReport(strings.NewReader(lcovFile))
	require.Error(t, err)
	assert.Empty(t, report)
}

func TestParseLCOVIntoSummary(t *testing.T) {
	report := `SF:bar.cpp
FNH:2
FNF:21
BRH:1
BRF:23
LH:100
LF:200
end_of_record
SF:foo.cpp
FNH:1
FNF:1
BRH:9
BRF:10
LH:50
LF:50
end_of_record
`
	summary, err := ParseLCOVReportIntoSummary(strings.NewReader(report))
	require.NoError(t, err)

	require.Len(t, summary.Files, 2, "incorrect number of files")

	assert.Equal(t, 3, summary.Total.FunctionsHit, "incorrect number of total functions hit")
	assert.Equal(t, 22, summary.Total.FunctionsFound, "incorrect number of total functions found")
	assert.Equal(t, 10, summary.Total.BranchesHit, "incorrect number of total branches hit")
	assert.Equal(t, 33, summary.Total.BranchesFound, "incorrect number of total branches found")
	assert.Equal(t, 150, summary.Total.LinesHit, "incorrect number of total lines hit")
	assert.Equal(t, 250, summary.Total.LinesFound, "incorrect number of total lines found")

	assert.Equal(t, 2, summary.Files[0].Coverage.FunctionsHit, "incorrect number of functions hit (0)")
	assert.Equal(t, 21, summary.Files[0].Coverage.FunctionsFound, "incorrect number of functions found (0)")
	assert.Equal(t, 1, summary.Files[0].Coverage.BranchesHit, "incorrect number of branches hit (0)")
	assert.Equal(t, 23, summary.Files[0].Coverage.BranchesFound, "incorrect number of branches found (0)")
	assert.Equal(t, 100, summary.Files[0].Coverage.LinesHit, "incorrect number of lines hit (0)")
	assert.Equal(t, 200, summary.Files[0].Coverage.LinesFound, "incorrect number of lines found (0)")

	assert.Equal(t, 1, summary.Files[1].Coverage.FunctionsHit, "incorrect number of functions hit (1)")
	assert.Equal(t, 1, summary.Files[1].Coverage.FunctionsFound, "incorrect number of functions found (1)")
	assert.Equal(t, 9, summary.Files[1].Coverage.BranchesHit, "incorrect number of branches hit (1)")
	assert.Equal(t, 10, summary.Files[1].Coverage.BranchesFound, "incorrect number of branches found (1)")
	assert.Equal(t, 50, summary.Files[1].Coverage.LinesHit, "incorrect number of lines hit (1)")
	assert.Equal(t, 50, summary.Files[1].Coverage.LinesFound, "incorrect number of lines found (1)")
}

func TestParseLCOVIntoSummary_EmptyReport(t *testing.T) {
	report := ""
	summary, err := ParseLCOVReportIntoSummary(strings.NewReader(report))
	require.NoError(t, err)
	assert.Len(t, summary.Files, 0, "summary should not have any source files")
	assert.Empty(t, summary.Total.BranchesFound, "summary shouldn't have any found branches")
	assert.Empty(t, summary.Total.LinesFound, "summary shouldn't have any found lines")
	assert.Empty(t, summary.Total.FunctionsFound, "summary shouldn't have any found functions")
}
