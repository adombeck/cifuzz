package runner

import (
	"github.com/pterm/pterm"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runner/jazzerjs"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
)

type NodeJSRunner struct {
}

func (r *NodeJSRunner) CheckDependencies(projectDir string) error {
	return dependencies.Check([]dependencies.Key{
		dependencies.Node,
	}, projectDir)
}

func (r *NodeJSRunner) Run(opts *RunOptions) (*reporthandler.ReportHandler, error) {
	style := pterm.Style{pterm.Reset, pterm.FgLightBlue}
	log.Infof("Running %s", style.Sprintf(opts.FuzzTest+":"+opts.TestNamePattern))

	reportHandler, err := createReportHandler(opts, &build.BuildResult{})
	if err != nil {
		return nil, err
	}

	runnerOpts := &jazzerjs.RunnerOptions{
		PackageManager:  "npm",
		TestPathPattern: opts.FuzzTest,
		TestNamePattern: opts.TestNamePattern,
		LibfuzzerOptions: &libfuzzer.RunnerOptions{
			Dictionary:     opts.Dictionary,
			EngineArgs:     opts.EngineArgs,
			EnvVars:        []string{"NO_CIFUZZ=1"},
			KeepColor:      !opts.PrintJSON && !log.PlainStyle(),
			ProjectDir:     opts.ProjectDir,
			ReportHandler:  reportHandler,
			SeedCorpusDirs: opts.SeedCorpusDirs,
			Timeout:        opts.Timeout,
			UseMinijail:    opts.UseSandbox,
			Verbose:        viper.GetBool("verbose"),
		},
	}
	err = ExecuteFuzzerRunner(jazzerjs.NewRunner(runnerOpts))
	if err != nil {
		return nil, err
	}

	return reportHandler, nil
}

func (*NodeJSRunner) Cleanup() {
}
