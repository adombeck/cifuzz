package adapter

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type BuildResultType interface {
	build.BuildResult | build.CBuildResult | build.JavaBuildResult
}

func wrapBuild[BR BuildResultType](opts *RunOptions, build func(*RunOptions) (*BR, error)) (*BR, error) {
	// Note that the build printer should *not* print to c.opts.buildStdout,
	// because that could be a file which is used to store the build log.
	// We don't want the messages of the build printer to be printed to
	// the build log file, so we let it print to stdout or stderr instead.
	var buildPrinterOutput io.Writer
	if opts.PrintJSON {
		buildPrinterOutput = opts.Stdout
	} else {
		buildPrinterOutput = opts.Stderr
	}
	buildPrinter := logging.NewBuildPrinter(buildPrinterOutput, log.BuildInProgressMsg)

	cBuildResult, err := build(opts)
	if err != nil {
		buildPrinter.StopOnError(log.BuildInProgressErrorMsg)
	} else {
		buildPrinter.StopOnSuccess(log.BuildInProgressSuccessMsg, true)
	}
	return cBuildResult, err
}

func prepareCorpusDir(opts *RunOptions, buildResult *build.BuildResult) error {
	switch opts.BuildSystem {
	case config.BuildSystemCMake, config.BuildSystemBazel, config.BuildSystemOther:
		// The generated corpus dir has to be created before starting the fuzzing run.
		err := os.MkdirAll(buildResult.GeneratedCorpus, 0o755)
		if err != nil {
			return errors.WithStack(err)
		}
		log.Infof("Storing generated corpus in %s", fileutil.PrettifyPath(buildResult.GeneratedCorpus))

		// Ensure that symlinks are resolved to be able to add minijail
		// bindings for the corpus dirs.
		exists, err := fileutil.Exists(buildResult.SeedCorpus)
		if err != nil {
			return err
		}
		if exists {
			buildResult.SeedCorpus, err = filepath.EvalSymlinks(buildResult.SeedCorpus)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		buildResult.GeneratedCorpus, err = filepath.EvalSymlinks(buildResult.GeneratedCorpus)
		if err != nil {
			return errors.WithStack(err)
		}

		for i, dir := range opts.SeedCorpusDirs {
			opts.SeedCorpusDirs[i], err = filepath.EvalSymlinks(dir)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	case config.BuildSystemMaven, config.BuildSystemGradle:
		// The seed corpus dir has to be created before starting the fuzzing run.
		// Otherwise jazzer will store the findings in the project dir.
		// It is not necessary to create the corpus dir. Jazzer will do that for us.
		err := os.MkdirAll(cmdutils.JazzerSeedCorpus(opts.FuzzTest, opts.ProjectDir), 0o755)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func createReportHandler(opts *RunOptions, buildResult *build.BuildResult) (*reporthandler.ReportHandler, error) {
	printerOutput := os.Stdout
	jsonOutput := io.Discard
	if opts.PrintJSON {
		printerOutput = os.Stderr
		jsonOutput = os.Stdout
	}

	// Initialize the report handler. Only do this right before we start
	// the fuzz test, because this is storing a timestamp which is used
	// to figure out how long the fuzzing run is running.
	return reporthandler.NewReportHandler(
		opts.FuzzTest,
		&reporthandler.ReportHandlerOptions{
			ProjectDir:           opts.ProjectDir,
			UserSeedCorpusDirs:   opts.SeedCorpusDirs,
			ManagedSeedCorpusDir: buildResult.SeedCorpus,
			GeneratedCorpusDir:   buildResult.GeneratedCorpus,
			PrinterOutput:        printerOutput,
			JSONOutput:           jsonOutput,
		},
	)
}
