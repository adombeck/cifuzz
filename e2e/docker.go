package e2e

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func prepareDockerfile(t *testing.T, testCase *TestCase) string {
	t.Helper()
	var baseImage string
	switch runtime.GOOS {
	case "linux", "darwin":
		baseImage = "ubuntu:latest"
	case "windows":
		baseImage = "mcr.microsoft.com/windows/servercore:ltsc2022"
	default:
		t.Fatal("unsupported OS")
	}
	dockerfile := "FROM " + baseImage + "\n"
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" { // Base image ships without certificates, so we can't call CI Sense API
		dockerfile += "RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates\n"
	}
	if len(testCase.ToolsRequired) > 0 {
		dockerfile += getDockerfileLinesForRequiredTools(testCase.ToolsRequired)
	}
	t.Log("Dockerfile used:\n", dockerfile)
	return dockerfile
}

func toDockerImageTag(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "/", "-"))
}

func buildImageFromDockerFile(t *testing.T, ctx context.Context, dockerClient *client.Client, testCase *TestCase) string {
	t.Helper()
	dockerfile := prepareDockerfile(t, testCase)
	dockerFolder := shared.CopyTestDockerDirForE2E(t, dockerfile)

	imageTar, err := container.CreateImageTar(dockerFolder)
	require.NoError(t, err)

	tag := fmt.Sprintf("cifuzz-e2e-test-%s:latest", toDockerImageTag(t.Name()))
	opts := types.ImageBuildOptions{
		Dockerfile:  "Dockerfile",
		Remove:      true,
		ForceRemove: true,
		Tags:        []string{tag},
	}

	res, err := dockerClient.ImageBuild(ctx, imageTar, opts)
	require.NoError(t, err)
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	t.Cleanup(func() { res.Body.Close() })
	return tag
}

func runTestCaseInContainer(t *testing.T, ctx context.Context, dockerClient *client.Client, testCase *TestCase,
	testCaseRun testCaseRunOptions, imageTag string, server string) CommandOutput {

	t.Helper()
	if testCase.CIUser != AnonymousCIUser {
		testCaseRun.args = fmt.Sprintf("--server=%s %s", server, testCaseRun.args)
	}

	installDir := shared.InstallCIFuzzInTemp(t)

	cifuzzTargetMount := "/cifuzz-install"
	coverageDirectoryPath := "/coverage/e2e"
	if runtime.GOOS == "windows" {
		cifuzzTargetMount = "C:\\cifuzz-install"
		coverageDirectoryPath = "C:\\coverage\\e2e"
	}
	cifuzzExecutablePath := filepath.Join(cifuzzTargetMount, "bin", "cifuzz")
	if runtime.GOOS == "windows" {
		cifuzzExecutablePath += ".exe"
	}

	fmt.Println("Running test:", testCase.Description)
	fmt.Println("Command:", cifuzzExecutablePath, testCaseRun.command, testCaseRun.args)
	fmt.Println(" ")

	var hostCoverageDirectoryPath string
	hostCoverageDirectoryPath, _ = testutil.SetupCoverage(t, os.Environ(), "e2e")
	testCase.Environment = append(testCase.Environment, "GOCOVERDIR="+coverageDirectoryPath)

	targetMount := "/tmp/app"
	if runtime.GOOS == "windows" {
		targetMount = "C:\\app"
	}
	contextFolder := shared.CopyTestdataDirForE2E(t, testCaseRun.sampleFolder)
	t.Cleanup(func() {
		fileutil.Cleanup(contextFolder)
	})
	containerConfig := &dockerContainer.Config{
		Image:      imageTag,
		Tty:        false,
		Env:        testCase.Environment,
		Cmd:        []string{cifuzzExecutablePath},
		WorkingDir: targetMount,
	}
	containerConfig.Cmd = append(containerConfig.Cmd, strings.Split(testCaseRun.command, " ")...)

	if len(testCaseRun.args) > 0 {
		containerConfig.Cmd = append(containerConfig.Cmd, strings.Split(testCaseRun.args, " ")...)
	}

	containerConfig.Cmd = deleteEmptyStringsFromSlice(containerConfig.Cmd)

	containerBinds := []string{
		installDir + ":" + cifuzzTargetMount,
		contextFolder + ":" + targetMount,
		hostCoverageDirectoryPath + ":" + coverageDirectoryPath,
	}

	for _, tool := range testCase.ToolsRequired {
		// Mount Docker socket only when required
		if tool == "docker" {
			if runtime.GOOS == "windows" {
				containerBinds = append(containerBinds, "//./pipe/docker_engine://./pipe/docker_engine")
			} else {
				containerBinds = append(containerBinds, "/var/run/docker.sock:/var/run/docker.sock")
			}
		}
	}

	cont, err := dockerClient.ContainerCreate(
		ctx,
		containerConfig,
		&dockerContainer.HostConfig{
			Binds:      containerBinds,
			ExtraHosts: []string{"host.docker.internal:host-gateway"},
		},
		nil,
		nil,
		"", // TODO: should the container have a name?
	)
	require.NoError(t, err)

	err = dockerClient.ContainerStart(ctx, cont.ID, types.ContainerStartOptions{})
	require.NoError(t, err)

	var exitCode int64
	statusCh, errCh := dockerClient.ContainerWait(ctx, cont.ID, dockerContainer.WaitConditionNotRunning)
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	out, err := dockerClient.ContainerLogs(ctx, cont.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	require.NoError(t, err)

	containerStdOut := new(bytes.Buffer)
	containerStdErr := new(bytes.Buffer)
	_, err = stdcopy.StdCopy(containerStdOut, containerStdErr, out)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	stdOut := containerStdOut.String()
	stdErr := containerStdErr.String()

	return CommandOutput{
		ExitCode: int(exitCode),
		Stdout:   stdOut,
		Stderr:   stdErr,
		Stdall:   stdOut + stdErr,
		Workdir:  os.DirFS(contextFolder),
		t:        t,
	}
}

func deleteEmptyStringsFromSlice(s []string) []string {
	n := 0
	for _, str := range s {
		if str != "" {
			s[n] = str
			n++
		}
	}
	return s[:n]
}
