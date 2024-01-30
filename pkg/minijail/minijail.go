package minijail

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/fileutil"
)

const (
	// BindingsEnvVarName is an environment variable which users can
	// use to specify additional Minijail bindings. The bindings must
	// be separated by colon and can be specified in the same format
	// that is supported by minijail's --bind-mount flag:
	// <src>[,[dest][,<writeable>]], where <src> must be an absolute
	// path and <writeable> is either 0 or 1.
	BindingsEnvVarName = "CIFUZZ_MINIJAIL_BINDINGS"

	// Mount flags as defined in golang.org/x/sys/unix. We're not using
	// that package because it's not available on macOS.
	MS_RDONLY      = 0x1       //nolint:all
	MS_NOSUID      = 0x2       //nolint:all
	MS_NODEV       = 0x4       //nolint:all
	MS_BIND        = 0x1000    //nolint:all
	MS_REC         = 0x4000    //nolint:all
	MS_STRICTATIME = 0x1000000 //nolint:all
)

type WritableOption int

const (
	ReadOnly WritableOption = iota
	ReadWrite
)

type Binding struct {
	Source   string
	Target   string
	Writable WritableOption
}

func (b *Binding) String() string {
	if b.Target == "" {
		b.Target = b.Source
	}
	if b.Writable == ReadWrite {
		return fmt.Sprintf("%s,%s,1", b.Source, b.Target)
	}
	// Don't use a short form if the source or target contain a comma,
	// which would be interpreted as separators by minijail.
	if strings.ContainsRune(b.Source, ',') || strings.ContainsRune(b.Target, ',') {
		return fmt.Sprintf("%s,%s,0", b.Source, b.Target)
	}
	if b.Source != b.Target {
		return fmt.Sprintf("%s,%s", b.Source, b.Target)
	}
	return b.Source
}

func BindingFromString(s string) (*Binding, error) {
	tokens := strings.SplitN(s, ",", 3)
	switch len(tokens) {
	case 1:
		return &Binding{Source: tokens[0], Target: tokens[0], Writable: 0}, nil
	case 2:
		return &Binding{Source: tokens[0], Target: tokens[1], Writable: 0}, nil
	case 3:
		writable, err := strconv.Atoi(tokens[2])
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return &Binding{Source: tokens[0], Target: tokens[1], Writable: WritableOption(writable)}, nil
	}
	return nil, errors.Errorf("Bad binding: %s", s)
}

var fixedMinijailArgs = []string{
	// Most of these args are the same as the ones clusterfuzz sets in
	// their minijail wrapper:
	// https://github.com/google/clusterfuzz/blob/4f8020c4c7ce73c1da0e68f04943af30bb5f0b32/src/clusterfuzz/_internal/system/minijail.py
	//
	"-U", "-m", // Quote from clusterfuzz:
	// root (uid 0 in namespace) -> USER.
	// The reason for this is that minijail does setresuid(0, 0, 0) before doing a
	// chroot, which means uid 0 needs access to the chroot dir (owned by USER).
	//
	// Note that we also run fuzzers as uid 0 (but with no capabilities in
	// permitted/effective/inherited sets which *should* mean there"s nothing
	// special about it). This is because the uid running the fuzzer also need
	// access to things owned by USER (fuzzer binaries, supporting files), and USER
	// can only be mapped once.
	"-M",      // Map current gid to root
	"-c", "0", // drop all capabilities.
	"-n", // no_new_privs
	"-v", // mount namespace
	"-p", // PID namespace
	"-l", // IPC namespace
	"-I", // Run jailed process as init.
	// Mount the whole filesystem read-only. All paths which should be
	// writable have to be added explicitly as read-write bindings.
	"-k", "/,/,none," + strconv.Itoa(MS_RDONLY|MS_BIND|MS_REC),
	// Mount a new procfs on /proc
	"-k", "proc,/proc,proc," + strconv.Itoa(MS_RDONLY),
	// Mount a new tmpfs on /dev/shm
	"-k", "tmpfs,/dev/shm,tmpfs," + strconv.Itoa(MS_NOSUID|MS_NODEV|MS_STRICTATIME) + ",mode=1777",
	// Applications generally assume that /tmp is writable, so we mount
	// a tmpfs on /tmp.
	// Note that this causes paths below /tmp which are printed by the
	// application not being accessible on the host. The alternative
	// would be to mount the /tmp from the host read-writable, but that
	// could cause PID file collisions.
	"-k", "tmpfs,/tmp,tmpfs," + strconv.Itoa(MS_NOSUID|MS_NODEV|MS_STRICTATIME) + ",mode=1777",
	// Same as for /tmp, /run and /var/run should be writable
	"-k", "tmpfs,/run,tmpfs," + strconv.Itoa(MS_NOSUID|MS_NODEV|MS_STRICTATIME) + ",mode=1777",
	"-k", "tmpfs,/var/run,tmpfs," + strconv.Itoa(MS_NOSUID|MS_NODEV|MS_STRICTATIME) + ",mode=1777",
	// Added by us, to log to stderr
	"--logging=stderr",
}

var defaultBindings = []*Binding{
	// We allow access to /dev/null and /dev/urandom because AFL needs
	// access to them and some fuzz targets might as well (for example
	// our lighttpd example fuzz target).
	// They have to be mounted read-write, else minijail fails with
	// libminijail[1]: cannot bind-remount: [...] Operation not permitted
	{Source: "/dev/null", Writable: ReadWrite},
	{Source: "/dev/urandom", Writable: ReadWrite},
}

type Options struct {
	Args      []string
	Bindings  []*Binding
	OutputDir string
}

type minijail struct {
	*Options
	Args      []string
	chrootDir string
}

func NewMinijail(opts *Options) (*minijail, error) {
	// Evaluate symlinks in the executable path
	path, err := filepath.EvalSymlinks(opts.Args[0])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	opts.Args[0] = path

	// --------------------------
	// --- Create directories ---
	// --------------------------
	// Create chroot directory
	chrootDir, err := os.MkdirTemp("", "minijail-chroot-")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Create /tmp, /proc directories.
	for _, dir := range []string{"/proc", "/tmp"} {
		err = os.MkdirAll(filepath.Join(chrootDir, dir), 0o755)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Create /dev/shm which is required to allow using shared memory
	err = os.MkdirAll(filepath.Join(chrootDir, "dev", "shm"), 0o755)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// ----------------------------
	// --- Set up minijail args ---
	// ----------------------------
	minijailPath, err := runfiles.Finder.Minijail0Path()
	if err != nil {
		return nil, err
	}
	minijailArgs := append([]string{minijailPath}, fixedMinijailArgs...)

	// This causes minijail to not use preload hooking, which
	// allows us to run it without the libminijailpreload.so. That has
	// two benefits:
	// * We can use a statically built minijail0 binary, avoiding runtime
	//   dependencies on libcap.
	// * It avoids that minijail0 doesn't print error messages, which
	//   happens when preloading is used.
	//
	// Note that (quoting the Minijail manual [1]): "some jailing can
	// only be achieved from the process to which they will actually
	// apply [via preloading]".
	// [1] https://google.github.io/minijail/minijail0.1.html#implementation
	//
	// Since we don't use minijail for security but only for safety
	// (i.e. we only want to protect against accidental damage done to
	// the system, like the fuzz target accidentally deleting files or
	// killing processes etc), it should be fine that the jailing is not
	// perfect.
	minijailArgs = append(minijailArgs, "-T", "static", "--ambient")

	// Change root filesystem to the chroot directory. See pivot_root(2).
	minijailArgs = append(minijailArgs, "-P", chrootDir)

	// -----------------------
	// --- Set up bindings ---
	// -----------------------
	bindings := append(opts.Bindings, defaultBindings...)

	// Allow read-write access to the minijail output directory
	if opts.OutputDir != "" {
		bindings = append(bindings, &Binding{Source: opts.OutputDir, Writable: ReadWrite})
	}

	// We expect the current working directory to be the artifacts
	// directory, which should be accessible to the fuzz target, so we
	// add a binding for it.
	// Some fuzz targets (e.g. the one for nginx) write to the working
	// directory, which is why we mount it read-write. We decided that
	// this is fine on CIFUZZ-1192.
	workdir, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bindings = append(bindings, &Binding{Source: workdir, Writable: ReadWrite})

	// Add binding for the executable
	bindings = append(bindings, &Binding{Source: path})

	// Add binding for process_wrapper. process_wrapper changes the
	// working directory and then executes the specified command.
	processWrapperPath, err := runfiles.Finder.ProcessWrapperPath()
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, &Binding{Source: processWrapperPath})

	// Add additional bindings from the environment variable
	additionalBindingsEnv := os.Getenv(BindingsEnvVarName)
	for _, s := range strings.Split(additionalBindingsEnv, ":") {
		if s == "" {
			continue
		}
		binding, err := BindingFromString(s)
		if err != nil {
			return nil, err
		}

		exists, err := fileutil.Exists(binding.Source)
		if err != nil {
			return nil, err
		}
		if !exists {
			log.Debugf("Skipping binding %v: No such file or directory", binding.Source)
			continue
		}

		log.Debugf("Adding binding %v", binding.Source)
		bindings = append(bindings, binding)
	}

	// Create the bindings
	for _, binding := range bindings {
		if binding.Target == "" {
			binding.Target = binding.Source
		}
		// Skip if the source doesn't exist
		exists, err := fileutil.Exists(binding.Source)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}

		// Create the destination
		if fileutil.IsDir(binding.Source) {
			err = os.MkdirAll(filepath.Join(chrootDir, binding.Target), 0o755)
			if err != nil {
				return nil, errors.WithStack(err)
			}
		} else {
			err = os.MkdirAll(filepath.Join(chrootDir, filepath.Dir(binding.Target)), 0o755)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			err = fileutil.Touch(filepath.Join(chrootDir, binding.Target))
			if err != nil {
				return nil, err
			}
		}

		minijailArgs = append(minijailArgs, "-b", binding.String())
	}

	// -----------------------------------
	// --- Set up process wrapper args ---
	// -----------------------------------
	// The process wrapper changes the working directory inside the
	// sandbox to the first argument
	processWrapperArgs := []string{processWrapperPath, workdir}

	// --------------------
	// --- Run minijail ---
	// --------------------
	args := append(minijailArgs, "--")
	args = append(args, processWrapperArgs...)
	args = append(args, opts.Args...)

	// When DEBUG_MINIJAIL is set, we don't execute the actual libFuzzer
	// command but only print it and start a shell instead. When used
	// together with SKIP_CLEANUP, this allows to copy the Minijail
	// command from the logs to open a shell in the sandbox environment
	// to debug issues interactively.
	if os.Getenv("DEBUG_MINIJAIL") != "" {
		log.Print("libFuzzer command: ", strings.Join(opts.Args, " "))
		args = append(minijailArgs, "--")
		args = append(args, processWrapperArgs...)
		args = append(args, "/bin/sh")
	}

	return &minijail{
		Options:   opts,
		chrootDir: chrootDir,
		Args:      args,
	}, nil
}

func (m *minijail) Cleanup() {
	fileutil.Cleanup(m.chrootDir)
}
