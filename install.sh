#!/bin/sh
set -e

BASE_URL="https://github.com/adombeck/cifuzz/releases/latest/download/"

# detect os to get the correct url for the installer
case $(uname -s) in
  Linux*)
    INSTALLER="cifuzz_installer_linux_amd64" ;;

  Windows*|MINGW*|MSYS*|CYGWIN*)
    INSTALLER="cifuzz_installer_windows_amd64.exe" ;;

  Darwin*)
    if [ "$(uname -m)" = 'arm64' ]; then
      INSTALLER="cifuzz_installer_macOS_arm64"
    else
      INSTALLER="cifuzz_installer_macOS_amd64"
    fi ;;

  *)
    echo "No installer available for this operating system '$(uname -s) $(uname -m)'"
    exit 1 ;;
esac

DOWNLOAD_URL="${BASE_URL}${INSTALLER}"
printf '%s' "Downloading installer... "
curl --fail --silent --show-error --location -o cifuzz_installer "${DOWNLOAD_URL}"
echo "ok"

echo "Starting installer..."
chmod u+x cifuzz_installer
./cifuzz_installer
