package gradle

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/build/java/gradle"
)

type GradleRunnerMock struct {
	mock.Mock
}

func (runner *GradleRunnerMock) RunCommand(args []string) error {
	runner.Called(args)

	return nil
}

func TestBuildFuzzTestForCoverage(t *testing.T) {
	outputPath := "project-dir/cov-output"
	fuzzTest := "com.example.FuzzTestCase"
	targetMethod := "MyFuzzTest"

	runnerMock := &GradleRunnerMock{}
	gen := &CoverageGenerator{
		OutputPath:   outputPath,
		FuzzTest:     fuzzTest,
		TargetMethod: targetMethod,
		Parallel: gradle.ParallelOptions{
			Enabled: false,
		},
		GradleRunner: runnerMock,
	}

	expectedArgs := []string{
		fmt.Sprintf("-Pcifuzz.fuzztest=%s.%s", fuzzTest, targetMethod),
		"cifuzzReport",
		fmt.Sprintf("-Pcifuzz.report.output=%s", outputPath),
	}
	runnerMock.On("RunCommand", expectedArgs).Return(nil)

	err := gen.BuildFuzzTestForCoverage()
	require.NoError(t, err)
	runnerMock.AssertExpectations(t)
}
