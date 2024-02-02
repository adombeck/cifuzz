#!/bin/bash
set -euo pipefail

# Ensure cifuzz in PATH in Docker image

# Check if cifuzz is already in PATH, because it is installed in the Base Docker Image
if command -v cifuzz &> /dev/null
then
  # Exit script
  exit 0
fi

# If available, use mounted version of cifuzz from local dev machine
# In the bundler.go check if version == "dev", then mount the build folder and get an appropriate cifuzz binary from it!

# Check if there is a /internal/cifuzz folder
if [ -d "/internal/cifuzz_binaries" ]; then
	# Select a cifuzz binary depending on whether the OS is linux or macOS
	if [ "$(uname)" == "Darwin" ]; then
		# if there is a cifuzz_macos binary, use it
		if [ -f "/internal/cifuzz_binaries/cifuzz_macOS" ]; then
			echo "Using cifuzz_macOS binary from local dev machine"
			cp /internal/cifuzz_binaries/cifuzz_macOS /usr/local/bin/cifuzz
			# Make cifuzz binary executable
			chmod +x /usr/local/bin/cifuzz
			exit 0
		fi
	elif [ "$(expr substr "$(uname -s)" 1 5)" == "Linux" ]; then
		if [ -f "/internal/cifuzz_binaries/cifuzz_linux" ]; then
			echo "Using cifuzz_linux binary from local dev machine"
			cp /internal/cifuzz_binaries/cifuzz_linux /usr/local/bin/cifuzz
			# Make cifuzz binary executable
			chmod +x /usr/local/bin/cifuzz
			exit 0
		fi
	fi
fi

# By default, install latest production version of cifuzz from our Installer script
sh -c "$(curl -fsSL https://raw.githubusercontent.com/adombeck/cifuzz/main/install.sh)"
