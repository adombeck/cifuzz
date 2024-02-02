## What is cifuzz

**cifuzz** is a CLI tool that helps you to integrate and run fuzzing
based tests into your project.

## Features

- Easily set up, create and run fuzz tests
- Generate coverage reports that [can be integrated in your
  IDE](docs/Coverage-ide-integrations.md)
- Supports multiple programming languages and build systems

![CLion](/docs/assets/tools/clion.png)
![IDEA](/docs/assets/tools/idea.png)
![VSCode](/docs/assets/tools/vscode.png)
![C++](/docs/assets/tools/cpp.png)
![Java](/docs/assets/tools/java.png)
![Android](/docs/assets/tools/android.png)
![CMake](/docs/assets/tools/cmake.png)
![gradle](/docs/assets/tools/gradle.png)
![Maven](/docs/assets/tools/maven.png)
![Bazel](/docs/assets/tools/bazel.png)

## Getting started

All you need to get started with fuzzing are these three simple commands:

```elixir
$ cifuzz init            # initialize your project
$ cifuzz create          # create a simple fuzz test to start from
$ cifuzz run myfuzztest  # run the fuzz test
```

![CLI showcase](/docs/assets/showcase.gif)

If you are new to the world of fuzzing, we recommend you to take a
look at our [Glossary](docs/Glossary.md) and our
[example projects](examples/).

**Read the [getting started guide](docs/Getting-Started.md) if you just want to
learn how to fuzz your applications with cifuzz.**

## Installation

You can get the
[latest release from GitHub](https://github.com/adombeck/cifuzz/releases/latest)
or by running our install script:

```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/adombeck/cifuzz/main/install.sh)"
```

If you are using Windows you can download
the [latest release](https://github.com/adombeck/cifuzz/releases/latest/download/cifuzz_installer_windows_amd64.exe)
and execute it.

Do not forget to add the installation's `bin` directory to your `PATH`.
You can find additional information in our [Installation Guide](docs/Installation-Guide.md).

### Prerequisites

Depending on your language / build system of choice **cifuzz** has
different prerequisites:

<details>
 <summary>C/C++ with CMake</summary>

- [CMake >= 3.16](https://cmake.org/)
- [LLVM >= 11](https://clang.llvm.org/get_started.html)

**Ubuntu / Debian**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo apt install cmake clang llvm lcov
```

**Arch**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo pacman -S cmake clang llvm lcov
```

**macOS**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
brew install cmake llvm lcov
```

**Windows**

At least Visual Studio 2022 version 17 is required.

Please make sure to

- select **"Develop Desktop C++ applications"** in the Visual Studio Installer
- check **"C++ Clang Compiler for Windows"** in the "Individual Components" tab
- check **"C++ CMake Tools for Windows"** in the "Individual Components" tab
- check **"MSBuild support for LLVM (clang-cl) toolset"** in the "Individual Components" tab

You can add these components anytime by choosing "Modify" in the Visual Studio Installer.

```bash
choco install lcov
```

You may have to add %ChocolateyInstall%\lib\lcov\tools\bin to your PATH variable.

</details>

<details>
 <summary>C/C++ with Bazel</summary>

- [Bazel >= 5.3.2 (>=6.0.0 on macOS)](https://bazel.build/install)
- Java JDK >= 8 (1.8) (e.g. [OpenJDK](https://openjdk.java.net/install/) or
  [Zulu](https://www.azul.com/downloads/zulu-community/))
  is needed for Bazel's coverage feature.
- [LLVM >= 11](https://clang.llvm.org/get_started.html)
- [lcov](https://github.com/linux-test-project/lcov)

**Ubuntu / Debian**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo apt install clang llvm lcov default-jdk zip

# install bazelisk
sudo curl -L https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-amd64 -o /usr/local/bin/bazel
sudo chmod +x /usr/local/bin/bazel
```

**Arch**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo pacman -S clang llvm lcov python jdk-openjdk zip

# install bazelisk
sudo curl -L https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-amd64 -o /usr/local/bin/bazel
sudo chmod +x /usr/local/bin/bazel
```

**macOS**
Bazel C/C++ projects are currently not supported on macOS.

**Windows**
Bazel C/C++ projects are currently not supported on Windows.

</details>

<details>
 <summary>Java with Maven</summary>

- Java JDK >= 8 (1.8) (e.g. [OpenJDK](https://openjdk.java.net/install/) or
  [Zulu](https://www.azul.com/downloads/zulu-community/))
- [Maven](https://maven.apache.org/install.html)

**Ubuntu / Debian**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo apt install default-jdk maven
```

**Arch**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo pacman -S jdk-openjdk maven
```

**macOS**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
brew install openjdk maven
```

**Windows**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
choco install microsoft-openjdk maven
```

</details>

<details>
 <summary>Java with Gradle</summary>

- Java JDK >= 8 (1.8) (e.g. [OpenJDK](https://openjdk.java.net/install/) or
  [Zulu](https://www.azul.com/downloads/zulu-community/))
- [Gradle](https://gradle.org/install/) >= 6.1

**Ubuntu / Debian**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo apt install default-jdk gradle
```

**Arch**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo pacman -S jdk-openjdk gradle
```

**macOS**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
brew install openjdk gradle
```

**Windows**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
choco install microsoft-openjdk gradle
```

</details>

<details>
 <summary>Android</summary>

**Info:** Currently cifuzz is **not** supporting fuzz tests running in an
emulator or on a device, it is still possible to run local tests.
You can find more information and an example at
the [cifuzz-gradle-plugin](https://github.com/adombeck/cifuzz-gradle-plugin)
repository.

- [Gradle](https://gradle.org/install/) >= 7.5
- [Android Gradle Plugin](https://developer.android.com/build) >= 7.4.2

</details>

<details>
 <summary>Node.js</summary>

**Info:** Support for Node.js projects is still experimental and
hidden behind a feature flag. You can try it out by running:

```bash
export CIFUZZ_PRERELEASE=1
```

- [Node.js](https://nodejs.org) >= 16.0

**Ubuntu / Debian**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo apt install nodejs
```

**Arch**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo pacman -S nodejs
```

**macOS**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
brew install nodejs
```

**Windows**

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
choco install nodejs
```

</details>

### Windows

In order to get font colors and glyphs to render properly install the
[Windows Terminal from the Microsoft Store](https://aka.ms/terminal).
Run `cifuzz` in `Developer PowerShell for VS 2022` inside of `Windows Terminal`.

## Limitations

**Windows**

- C/C++ projects are only supported with CMake and fuzz tests cannot depend on shared libraries.
- Continuous code coverage is not supported for C/C++ projects.

## Troubleshooting

If you encounter problems installing or running cifuzz, you can check [Troubleshooting](docs/Troubleshooting.md)
for possible solutions.

## Contributing

Want to help improve cifuzz? Check out our [contributing documentation](CONTRIBUTING.md).
There you will find instructions for building the tool locally.
