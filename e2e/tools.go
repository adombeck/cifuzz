package e2e

import "runtime"

var toolsAvailable = map[string]func() string{
	"java": func() string {
		if runtime.GOOS == "windows" {
			return "RUN choco install -y microsoft-openjdk --version=17.0.6"
		}
		return "RUN apt-get install -y --no-install-recommends openjdk-8-jdk"
	},
	"maven": func() string {
		if runtime.GOOS == "windows" {
			return "RUN choco install -y maven"
		}
		return "RUN apt-get install -y --no-install-recommends maven"
	},
	// When "docker" tool is required, the docker socket gets mounted to a container
}

func getDockerfileLinesForRequiredTools(toolNames []string) string {
	var lines string
	if runtime.GOOS == "windows" { // When needed, install chocolatey on Windows for installing test dependencies
		lines += "RUN powershell Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))\n"
	}
	for _, name := range toolNames {
		if name == "docker" {
			// Docker socket is mounted separately
			continue
		}

		tool, ok := toolsAvailable[name]
		if !ok {
			// Panic because this is a programming error
			panic("Tool " + name + " is not available")
		}
		lines += tool() + "\n"
	}
	return lines
}
