# Contributing

## Release process

We are using a automatic release process based on conventional commits
and semantic versioning.

## Building from Source (Linux / macOS)

### Prerequisites

#### Build dependencies:

- [git](https://git-scm.com/)
- [go >= 1.21](https://go.dev/doc/install)
- [libcap](https://man7.org/linux/man-pages/man3/libcap.3.html)
- Java JDK >= 8 (e.g. [OpenJDK](https://openjdk.java.net/install/) or
  [Zulu](https://www.azul.com/downloads/zulu-community/))
- [Maven](https://maven.apache.org/install.html)

#### Test dependencies:

- [LLVM >= 14](https://clang.llvm.org/get_started.html)
- [make](https://www.gnu.org/software/make/)
- [CMake >= 3.21](https://cmake.org/)
- [Bazel >= 5.3.2](https://bazel.build/install)
- Java JDK >= 8 (e.g. [OpenJDK](https://openjdk.java.net/install/) or
  [Zulu](https://www.azul.com/downloads/zulu-community/))
- [Maven](https://maven.apache.org/install.html)
- [Gradle](https://gradle.org/install/) >= 6.1

#### Dev Tooling

- [Node.js/npm](https://nodejs.org/)

### Ubuntu / Debian

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo apt install git make cmake clang llvm golang-go libcap-dev default-jdk maven gradle nodejs

# install bazelisk
sudo curl -L https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-amd64 -o /usr/local/bin/bazel
sudo chmod +x /usr/local/bin/bazel
```

### Arch

<!-- when changing this, please make sure it is in sync with the E2E pipeline -->

```bash
sudo pacman -S git make cmake clang llvm go jdk-openjdk maven gradle nodejs npm

# install bazelisk
sudo curl -L https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-amd64 -o /usr/local/bin/bazel
sudo chmod +x /usr/local/bin/bazel
```

Unfortunately, the Arch `libcap` package does not include the static
libcap library, which is needed to build cifuzz. You have to build it from
source instead:

```bash
pacman -Sy --noconfirm glibc pam linux-api-headers make diffutils
git clone git://git.kernel.org/pub/scm/libs/libcap/libcap.git
cd libcap
git checkout libcap-2.65
make
make install
```

### macOS

<!-- when changing this, please make sure it`is in sync with the E2E pipeline -->

```bash
brew install git cmake llvm lcov go openjdk maven gradle bazelisk node
```

Finally, add the following to your `~/.zshrc` or `~/.bashrc` to use the correct version of
LLVM:

```bash
export PATH=$(brew --prefix)/opt/llvm/bin:$PATH
export LDFLAGS="-L$(brew --prefix)/opt/llvm/lib"
export CPPFLAGS="-I$(brew --prefix)/opt/llvm/include"
```

## Steps

To build **cifuzz** from source you have to execute the following steps:

```bash
git clone https://github.com/CodeIntelligenceTesting/cifuzz.git
cd cifuzz
make test
make install
```

To verify the installation we recommend you to start a fuzzing run
in one of our example projects:

```bash
cd examples/cmake
cifuzz run my_fuzz_test
```

This should stop after a few seconds with an actual finding.

### Windows

We use symlinks in our repository. Developer mode needs to be
activated in order to create symlinks without admin privileges.
It can be activated in the system settings under:

```
Settings > Update & Security > For developers > Developer Mode
```
