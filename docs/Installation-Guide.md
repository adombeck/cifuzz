# Installation Guide

## Installation Directories

### Linux/MacOS

When running the installer as a **non-root** user, files are installed to:

- `~/.local/share/cifuzz` (default) or
- `$XDG_DATA_HOME/cifuzz` if `$XDG_DATA_HOME` is set.

A symlink to the `cifuzz` executable is created in `~/.local/bin/cifuzz`.

When running the installer as **root**, files are installed to
`/opt/code-intelligence/cifuzz` and a symlink to the `cifuzz` executable
if created in `/usr/local/bin/cifuzz`.

### Windows

All files are installed to `%APPDATA%/cifuzz` with the executable located
in `%APPDATA%/cifuzz/bin`.

## How to uninstall cifuzz

### Linux / macOS

#### Version < 0.7.0

If you installed cifuzz into the default directory as **root**:

```bash
sudo rm -rf ~/cifuzz /usr/local/share/cifuzz
```

If you installed cifuzz as a **non-root** user:

```bash
rm -rf ~/cifuzz ~/.cmake/packages/cifuzz
```

If you installed into a custom installation directory you have to remove
that one instead.

#### Version >= 0.7.0

From version 0.7.0 the default installation directory has changed.

If you installed cifuzz as **root**:

```bash
sudo rm -rf /opt/code-intelligence/cifuzz /usr/local/bin/cifuzz /usr/local/share/cifuzz
```

If you installed cifuzz as a **non-root** user:

```bash
rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/cifuzz" ~/.local/bin/cifuzz ~/.cmake/packages/cifuzz
```

If you installed into a custom installation directory you have to remove
that one instead.

### Windows

To uninstall cifuzz and delete the corresponding registry entries:

```bash
rd /s %APPDATA%\cifuzz

reg delete "HKLM\Software\Kitware\CMake\Packages\cifuzz" /f 2> nul
reg delete "HKCU\Software\Kitware\CMake\Packages\cifuzz" /f 2> nul
```

## Using a pre-release installer

The [Installer Pipeline](https://github.com/CodeIntelligenceTesting/cifuzz/actions/workflows/pipeline_installer.yml)
creates installers (for all supported operating systems) for every
change on the `main` branch and can also be used on other branches too.

To use one of these installers the following steps have to be executed:

1. Download the `cifuzz_installer` artifact from the successful pipeline run
2. Unpack the artifact, for example with `unzip cifuzz_installer.zip`
3. Chose the installer for your operating system and run it

```
chmod u+x cifuzz_installer_linux
./cifuzz_installer_linux
```
