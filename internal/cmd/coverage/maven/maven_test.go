package maven

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/build/java/maven"
)

type MavenRunnerMock struct {
	mock.Mock
}

func (runner *MavenRunnerMock) RunCommand(args []string) error {
	runner.Called(args)

	return nil
}

func TestBuildFuzzTestForCoverage(t *testing.T) {
	outputPath := "project-dir/cov-output"
	fuzzTest := "com.example.FuzzTestCase"
	targetMethod := "MyFuzzTest"

	runnerMock := &MavenRunnerMock{}
	gen := &CoverageGenerator{
		OutputPath:   outputPath,
		FuzzTest:     fuzzTest,
		TargetMethod: targetMethod,
		Parallel: maven.ParallelOptions{
			Enabled: false,
		},
		MavenRunner: runnerMock,
	}

	expectedArgsFirstCall := []string{
		"-Dmaven.test.failure.ignore=true",
		"-Djazzer.hooks=false",
		"-Pcifuzz",
		fmt.Sprintf("-Dtest=%s#%s", fuzzTest, targetMethod),
		"test",
	}
	expectedArgsSecondCall := []string{
		"-Pcifuzz",
		"jacoco:report",
		fmt.Sprintf("-Dcifuzz.report.output=%s", outputPath),
		"-Dcifuzz.report.format=XML,HTML",
	}
	runnerMock.On("RunCommand", expectedArgsFirstCall).Return(nil)
	runnerMock.On("RunCommand", expectedArgsSecondCall).Return(nil)

	err := gen.BuildFuzzTestForCoverage()
	require.NoError(t, err)
	runnerMock.AssertExpectations(t)
}
