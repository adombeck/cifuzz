package log

import (
	"bytes"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testOut io.ReadWriter

func TestMain(m *testing.M) {
	testOut = bytes.NewBuffer([]byte{})
	Output = testOut
	disableColor = true

	viper.Set("verbose", false)
	m.Run()
	viper.Set("verbose", false)
}

func TestDebug_NoVerbose(t *testing.T) {
	Debugf("Test")
	out, err := io.ReadAll(testOut)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestDebug_Verbose(t *testing.T) {
	viper.Set("verbose", true)
	Debugf("Test")
	viper.Set("verbose", false)
	checkOutput(t, "Test\n")
}

func TestError_Verbose(t *testing.T) {
	viper.Set("verbose", true)
	Errorf(errors.New("test-error"), "Test")
	viper.Set("verbose", false)
	checkOutput(t, "Test\n", "code-intelligence.com/")
}

func TestError_NoVerbose(t *testing.T) {
	Errorf(errors.New("test-error"), "Test")
	out := checkOutput(t, "Test\n")
	require.NotContains(t, out, "test-error")
}

func TestSuccess(t *testing.T) {
	Success("Test")
	checkOutput(t, "Test\n")
}

func TestInfo(t *testing.T) {
	Info("Test")
	checkOutput(t, "Test\n")
}

func TestWarn(t *testing.T) {
	Warn("Test")
	checkOutput(t, "Test\n")
}

func TestStylePretty(t *testing.T) {
	disableColor = false
	viper.Set("style", "pretty")

	Success("Test")
	hasColor, out := checkForColoredOutput(t)
	require.True(t, hasColor)
	require.Contains(t, out, "✅")
}

func TestStyleColor(t *testing.T) {
	disableColor = false
	viper.Set("style", "color")

	Success("Test")
	hasColor, out := checkForColoredOutput(t)
	require.True(t, hasColor)
	require.NotContains(t, out, "✅")
}

func TestStylePlain(t *testing.T) {
	viper.Set("style", "plain")

	Success("Test")
	hasColor, out := checkForColoredOutput(t)
	require.False(t, hasColor)
	require.NotContains(t, out, "✅")

	// To make sure that all other tests are not influenced
	// by the pterm.DisableColor() call in the log() func
	pterm.EnableColor()
}

// checkForColoredOutput tests if removing color codes from the string
// results in a shorter string, confirming there are colors in the output.
func checkForColoredOutput(t *testing.T) (bool, string) {
	out, err := io.ReadAll(testOut)
	require.NoError(t, err)
	lenOutput := len(out)
	colorlessOutput := pterm.RemoveColorFromString(string(out))
	return len(colorlessOutput) < lenOutput, string(out)
}

func checkOutput(t *testing.T, a ...string) string {
	out, err := io.ReadAll(testOut)
	require.NoError(t, err)
	for _, s := range a {
		require.Contains(t, string(out), s)
	}
	return string(out)
}
