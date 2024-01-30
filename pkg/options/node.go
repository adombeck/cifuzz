package options

import (
	"fmt"
)

const JazzerJSTestNamePattern string = "--testNamePattern"
const JazzerJSTestPathPattern string = "--testPathPattern"
const JazzerJSReporters string = "--reporters"
const JazzerJSCoverageDirectory string = "--coverageDirectory"
const JazzerJSCoverageReporters string = "--coverageReporters"
const JestTestFailureExitCode string = "--testFailureExitCode"

func JazzerJSTestNamePatternFlag(value string) string {
	return JazzerJSTestNamePattern + fmt.Sprintf("='%s'", value)
}

func JazzerJSTestPathPatternFlag(value string) string {
	return JazzerJSTestPathPattern + fmt.Sprintf("='%s'", value)
}

func JazzerJSReportersFlag(value string) string {
	if value == "" {
		return JazzerJSReporters
	}
	return JazzerJSReporters + fmt.Sprintf("='%s'", value)
}

func JazzerJSCoverageDirectoryFlag(value string) string {
	return JazzerJSCoverageDirectory + fmt.Sprintf("='%s'", value)
}

func JazzerJSCoverageReportersFlag(value string) string {
	return JazzerJSCoverageReporters + fmt.Sprintf("='%s'", value)
}

func JestTestFailureExitCodeFlag(value int) string {
	return JestTestFailureExitCode + fmt.Sprintf("='%d'", value)
}
