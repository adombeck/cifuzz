package stacktrace

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/java/sourcemap"
	"code-intelligence.com/cifuzz/util/regexutil"
)

var framePattern = regexp.MustCompile(
	`#(?P<frame_number>\d+)\s+0x[a-fA-F0-9]+\s+in\s+(?P<function>(\(anonymous namespace\))?[^(\s]+).*\s(?P<source_file>\S+?):(?P<line>\d+):?(?P<column>\d*)`)

// Special pattern for Java stack traces
var framePatternJava = regexp.MustCompile(`^\s*at\s+(?P<function>[^(]*)\((?P<source_file>[^:]*):(?P<line>\d*)\)\s*$`)

// Special pattern for Node stack traces
var framePatternNode = regexp.MustCompile(`\s*at\s((?P<function>\S+)\s+(\[.*\])?\s*\()?(?P<source_file>\S+?):(?P<line>\d+):?(?P<column>\d*)\)?`)

// This matches diagnostic messages printed by UBSan when it reports an
// error. UBSan doesn't always print a stack trace, so we extract the
// source file from this line.
// TODO: The <message> part contains useful information which we should
// store in the finding
var ubSanDiagPattern = regexp.MustCompile(`^(?P<source_file>\S+?):((?P<line>\d+):)?((?P<column>\d+):)? runtime error: (?P<message>.*)$`)

// A StackFrame represents an element of the stack trace
type StackFrame struct {
	SourceFile  string
	Line        uint32
	Column      uint32
	FrameNumber uint32
	Function    string
}

func EncodeStackTrace(stacktrace []*StackFrame) []byte {
	out := []byte("")
	for _, sf := range stacktrace {
		out = append(out, fmt.Sprintf("#%d|%s|%s|%d|%d", sf.FrameNumber, sf.Function, sf.SourceFile, sf.Line, sf.Column)...)
	}
	return out
}

type ParserOptions struct {
	ProjectDir      string
	SourceMap       *sourcemap.SourceMap
	SupportJazzer   bool
	SupportJazzerJS bool
}

type parser struct {
	*ParserOptions
	filterJazzerStackFrames bool
}

func NewParser(opts *ParserOptions) (*parser, error) {
	return &parser{opts, true}, nil
}

// Parse parses output from an error reported by libFuzzer or a sanitizer
// and returns a stack trace if one is found in the error report.
func (p *parser) Parse(logs []string) ([]*StackFrame, error) {
	trace, err := p.parseStackTrace(logs)
	if err != nil {
		return nil, err
	}
	if trace != nil {
		return trace, nil
	}

	// Some findings don't produce a stack trace but a single source
	// location, like this:
	//
	//    SUMMARY: UndefinedBehaviorSanitizer: undefined-behavior fuzz-targets/trigger_ubsan.cpp:5:5 in
	//
	// If no stack trace was found in the logs, we use that as a single
	// frame stack trace.
	return p.parseSourceLocation(logs)
}

func (p *parser) parseStackTrace(logs []string) ([]*StackFrame, error) {
	var frames []*StackFrame
	for _, line := range logs {
		frame, err := p.stackFrameFromLine(line)
		if err != nil {
			return nil, err
		}
		if frame == nil {
			continue
		}

		if !p.SupportJazzer && !p.SupportJazzerJS {
			// Frame numbers are only available for libfuzzer stacktraces
			if len(frames) > 0 && frame.FrameNumber <= frames[len(frames)-1].FrameNumber {
				// The frame number is lower than the one from the previous
				// frame, so this is probably a new stack trace.
				break
			}
		}

		frames = append(frames, frame)

		// LLVMFuzzerTestOneInputNoReturn is the function which is
		// defined by the user (via the FUZZ_TEST macro). We're not
		// interested in stack frames below that.
		//
		// For compatibility with fuzz tests which don't use the
		// FUZZ_TEST macro, we also stop parsing here if the function is
		// LLVMFuzzerTestOneInput.
		if frame.Function == "LLVMFuzzerTestOneInputNoReturn" || frame.Function == "LLVMFuzzerTestOneInput" {
			break
		}
	}
	return frames, nil
}

func (p *parser) parseSourceLocation(logs []string) ([]*StackFrame, error) {
	for _, line := range logs {
		sourceLocation, err := p.sourceLocationFromLine(line)
		if err != nil {
			return nil, err
		}
		if sourceLocation != nil {
			return []*StackFrame{sourceLocation}, nil
		}
	}
	return nil, nil
}

func (p *parser) stackFrameFromLine(line string) (*StackFrame, error) {
	var err error
	matches, found := regexutil.FindNamedGroupsMatch(framePattern, line)
	if !found && p.SupportJazzer {
		matches, found = regexutil.FindNamedGroupsMatch(framePatternJava, line)
		if !found {
			return nil, nil
		}
	}
	if !found && p.SupportJazzerJS {
		matches, found = regexutil.FindNamedGroupsMatch(framePatternNode, line)
		if !found {
			return nil, nil
		}
	}
	if !found {
		return nil, nil
	}

	sourceFile := p.validateSourceFile(matches["source_file"], matches["function"])
	if sourceFile == "" {
		// Not a valid source file, ignore this stack frame
		return nil, nil
	}

	var frameNumber uint64
	if matches["frame_number"] != "" {
		frameNumber, err = strconv.ParseUint(matches["frame_number"], 10, 32)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	lineNumber, err := strconv.ParseUint(matches["line"], 10, 32)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// column is not always reported
	var column uint64
	if matches["column"] != "" {
		column, err = strconv.ParseUint(matches["column"], 10, 32)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	stackFrame := &StackFrame{
		SourceFile:  filepath.ToSlash(sourceFile),
		Line:        uint32(lineNumber),
		Column:      uint32(column),
		FrameNumber: uint32(frameNumber),
		Function:    matches["function"],
	}

	return stackFrame, nil
}

func (p *parser) validateSourceFile(sourceFile string, function string) string {
	var err error

	// To make the stack trace more useful when the finding is shared
	// with others, and to avoid leaking the directory structure, we
	// make the source file path relative to the project directory.
	//
	// Note that we can't rely on the the --relativenames option of
	// llvm-symbolizer to produce relative paths in the stack trace
	// because that only produces relative paths if the compiler
	// command-line also contained relative paths to the source files.
	path := sourceFile
	if filepath.IsAbs(sourceFile) {
		path, err = filepath.Rel(p.ProjectDir, path)
		// We don't return the error here, because on Windows an error
		// is returned when the paths are on different drives (e.g. C:
		// and D:), which means that the source file is not below the
		// project directory and we handle that case below.
	}
	if err != nil || strings.HasPrefix(path, "..") {
		// The source file is not below the project directory, so it's
		// probably a third-party library. We don't store the stack
		// frames of third-party libraries, because
		// 1. We want to ignore them in the finding summary, which only
		//    includes one source file from the stack trace and we think
		//    using a source file from the project directory there makes
		//    it easier to quickly understand which finding it is and
		//    what's it about.
		// 2. We want to ignore them when deduplicating findings,
		//    because (a) we would create duplicate findings if the
		//    same fuzz test is run with and without debug symbols for
		//    third-party libraries, and (b) we assume that users want
		//    to find bugs in the code inside the project directory. If
		//    the stack frames from those source files are the same, the
		//    root cause of the findings is probably also the same, so
		//    it makes sense to deduplicate them.
		return ""
	}

	if p.SupportJazzer {
		sourceFilePath := p.getJavaSourceFilePath(sourceFile, function)
		if sourceFilePath != path {
			// If source file is found in the source map, all further
			// stack frames do not get filtered anymore
			p.filterJazzerStackFrames = false

			path = sourceFilePath
		} else if p.filterJazzerStackFrames {
			// Filter stack frame if source file is not found in the
			// source map and filterJazzerStackFrames is true
			return ""
		}
	}

	// Ignore files from node_modules
	if p.SupportJazzerJS {
		if strings.Contains(path, "node_modules") {
			return ""
		}
	}

	return path
}

func (p *parser) sourceLocationFromLine(line string) (*StackFrame, error) {
	matches, found := regexutil.FindNamedGroupsMatch(ubSanDiagPattern, line)
	if !found {
		return nil, nil
	}

	sourceFile := p.validateSourceFile(matches["source_file"], matches["function"])
	if sourceFile == "" {
		// Not a valid source file, ignore this stack frame
		return nil, nil
	}

	lineNumber, err := strconv.ParseUint(matches["line"], 10, 32)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// column is not always reported
	var column uint64
	if matches["column"] != "" {
		column, err = strconv.ParseUint(matches["column"], 10, 32)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return &StackFrame{
		SourceFile: filepath.ToSlash(sourceFile),
		Line:       uint32(lineNumber),
		Column:     uint32(column),
	}, nil
}

func (p *parser) getJavaSourceFilePath(sourceFile string, function string) string {
	// remove function and class name
	possiblePackageName := removeLastPart(removeLastPart(function))
	// In the case of nested classes we are not at the true package
	// name yet. Remove possible class suffixes until we find the
	// Java package that matches.

	for possiblePackageName != "" {
		for _, relFile := range p.SourceMap.JavaPackages[possiblePackageName] {
			if filepath.Base(relFile) == sourceFile {
				return relFile
			}
		}
		possiblePackageName = removeLastPart(possiblePackageName)
	}

	return sourceFile
}

func removeLastPart(packageName string) string {
	sepIndex := strings.LastIndex(packageName, ".")
	if sepIndex > 0 {
		return packageName[0:sepIndex]
	}
	return ""
}
