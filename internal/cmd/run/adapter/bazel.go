package adapter

import (
	"os"
	"os/exec"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/bazel"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type BazelAdapter struct {
	tempDir string
}

func (r *BazelAdapter) CheckDependencies(projectDir string) error {
	// All dependencies are managed via bazel but it should be checked
	// that the correct bazel version is installed
	return dependencies.Check([]dependencies.Key{
		dependencies.Bazel,
	}, projectDir)
}

func (r *BazelAdapter) Run(opts *RunOptions) (*reporthandler.ReportHandler, error) {
	// Create a temporary directory which the builder can use to create
	// temporary files
	var err error
	r.tempDir, err = os.MkdirTemp("", "cifuzz-run-")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	buildResult, err := wrapBuild[build.BuildResult](opts, r.build)
	if err != nil {
		return nil, err
	}

	if opts.BuildOnly {
		return nil, nil
	}

	err = prepareCorpusDir(opts, buildResult)
	if err != nil {
		return nil, err
	}

	reportHandler, err := createReportHandler(opts, buildResult)
	if err != nil {
		return nil, err
	}

	// The install base directory contains e.g. the script generated
	// by bazel via --script_path and must therefore be accessible
	// inside the sandbox.
	cmd := exec.Command("bazel", "info", "install_base")
	err = cmd.Run()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	err = runLibfuzzer(opts, buildResult, reportHandler)
	if err != nil {
		return nil, err
	}
	return reportHandler, nil
}

func (r *BazelAdapter) build(opts *RunOptions) (*build.BuildResult, error) {

	// The cc_fuzz_test rule defines multiple bazel targets: If the
	// name is "foo", it defines the targets "foo", "foo_bin", and
	// others. We need to run the "foo_bin" target but want to
	// allow users to specify either "foo" or "foo_bin", so we check
	// if the fuzz test name appended with "_bin" is a valid target
	// and use that in that case
	cmd := exec.Command("bazel", "query", opts.FuzzTest+"_bin")
	err := cmd.Run()
	if err == nil {
		opts.FuzzTest += "_bin"
	}

	var builder *bazel.Builder
	builder, err = bazel.NewBuilder(&bazel.BuilderOptions{
		ProjectDir: opts.ProjectDir,
		Args:       opts.ArgsToPass,
		NumJobs:    opts.NumBuildJobs,
		Stdout:     opts.BuildStdout,
		Stderr:     opts.BuildStderr,
		TempDir:    r.tempDir,
		Verbose:    viper.GetBool("verbose"),
	})
	if err != nil {
		return nil, err
	}

	var buildResults []*build.BuildResult
	buildResults, err = builder.BuildForRun([]string{opts.FuzzTest})
	if err != nil {
		return nil, err
	}
	return buildResults[0], nil
}

func (r *BazelAdapter) Cleanup() {
	fileutil.Cleanup(r.tempDir)
}
