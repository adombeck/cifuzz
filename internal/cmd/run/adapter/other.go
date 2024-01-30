package adapter

import (
	"runtime"
	"strings"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/other"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
)

type OtherAdapter struct {
}

func (r *OtherAdapter) CheckDependencies(projectDir string) error {

	var deps []dependencies.Key
	switch runtime.GOOS {
	case "linux", "darwin":
		deps = []dependencies.Key{
			dependencies.Clang,
			dependencies.LLVMSymbolizer,
		}
	case "windows":
		deps = []dependencies.Key{
			dependencies.VisualStudio,
		}
	}
	return dependencies.Check(deps, projectDir)
}

func (r *OtherAdapter) Run(opts *RunOptions) (*reporthandler.ReportHandler, error) {
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

func (r *OtherAdapter) build(opts *RunOptions) (*build.CBuildResult, error) {
	if len(opts.ArgsToPass) > 0 {
		log.Warnf("Passing additional arguments is not supported for build system type \"other\".\n"+
			"These arguments are ignored: %s", strings.Join(opts.ArgsToPass, " "))
	}

	sanitizers := []string{"address", "undefined"}

	var builder *other.Builder
	builder, err := other.NewBuilder(&other.BuilderOptions{
		ProjectDir:   opts.ProjectDir,
		BuildCommand: opts.BuildCommand,
		CleanCommand: opts.CleanCommand,
		Sanitizers:   sanitizers,
		Stdout:       opts.BuildStdout,
		Stderr:       opts.BuildStderr,
	})
	if err != nil {
		return nil, err
	}

	err = builder.Clean()
	if err != nil {
		return nil, err
	}

	cBuildResult, err := builder.Build(opts.FuzzTest)
	if err != nil {
		return nil, err
	}
	return cBuildResult, nil
}

func (*OtherAdapter) Cleanup() {
}
