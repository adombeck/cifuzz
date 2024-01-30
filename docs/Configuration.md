# cifuzz configuration

You can change the behavior of **cifuzz** both via command-line flags
and via settings stored in the `cifuzz.yaml` config file. Flags take
precedence over the respective config file setting.

## cifuzz.yaml settings

[build-system](#build-system) <br/>
[build-command](#build-command) <br/>
[seed-corpus-dirs](#seed-corpus-dirs) <br/>
[dict](#dict) <br/>
[engine-args](#engine-args) <br/>
[timeout](#timeout) <br/>
[use-sandbox](#use-sandbox) <br/>
[print-json](#print-json) <br/>
[no-notifications](#no-notifications) <br/>
[server](#server) <br/>
[project](#project) <br/>
[style](#style) <br/>

<a id="build-system"></a>

### build-system

The build system used to build this project. If not set, cifuzz tries
to detect the build system automatically.
Valid values: "bazel", "cmake", "maven", "gradle", "other".

#### Example

```yaml
build-system: cmake
```

<a id="build-command"></a>

### build-command

If the build system type is "other", this command is used by
`cifuzz run` to build the fuzz test.

#### Example

```yaml
build-command: "make all"
```

<a id="seed-corpus-dirs"></a>

### seed-corpus-dirs

Directories containing sample inputs for the code under test.
See https://llvm.org/docs/LibFuzzer.html#corpus.

#### Example

```yaml
seed-corpus-dirs:
  - path/to/seed-corpus
```

<a id="dict"></a>

### dict

A file containing input language keywords or other interesting byte
sequences.
See https://llvm.org/docs/LibFuzzer.html#dictionaries.

#### Example

```yaml
dict: path/to/dictionary.dct
```

<a id="engine-args"></a>

### engine-args

Command-line arguments to pass to libFuzzer or Jazzer for running fuzz tests.
Engine-args are not supported for running `cifuzz coverage` on JVM-projects
and are not supported for Node.js projects.

For possible libFuzzer options see https://llvm.org/docs/LibFuzzer.html#options.

For advanced configuration with Jazzer parameters see https://github.com/CodeIntelligenceTesting/jazzer/blob/main/docs/advanced.md.

Fuzzer customization for Node.js projects can be specified in `.jazzerjsrc.json`
in the root project directory. See https://github.com/CodeIntelligenceTesting/jazzer.js/blob/main/docs/jest-integration.md
for further information.

#### Example Libfuzzer

```yaml
engine-args:
  - -rss_limit_mb=4096
  - -timeout=5s
```

#### Example Jazzer

```yaml
engine-args:
  - --instrumentation_includes=com.**
  - --keep_going
```

<a id="timeout"></a>

### timeout

Maximum time in seconds to run the fuzz tests. The default is to run
indefinitely.

#### Example

```yaml
timeout: 300
```

<a id="use-sandbox"></a>

### use-sandbox

By default, fuzz tests are executed in a sandbox to prevent accidental
damage to the system. Set to false to run fuzz tests unsandboxed.
Only supported on Linux.

#### Example

```yaml
use-sandbox: false
```

<a id="print-json"></a>

### print-json

Set to true to print output of the `cifuzz run` command as JSON.

#### Example

```yaml
print-json: true
```

### no-notifications

Set to true to disable desktop notifications

#### Example

```yaml
no-notifications: true
```

### server

Set URL of CI Sense

#### Example

```yaml
server: https://app.code-intelligence.com
```

### project

Set the project name of CI Sense project

#### Example

```yaml
project: my-project-1a2b3c4d
```

### style

Choose the style to run cifuzz in

- `pretty`: Colored output and icons (default)
- `color`: Colored output
- `plain`: Pure text without any styles

#### Example

```yaml
style: plain
```
