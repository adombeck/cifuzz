package coverage

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummary_PrintTable(t *testing.T) {
	rPipe, wPipe, err := os.Pipe()
	require.NoError(t, err)

	report := `SF:bar.cpp
FNH:2
FNF:21
end_of_record
SF:foo.cpp
FNH:1
FNF:1
end_of_record
`
	summary, err := ParseLCOVReportIntoSummary(strings.NewReader(report))
	require.NoError(t, err)
	summary.PrintTable(wPipe)

	wPipe.Close()
	pipeOut, err := io.ReadAll(rPipe)
	require.NoError(t, err)
	out := string(pipeOut)

	assert.Contains(t, out, "bar.cpp")
	assert.Contains(t, out, "foo.cpp")
	assert.Contains(t, out, "2 / 21   (9.5%)")
	assert.Contains(t, out, "0 / 0 (100.0%)")
	assert.Contains(t, out, "3 / 22")
}
