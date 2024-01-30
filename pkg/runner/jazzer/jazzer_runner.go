package jazzer

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/options"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	fuzzer_runner "code-intelligence.com/cifuzz/pkg/runner"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type RunnerOptions struct {
	LibfuzzerOptions              *libfuzzer.RunnerOptions
	AutofuzzTarget                string
	TargetClass                   string
	TargetMethod                  string
	ClassPaths                    []string
	InstrumentationPackageFilters []string
}

func (options *RunnerOptions) ValidateOptions() error {
	err := options.LibfuzzerOptions.ValidateOptions()
	if err != nil {
		return err
	}

	if options.AutofuzzTarget == "" && options.TargetClass == "" {
		return errors.New("Either a autofuzz target or a target class must be specified")
	}
	if options.AutofuzzTarget != "" && options.TargetClass != "" {
		return errors.New("Only specify either an autofuzz target or a target class")
	}

	return nil
}

type Runner struct {
	*RunnerOptions
	*libfuzzer.Runner
}

func NewRunner(options *RunnerOptions) *Runner {
	libfuzzerRunner := libfuzzer.NewRunner(options.LibfuzzerOptions)
	libfuzzerRunner.SupportJazzer = true
	libfuzzerRunner.SupportJazzerJS = false
	return &Runner{options, libfuzzerRunner}
}

func (r *Runner) Run(ctx context.Context) error {
	err := r.ValidateOptions()
	if err != nil {
		return err
	}

	classPath := strings.Join(r.ClassPaths, string(os.PathListSeparator))

	javaBin, err := runfiles.Finder.JavaPath()
	if err != nil {
		return err
	}

	// Print version information for debugging purposes
	err = r.printDebugVersionInfos(classPath, javaBin)
	if err != nil {
		return err
	}

	args := []string{javaBin}

	// class path
	args = append(args, "-cp", classPath)

	// JVM tuning args
	// See https://github.com/CodeIntelligenceTesting/jazzer/blob/main/docs/common.md#recommended-jvm-options
	args = append(args,
		// Preserve and emit stack trace information even on hot paths.
		// This may hurt performance, but also helps find flaky bugs.
		"-XX:-OmitStackTraceInFastThrow",
		// Optimize GC for high throughput rather than low latency.
		"-XX:+UseParallelGC",
		// CriticalJNINatives has been removed in JDK 18.
		"-XX:+IgnoreUnrecognizedVMOptions",
		// Improves the performance of Jazzer's tracing hooks.
		"-XX:+CriticalJNINatives",
		// Disable warnings caused by the use of Jazzer's Java agent on JDK 21+.
		"-XX:+EnableDynamicAgentLoading",
	)

	// Jazzer main class
	args = append(args, options.JazzerMainClass)

	// ----------------------
	// --- Jazzer options ---
	// ----------------------
	if r.AutofuzzTarget != "" {
		args = append(args, options.JazzerAutoFuzzFlag(r.AutofuzzTarget))
	} else {
		args = append(args, options.JazzerTargetClassFlag(r.TargetClass))
		args = append(args, options.JazzerTargetMethodFlag(r.TargetMethod))
	}
	// -------------------------
	// --- libfuzzer options ---
	// -------------------------
	// Tell libfuzzer to exit after the timeout but only add the argument if the timeout is not 0 otherwise it will
	// override jazzer's default timeout and never stop
	timeoutSeconds := int64(r.Timeout.Seconds())
	if timeoutSeconds > 0 {
		timeoutStr := strconv.FormatInt(timeoutSeconds, 10)
		args = append(args, options.LibFuzzerMaxTotalTimeFlag(timeoutStr))
	}

	// Tell libfuzzer which dictionary it should use
	if r.Dictionary != "" {
		args = append(args, options.LibFuzzerDictionaryFlag(r.Dictionary))
	}

	// Add user-specified Jazzer/libfuzzer options
	args = append(args, r.EngineArgs...)

	// Tell Jazzer which corpus directory it should use, if specified.
	// By default, Jazzer stores the generated corpus in
	// .cifuzz-corpus/<test class name>/<test method name>.
	if r.GeneratedCorpusDir != "" {
		args = append(args, r.GeneratedCorpusDir)
	}

	// Add any additional corpus directories as further positional arguments
	args = append(args, r.SeedCorpusDirs...)

	// Set the directory in which fuzzing artifacts (e.g. crashes) are
	// stored. This must be an absolute path, because else crash files
	// are created in the current working directory, which the fuzz test
	// could change, causing the parser to not find the crash files.
	outputDir, err := os.MkdirTemp("", "jazzer-out-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer fileutil.Cleanup(outputDir)
	args = append(args, options.LibFuzzerArtifactPrefixFlag(outputDir+"/"))

	// The environment we run the fuzzer in
	env, err := r.FuzzerEnvironment()
	if err != nil {
		return err
	}

	return r.RunLibfuzzerAndReport(ctx, args, env)
}

func (r *Runner) ProduceJacocoReport(ctx context.Context, outputFile string) (string, error) {
	err := r.ValidateOptions()
	if err != nil {
		return "", err
	}

	jacocoExecFile := "/tmp/jacoco.exec"
	err = r.produceJacocoExecFile(ctx, jacocoExecFile)
	if err != nil {
		return "", err
	}

	classFilesDir := "/cifuzz/runtime_deps/target/classes"

	// Find the jacococli JAR in the class paths
	jacocoCLIPattern := regexp.MustCompile(`^org\.jacoco\.cli-.*\.jar$`)
	var jacocoCLIJar string
	for _, classPath := range r.ClassPaths {
		if jacocoCLIPattern.MatchString(filepath.Base(classPath)) {
			jacocoCLIJar = classPath
			break
		}
	}
	if jacocoCLIJar == "" {
		return "", errors.New("jacococli JAR not found in class paths")
	}

	// Produce a JaCoCo XML report from the jacoco.exec file
	cmd := executil.CommandContext(ctx, "java", "-jar", jacocoCLIJar, "report", jacocoExecFile,
		"--xml", outputFile, "--classfiles", classFilesDir)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	log.Debugf("Command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))
	err = cmd.Run()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return outputFile, nil
}

func (r *Runner) produceJacocoExecFile(ctx context.Context, outputFile string) error {
	classPath := strings.Join(r.ClassPaths, string(os.PathListSeparator))

	javaBin, err := runfiles.Finder.JavaPath()
	if err != nil {
		return err
	}
	args := []string{javaBin}

	// Find the JaCoCo Java agent JAR in the class paths
	jacocoAgentRuntimePattern := regexp.MustCompile(`^org\.jacoco\.agent-.*-runtime\.jar$`)
	var jacocoAgentJar string
	for _, classPath := range r.ClassPaths {
		if jacocoAgentRuntimePattern.MatchString(filepath.Base(classPath)) {
			jacocoAgentJar = classPath
			break
		}
	}
	if jacocoAgentJar == "" {
		return errors.New("JaCoCo agent JAR not found in class paths")
	}

	// Set the Java agent
	args = append(args, fmt.Sprintf("-javaagent:%s=destfile=%s", jacocoAgentJar, outputFile))

	// class path
	args = append(args, "-cp", classPath)

	// JVM tuning args
	// See https://github.com/CodeIntelligenceTesting/jazzer/blob/main/docs/common.md#recommended-jvm-options
	args = append(args,
		// Preserve and emit stack trace information even on hot paths.
		// This may hurt performance, but also helps find flaky bugs.
		"-XX:-OmitStackTraceInFastThrow",
		// Optimize GC for high throughput rather than low latency.
		"-XX:+UseParallelGC",
		// CriticalJNINatives has been removed in JDK 18.
		"-XX:+IgnoreUnrecognizedVMOptions",
		// Improves the performance of Jazzer's tracing hooks.
		"-XX:+CriticalJNINatives",
		// Disable warnings caused by the use of Jazzer's Java agent on JDK 21+.
		"-XX:+EnableDynamicAgentLoading",
	)

	// Jazzer main class
	args = append(args, options.JazzerMainClass)

	// Add user-specified Jazzer/libfuzzer options first and set options
	// which should not be user-configurable later, because if the same
	// option is used more than once, Jazzer respects the last occurrence.
	args = append(args, r.EngineArgs...)

	// ----------------------
	// --- Jazzer options ---
	// ----------------------
	if r.AutofuzzTarget != "" {
		args = append(args, options.JazzerAutoFuzzFlag(r.AutofuzzTarget))
	} else {
		args = append(args, options.JazzerTargetClassFlag(r.TargetClass))
		args = append(args, options.JazzerTargetMethodFlag(r.TargetMethod))
	}

	// Tell Jazzer to not apply fuzzing instrumentation, because we only
	// want to run the inputs from the corpus directories to produce
	// coverage data.
	args = append(args, options.JazzerHooksFlag(false))

	// Tell Jazzer to continue on findings, because we want to collect
	// coverage for all the inputs and not stop on findings.
	args = append(args, options.JazzerKeepGoingFlag(math.MaxInt32))

	// The --keep_going flag requires --dedup to be set as well
	args = append(args, options.JazzerDedupFlag(true))

	// -------------------------
	// --- libfuzzer options ---
	// -------------------------

	// Tell libFuzzer to never stop on timeout (but only when all inputs
	// were used).
	args = append(args, options.LibFuzzerMaxTotalTimeFlag("0"))

	// Only run the inputs from the corpus directories
	args = append(args, "-runs=0")

	// Tell Jazzer which corpus directory it should use, if specified.
	// By default, Jazzer stores the generated corpus in
	// .cifuzz-corpus/<test class name>/<test method name>.
	if r.GeneratedCorpusDir != "" {
		args = append(args, r.GeneratedCorpusDir)
	}

	// Add any additional corpus directories as further positional arguments
	args = append(args, r.SeedCorpusDirs...)

	// Set the directory in which fuzzing artifacts (e.g. crashes) are
	// stored. This must be an absolute path, because else crash files
	// are created in the current working directory, which the fuzz test
	// could change.
	outputDir, err := os.MkdirTemp("", "jazzer-out-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer fileutil.Cleanup(outputDir)
	args = append(args, options.LibFuzzerArtifactPrefixFlag(outputDir+"/"))

	// The environment we run the fuzzer in
	env, err := r.FuzzerEnvironment()
	if err != nil {
		return err
	}

	// Run Jazzer with the JaCoCo agent to produce a jacoco.exec file
	cmd := executil.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Debugf("Command: %s", envutil.QuotedCommandWithEnv(cmd.Args, env))

	return cmd.Run()
}

func (r *Runner) FuzzerEnvironment() ([]string, error) {
	var env []string
	var err error

	env, err = fuzzer_runner.AddEnvFlags(env, r.EnvVars)
	if err != nil {
		return nil, err
	}

	// Try to find a reasonable JAVA_HOME if none is set.
	if _, set := envutil.LookupEnv(env, "JAVA_HOME"); !set {
		javaHome, err := runfiles.Finder.JavaHomePath()
		if err != nil {
			return nil, err
		}
		env, err = envutil.Setenv(env, "JAVA_HOME", javaHome)
		if err != nil {
			return nil, err
		}
	}

	// Enable more verbose logging for Jazzer's libjvm.so search process.
	env, err = envutil.Setenv(env, "RULES_JNI_TRACE", "1")
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (r *Runner) Cleanup(ctx context.Context) {
	r.Runner.Cleanup(ctx)
}

func (r *Runner) printDebugVersionInfos(classPath string, javaBin string) error {
	jazzerVersion, err := dependencies.JazzerVersion(classPath)
	if err != nil {
		return err
	}
	junitVersion, err := dependencies.JUnitVersion(classPath)
	if err != nil {
		return err
	}
	javaVersion, err := dependencies.JavaVersion(javaBin)
	if err != nil {
		return err
	}

	log.Debugf("Jazzer version: %s", jazzerVersion)
	log.Debugf("JUnit Jupiter Engine version: %s", junitVersion)
	log.Debugf("Java version: %s", javaVersion)

	return nil
}
