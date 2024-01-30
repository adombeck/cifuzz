package logging

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func CreateLogDir(projectDir string) (string, error) {
	logDir := filepath.Join(projectDir, ".cifuzz-build", "logs")
	// create logs dir if it doesn't exist
	err := os.MkdirAll(logDir, 0700)
	if err != nil {
		return "", errors.Wrap(err, "Failed to create directory for log file")
	}

	return logDir, nil
}

func SuffixForLog(fuzzTestNames []string) string {
	var logSuffix string
	switch {
	case len(fuzzTestNames) == 0 || (len(fuzzTestNames) == 1 && fuzzTestNames[0] == ""):
		logSuffix = "all"
	case len(fuzzTestNames) > 1:
		logSuffix = strings.Join(fuzzTestNames, "_")
	default:
		logSuffix = fuzzTestNames[0]
	}
	// Make sure that calling fuzz tests in subdirs don't mess up the build log path
	return strings.ReplaceAll(logSuffix, string(os.PathSeparator), "_")
}
