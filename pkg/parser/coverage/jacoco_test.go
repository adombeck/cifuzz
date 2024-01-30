package coverage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJacocoXMLIntoLCOVReport(t *testing.T) {
	reportData := `
<report name="gradle">
    <package name="com/example">
        <class name="com/example/ExploreMe" sourcefilename="ExploreMe.java">
            <method line="3" name="&lt;init&gt;">
                <counter covered="1" missed="0" type="LINE"/>
                <counter covered="1" missed="0" type="METHOD"/>
            </method>
            <method line="5" name="exploreMe">
                <counter covered="3" missed="3" type="BRANCH"/>
                <counter covered="4" missed="2" type="LINE"/>
                <counter covered="1" missed="0" type="METHOD"/>
            </method>
            <counter covered="3" missed="3" type="BRANCH"/>
            <counter covered="5" missed="2" type="LINE"/>
            <counter covered="2" missed="0" type="METHOD"/>
        </class>
        <sourcefile name="ExploreMe.java">
            <line cb="0" ci="3" mb="0" mi="0" nr="3"/>
            <line cb="1" ci="2" mb="1" mi="0" nr="5"/>
            <line cb="2" ci="3" mb="0" mi="0" nr="6"/>
            <line cb="0" ci="5" mb="0" mi="0" nr="7"/>
            <line cb="0" ci="0" mb="2" mi="3" nr="10"/>
            <line cb="0" ci="0" mb="0" mi="5" nr="11"/>
            <line cb="0" ci="1" mb="0" mi="0" nr="14"/>
            <counter covered="3" missed="3" type="BRANCH"/>
            <counter covered="5" missed="2" type="LINE"/>
            <counter covered="2" missed="0" type="METHOD"/>
        </sourcefile>
		<counter covered="3" missed="3" type="BRANCH"/>
        <counter covered="5" missed="2" type="LINE"/>
        <counter covered="2" missed="0" type="METHOD"/>
    </package>
</report>
`

	report, err := ParseJacocoXMLIntoLCOVReport(strings.NewReader(reportData))
	require.NoError(t, err)

	require.Len(t, report.SourceFiles, 1, "incorrect number of SourceFiles")
	assert.Equal(t, filepath.Join("src", "main", "java", "com", "example", "ExploreMe.java"), report.SourceFiles[0].Name)

	require.Len(t, report.SourceFiles[0].FunctionInformation, 2, "incorrect number of FunctionInformation")
	assert.Equal(t, "<init>", report.SourceFiles[0].FunctionInformation[0].Name, "incorrect function name (information)")
	assert.Equal(t, 3, report.SourceFiles[0].FunctionInformation[0].Line, "incorrect function line")
	assert.Equal(t, "exploreMe", report.SourceFiles[0].FunctionInformation[1].Name, "incorrect function name (information)")
	assert.Equal(t, 5, report.SourceFiles[0].FunctionInformation[1].Line, "incorrect function line")

	require.Len(t, report.SourceFiles[0].FunctionExecutions, 2, "incorrect number of FunctionExecutions")
	assert.Equal(t, "<init>", report.SourceFiles[0].FunctionExecutions[0].Name, "incorrect function name (executions)")
	assert.Equal(t, 1, report.SourceFiles[0].FunctionExecutions[0].Executions, "incorrect function executions")
	assert.Equal(t, "exploreMe", report.SourceFiles[0].FunctionExecutions[1].Name, "incorrect function name (executions)")
	assert.Equal(t, 1, report.SourceFiles[0].FunctionExecutions[1].Executions, "incorrect function executions")

	assert.Equal(t, 2, report.SourceFiles[0].FunctionsFound, "incorrect number of functions found")
	assert.Equal(t, 2, report.SourceFiles[0].FunctionsHit, "incorrect number of functions hit")

	require.Len(t, report.SourceFiles[0].BranchInformation, 6, "incorrect number of BranchInformation")
	covered := 0
	missed := 0
	for _, br := range report.SourceFiles[0].BranchInformation {
		if br.Executions == 0 {
			missed++
		} else {
			covered++
		}
	}
	assert.Equal(t, 3, covered, "incorrect number of branches covered")
	assert.Equal(t, 3, missed, "incorrect number of branches missed")
	assert.Equal(t, covered+missed, report.SourceFiles[0].BranchesFound, "incorrect number of branches found")
	assert.Equal(t, covered, report.SourceFiles[0].BranchesHit, "incorrect number of branches hit")

	require.Len(t, report.SourceFiles[0].LineInformation, 7, "incorrect number of LineInformation")
	covered = 0
	missed = 0
	for _, l := range report.SourceFiles[0].LineInformation {
		if l.Executions == 0 {
			missed++
		} else {
			covered++
		}
	}
	assert.Equal(t, 5, covered, "incorrect number of lines covered")
	assert.Equal(t, 2, missed, "incorrect number of lines missed")
	assert.Equal(t, covered+missed, report.SourceFiles[0].LinesFound, "incorrect number of lines found")
	assert.Equal(t, covered, report.SourceFiles[0].LinesHit, "incorrect number of lines hit")
}

func TestParseJacocoXMLIntoLCOVReport_Empty(t *testing.T) {
	report, err := ParseJacocoXMLIntoLCOVReport(strings.NewReader(""))
	require.NoError(t, err)
	assert.Len(t, report.SourceFiles, 0)
}

func TestParseJacocoIntoSummary(t *testing.T) {
	reportData := `
<report name="maven-example">
    <package name="com/example">
        <sourcefile name="ExploreMe.java">
            <counter type="LINE" missed="100" covered="100"/>
            <counter type="BRANCH" missed="22" covered="1"/>
            <counter type="METHOD" missed="19" covered="2"/>
        </sourcefile>
        <sourcefile name="App.java">
            <counter type="LINE" missed="0" covered="50"/>
            <counter type="BRANCH" missed="1" covered="9"/>
            <counter type="METHOD" missed="0" covered="1"/>
        </sourcefile>
    </package>
</report>
`
	summary := ParseJacocoXMLIntoSummary(strings.NewReader(reportData))

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

func TestParseJacocoIntoSummary_Empty(t *testing.T) {
	summary := ParseJacocoXMLIntoSummary(strings.NewReader(""))

	assert.Len(t, summary.Files, 0, "incorrect number of files")
	assert.Empty(t, summary.Total.BranchesFound, "incorrect number of total branches found")
	assert.Empty(t, summary.Total.LinesFound, "incorrect number of total functions found")
	assert.Empty(t, summary.Total.FunctionsFound, "incorrect number of total lines found")
}
