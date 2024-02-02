package container

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/template"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/term"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/version"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

const cifuzzImageBase = "ghcr.io/adombeck/cifuzz"

//go:embed ensure-cifuzz.sh
var ensureCifuzzScript string

//go:embed Dockerfile.tmpl
var dockerfileTemplate string

type dockerfileConfig struct {
	CIFuzzImage string
	Base        string
}

// BuildImageFromBundle creates an image based on an existing bundle
func BuildImageFromBundle(bundlePath string) error {
	buildContextDir, err := prepareBuildContext(bundlePath)
	if err != nil {
		return err
	}
	return buildImageFromDir(buildContextDir)
}

// prepareBuildContext takes a existing artifact bundle, extracts it
// and adds needed files/information
func prepareBuildContext(bundlePath string) (string, error) {
	// extract bundle to a temporary directory
	buildContextDir, err := os.MkdirTemp("", "bundle-extract")
	if err != nil {
		return "", errors.WithStack(err)
	}

	err = archive.Extract(bundlePath, buildContextDir)
	if err != nil {
		return "", err
	}

	// read metadata from bundle to use information for building
	// the right image
	metadata, err := archive.MetadataFromPath(filepath.Join(buildContextDir, archive.MetadataFileName))
	if err != nil {
		return "", err
	}

	// add additional files needed for the image
	// eg. build instructions and cifuzz executables
	err = createDockerfile(filepath.Join(buildContextDir, "Dockerfile"), metadata.Docker)
	if err != nil {
		return "", err
	}
	err = copyCifuzz(buildContextDir)
	if err != nil {
		return "", err
	}

	log.Debugf("Prepared build context for fuzz container image at %s", buildContextDir)

	return buildContextDir, nil
}

// builds an image based on an existing directory
func buildImageFromDir(buildContextDir string) error {
	imageTar, err := createImageTar(buildContextDir)
	if err != nil {
		return err
	}
	defer fileutil.Cleanup(imageTar.Name())

	dockerClient, err := getDockerClient()
	if err != nil {
		return errors.WithStack(err)
	}

	ctx := context.Background()
	opts := types.ImageBuildOptions{
		Dockerfile:  "Dockerfile",
		Platform:    "linux/amd64",
		Remove:      true,
		ForceRemove: true,
		Tags:        []string{"cifuzz"},
	}
	res, err := dockerClient.ImageBuild(ctx, imageTar, opts)
	if err != nil {
		return errors.WithStack(err)
	}
	defer res.Body.Close()

	if viper.GetBool("verbose") {
		fd, isTerminal := term.GetFdInfo(os.Stderr)
		err = jsonmessage.DisplayJSONMessagesStream(res.Body, os.Stderr, fd, isTerminal, nil)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		// Read messages from the docker daemon
		scanner := bufio.NewScanner(res.Body)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			// If scanner.Text matches a regex for "Step X/Y" then extract the current step X and all steps Y
			stepRegex := regexp.MustCompile(`^{"stream":"Step (?P<currentStep>\d+)/(?P<totalSteps>\d+) : `)
			if stepRegex.MatchString(scanner.Text()) {
				matches := stepRegex.FindStringSubmatch(scanner.Text())
				stepString := fmt.Sprintf("%s (Step %s/%s)", log.ContainerBuildInProgressMsg, matches[1], matches[2])
				log.UpdateCurrentProgressSpinner(stepString)
			}
		}
	}

	log.Debugf("Created fuzz container image with tags %s", opts.Tags)
	return nil
}

// creates a tar archive that can be used for building an image
// based on a given directory
func createImageTar(buildContextDir string) (*os.File, error) {
	imageTar, err := os.CreateTemp("", "*_image.tar")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer imageTar.Close()

	writer := archive.NewTarArchiveWriter(imageTar, false)
	defer writer.Close()
	err = writer.WriteDir("", buildContextDir)
	if err != nil {
		return nil, err
	}

	// the client.BuildImage from docker expects an unclosed io.Reader / os.File
	return os.Open(imageTar.Name())
}

func createDockerfile(path string, baseImage string) error {
	cifuzzImage := cifuzzImageBase + ":" + version.Version

	dockerConfig := dockerfileConfig{
		CIFuzzImage: cifuzzImage,
		Base:        baseImage,
	}
	tmpl, err := template.New("Dockerfile").Parse(dockerfileTemplate)
	if err != nil {
		return errors.WithStack(err)
	}
	dockerfile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o655)
	if err != nil {
		return errors.WithStack(err)
	}
	err = tmpl.Execute(dockerfile, dockerConfig)
	if err != nil {
		return errors.WithStack(err)
	}
	err = dockerfile.Close()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func copyCifuzz(buildContextDir string) error {
	// Add the CIFuzz binaries to the bundle if the version is "dev".
	// TODO: this should work even if internal doesn't exist
	exists, err := fileutil.Exists("../../build/bin")
	if exists && err == nil {

		dest := filepath.Join(buildContextDir, "internal", "cifuzz_binaries")
		src := "../../build/bin"
		err = copy.Copy(src, dest)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	ensureCifuzzScriptPath := filepath.Join(buildContextDir, "ensure-cifuzz.sh")
	err = os.WriteFile(ensureCifuzzScriptPath, []byte(ensureCifuzzScript), 0o755)
	if err != nil {
		return err
	}

	return nil
}
