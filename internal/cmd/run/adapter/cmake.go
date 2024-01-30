package adapter

import (
	"runtime"

	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/cmake"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/pkg/dependencies"
)

type CMakeAdapter struct {
}

func (r *CMakeAdapter) CheckDependencies(projectDir string) error {
	var deps []dependencies.Key
	deps = []dependencies.Key{
		dependencies.CMake,
		dependencies.LLVMSymbolizer,
	}
	switch runtime.GOOS {
	case "linux", "darwin":
		deps = append(deps, dependencies.Clang)
	case "windows":
		deps = append(deps, dependencies.VisualStudio)
	}

	return dependencies.Check(deps, projectDir)
}

func (r *CMakeAdapter) Run(opts *RunOptions) (*reporthandler.ReportHandler, error) {
	cBuildResult, err := wrapBuild[build.CBuildResult](opts, r.build)
	if err != nil {
		return nil, err
	}

	if opts.BuildOnly {
		return nil, nil
	}

	err = prepareCorpusDir(opts, cBuildResult.BuildResult)
	if err != nil {
		return nil, err
	}

	reportHandler, err := createReportHandler(opts, cBuildResult.BuildResult)
	if err != nil {
		return nil, err
	}

	err = runLibfuzzer(opts, cBuildResult.BuildResult, reportHandler)
	if err != nil {
		return nil, err
	}

	return reportHandler, nil
}

func (r *CMakeAdapter) build(opts *RunOptions) (*build.CBuildResult, error) {
	sanitizers := []string{"address", "undefined"}

	var builder *cmake.Builder
	builder, err := cmake.NewBuilder(&cmake.BuilderOptions{
		ProjectDir: opts.ProjectDir,
		Args:       opts.ArgsToPass,
		Sanitizers: sanitizers,
		Parallel: cmake.ParallelOptions{
			Enabled: viper.IsSet("build-jobs"),
			NumJobs: opts.NumBuildJobs,
		},
		Stdout:    opts.BuildStdout,
		Stderr:    opts.BuildStderr,
		BuildOnly: opts.BuildOnly,
	})
	if err != nil {
		return nil, err
	}
	err = builder.Configure()
	if err != nil {
		return nil, err
	}

	cBuildResults, err := builder.Build([]string{opts.FuzzTest})
	if err != nil {
		return nil, err
	}
	// TODO: Maybe it would be more elegant to let builder.Build return
	//       an empty build result so that this check is not needed.
	if opts.BuildOnly {
		return nil, nil
	}

	return cBuildResults[0], nil
}

func (*CMakeAdapter) Cleanup() {
}
