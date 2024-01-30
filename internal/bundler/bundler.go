package bundler

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/vcs"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

// The (possibly empty) directory inside the fuzzing artifact archive that will
// be the fuzzers working directory.
const archiveWorkDirPath = "work_dir"

type Bundler struct {
	opts *Opts
}

func New(opts *Opts) *Bundler {
	return &Bundler{opts: opts}
}

func (b *Bundler) Bundle() (string, error) {
	var err error

	// Create temp dir
	b.opts.tempDir, err = os.MkdirTemp("", "cifuzz-bundle-")
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer fileutil.Cleanup(b.opts.tempDir)

	var bundle *os.File
	bundle, err = b.createEmptyBundle()
	if err != nil {
		return "", err
	}
	// if an error occurs during bundling we should make sure that
	// the bundle gets removed
	defer func() {
		bundle.Close()
		if err != nil {
			os.Remove(bundle.Name())
		}
	}()

	// Create archive writer
	bufWriter := bufio.NewWriter(bundle)
	archiveWriter := archive.NewTarArchiveWriter(bufWriter, true)

	var fuzzers []*archive.Fuzzer
	switch b.opts.BuildSystem {
	case config.BuildSystemCMake, config.BuildSystemBazel, config.BuildSystemOther:
		fuzzers, err = newLibfuzzerBundler(b.opts, archiveWriter).bundle()
	case config.BuildSystemMaven, config.BuildSystemGradle:
		fuzzers, err = newJazzerBundler(b.opts, archiveWriter).bundle()
	default:
		err = errors.Errorf("Unknown build system for bundler: %s", b.opts.BuildSystem)
	}
	if err != nil {
		return "", err
	}

	dockerImageUsedInBundle := b.determineDockerImageForBundle()
	err = b.createMetadataFileInArchive(fuzzers, archiveWriter, dockerImageUsedInBundle)
	if err != nil {
		return "", err
	}

	err = b.createWorkDirInArchive(archiveWriter)
	if err != nil {
		return "", err
	}

	err = b.copyAdditionalFilesToArchive(archiveWriter)
	if err != nil {
		return "", err
	}

	if b.opts.BundleBuildLogFile != "" {
		err = archiveWriter.WriteFile("build.log", b.opts.BundleBuildLogFile)
		if err != nil {
			return "", errors.WithStack(err)
		}
	}

	// List contents of archive in verbose mode for easier debugging
	// when we do not have access to the bundle itself
	tableBuf := &strings.Builder{}
	w := tabwriter.NewWriter(tableBuf, 0, 0, 1, ' ', tabwriter.AlignRight)
	for _, h := range archiveWriter.Headers() {
		_, err := fmt.Fprintf(w, "%s\t%d\t %s\n", h.FileInfo().Mode().String(), h.Size, h.Name)
		if err != nil {
			return "", errors.WithStack(err)
		}
	}
	err = w.Flush()
	if err != nil {
		return "", errors.WithStack(err)
	}
	log.Debugf("Content of bundle %s:\n%s", bundle.Name(), tableBuf.String())

	err = archiveWriter.Close()
	if err != nil {
		return "", errors.WithStack(err)
	}
	err = bufWriter.Flush()
	if err != nil {
		return "", errors.WithStack(err)
	}
	err = bundle.Close()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return bundle.Name(), nil
}

func (b *Bundler) createEmptyBundle() (*os.File, error) {
	archiveExt := ".tar.gz"

	if b.opts.OutputPath != "" {
		// Check that outpath path makes sense
		if !strings.HasSuffix(b.opts.OutputPath, archiveExt) {
			log.Debugf("Provided output path was missing extension, %s has been added", archiveExt)
			b.opts.OutputPath += archiveExt
		}
	} else if len(b.opts.FuzzTests) == 1 {
		fuzzTestName := strings.ReplaceAll(b.opts.FuzzTests[0], "::", "_")
		b.opts.OutputPath = filepath.Base(fuzzTestName) + archiveExt
	} else {
		b.opts.OutputPath = "fuzz_tests" + archiveExt
	}

	bundle, err := os.Create(b.opts.OutputPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create fuzzing artifact archive")
	}

	log.Debugf("Bundle output path: %s", b.opts.OutputPath)

	return bundle, nil
}

func (b *Bundler) determineDockerImageForBundle() string {
	dockerImageUsedInBundle := b.opts.DockerImage
	if dockerImageUsedInBundle == "" {
		switch b.opts.BuildSystem {
		case config.BuildSystemCMake, config.BuildSystemBazel, config.BuildSystemOther:
			// Use default Ubuntu Docker image for CMake, Bazel, and other build systems
			dockerImageUsedInBundle = "ubuntu:rolling"
		case config.BuildSystemMaven, config.BuildSystemGradle:
			// Maven and Gradle should use a Docker image with Java
			dockerImageUsedInBundle = "eclipse-temurin:20"
		}
	}

	log.Debugf("Bundle uses %s as docker image", dockerImageUsedInBundle)

	return dockerImageUsedInBundle
}

func (b *Bundler) createMetadataFileInArchive(fuzzers []*archive.Fuzzer, archiveWriter archive.ArchiveWriter, dockerImageUsedInBundle string) error {
	// Create and add the top-level metadata file.
	metadata := &archive.Metadata{
		Fuzzers: fuzzers,
		RunEnvironment: &archive.RunEnvironment{
			Docker: dockerImageUsedInBundle,
		},
		CodeRevision: b.getCodeRevision(),
	}

	metadataYamlContent, err := metadata.ToYaml()
	if err != nil {
		return err
	}
	metadataYamlPath := filepath.Join(b.opts.tempDir, archive.MetadataFileName)
	err = os.WriteFile(metadataYamlPath, metadataYamlContent, 0o644)
	if err != nil {
		return errors.Wrapf(err, "failed to write %s", archive.MetadataFileName)
	}
	err = archiveWriter.WriteFile(archive.MetadataFileName, metadataYamlPath)
	if err != nil {
		return err
	}

	// Print bundle.yaml content for debugging purposes
	log.Debugf("Content of bundle.yaml:\n%s", metadataYamlContent)

	return nil
}

func (b *Bundler) createWorkDirInArchive(archiveWriter archive.ArchiveWriter) error {
	// The fuzzing artifact archive spec requires this directory even if it is empty.
	tempWorkDirPath := filepath.Join(b.opts.tempDir, archiveWorkDirPath)
	err := os.Mkdir(tempWorkDirPath, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = archiveWriter.WriteDir(archiveWorkDirPath, tempWorkDirPath)
	if err != nil {
		return err
	}

	return nil
}

func (b *Bundler) copyAdditionalFilesToArchive(archiveWriter archive.ArchiveWriter) error {
	for _, arg := range b.opts.AdditionalFiles {
		source, target, err := parseAdditionalFilesArgument(arg)
		if err != nil {
			return err
		}

		log.Debugf("Adding additional dir/file %s", arg)

		if !filepath.IsAbs(source) {
			source = filepath.Join(b.opts.ProjectDir, source)
		}

		if fileutil.IsDir(source) {
			err = archiveWriter.WriteDir(target, source)
			if err != nil {
				return err
			}
		} else {
			err = archiveWriter.WriteFile(target, source)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// getCodeRevision returns the code revision of the project, if it can be
// determined. If it cannot be determined, nil is returned.
func (b *Bundler) getCodeRevision() *archive.CodeRevision {
	revision := vcs.CodeRevision()
	if revision == nil {
		revision = &archive.CodeRevision{
			Git: &archive.GitRevision{},
		}
	}

	if b.opts.Commit != "" {
		revision.Git.Commit = b.opts.Commit
	}

	if b.opts.Branch != "" {
		revision.Git.Branch = b.opts.Branch
	}

	return revision
}

func prepareSeeds(seedCorpusDirs []string, archiveSeedsDir string, archiveWriter archive.ArchiveWriter) error {
	var targetDirs []string
	for _, sourceDir := range seedCorpusDirs {
		// Put the seeds into subdirectories of the "seeds" directory
		// to avoid seeds with the same name to override each other.

		// Choose a name for the target directory which wasn't used
		// before
		basename := filepath.Join(archiveSeedsDir, filepath.Base(sourceDir))
		targetDir := basename
		i := 1
		for sliceutil.Contains(targetDirs, targetDir) {
			targetDir = fmt.Sprintf("%s-%d", basename, i)
			i++
		}
		targetDirs = append(targetDirs, targetDir)

		// Add the seeds of the seed corpus directory to the target directory
		err := archiveWriter.WriteDir(targetDir, sourceDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseAdditionalFilesArgument(arg string) (string, string, error) {
	var source, target string
	parts := strings.Split(arg, ";")

	if len(parts) == 1 {
		// if there is no ; separator just use the work_dir
		// handles "test.txt"
		source = parts[0]
		target = filepath.Join(archiveWorkDirPath, filepath.Base(arg))
	} else {
		// handles test.txt;test2.txt
		source = parts[0]
		target = parts[1]
	}

	if len(parts) > 2 || source == "" || target == "" {
		return "", "", errors.New("could not parse '--add' argument")
	}

	if filepath.IsAbs(target) {
		return "", "", errors.New("when using '--add source;target', target has to be a relative path")
	}

	return source, target, nil
}
