package finding

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexflint/go-filemutex"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/fileutil"
)

const (
	nameCrashingInput = "crashing-input"
	nameJSONFile      = "finding.json"
	nameFindingsDir   = ".cifuzz-findings"
	lockFile          = ".lock"
)

type Finding struct {
	Origin string

	Name               string        `json:"name,omitempty"`
	Type               ErrorType     `json:"type,omitempty"`
	InputData          []byte        `json:"input_data,omitempty"`
	Logs               []string      `json:"logs,omitempty"`
	Details            string        `json:"details,omitempty"`
	HumanReadableInput string        `json:"human_readable_input,omitempty"`
	MoreDetails        *ErrorDetails `json:"more_details,omitempty"`
	Tag                string        `json:"tag,omitempty"`

	// Note: The following fields don't exist in the protobuf
	// representation used in the Code Intelligence core repository.
	CreatedAt  time.Time                `json:"created_at,omitempty"`
	InputFile  string                   `json:"input_file,omitempty"`
	StackTrace []*stacktrace.StackFrame `json:"stack_trace,omitempty"`

	seedPath string

	// We also store the name of the fuzz test that found this finding so that
	// we can show it in the finding overview.
	FuzzTest string `json:"fuzz_test,omitempty"`
}

type ErrorType string

// These constants must have this exact value (in uppercase) to be able
// to parse JSON-marshalled reports as protobuf reports which use an
// enum for this field.
const (
	ErrorTypeUnknownError     ErrorType = "UNKNOWN_ERROR"
	ErrorTypeCompilationError ErrorType = "COMPILATION_ERROR"
	ErrorTypeCrash            ErrorType = "CRASH"
	ErrorTypeWarning          ErrorType = "WARNING"
	ErrorTypeRuntimeError     ErrorType = "RUNTIME_ERROR"
)

type ErrorDetails struct {
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Description  string          `json:"description,omitempty"`
	Severity     *Severity       `json:"severity,omitempty"`
	Mitigation   string          `json:"mitigation,omitempty"`
	Links        []Link          `json:"links,omitempty"`
	OwaspDetails *ExternalDetail `json:"owasp_details,omitempty"`
	CweDetails   *ExternalDetail `json:"cwe_details,omitempty"`
}

type SeverityLevel string

const (
	SeverityLevelCritical SeverityLevel = "CRITICAL"
	SeverityLevelHigh     SeverityLevel = "HIGH"
	SeverityLevelMedium   SeverityLevel = "MEDIUM"
	SeverityLevelLow      SeverityLevel = "LOW"
)

type Severity struct {
	Level SeverityLevel `json:"description,omitempty"`
	Score float32       `json:"score,omitempty"`
}

type ExternalDetail struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type Link struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

func (f *Finding) GetDetails() string {
	if f != nil {
		return f.Details
	}
	return ""
}

func (f *Finding) GetSeedPath() string {
	if f != nil {
		return f.seedPath
	}
	return ""
}

// Exists returns whether the JSON file of this finding already exists
func (f *Finding) Exists(projectDir string) (bool, error) {
	jsonPath := filepath.Join(projectDir, nameFindingsDir, f.Name, nameJSONFile)
	return fileutil.Exists(jsonPath)
}

func (f *Finding) Save(projectDir string) error {
	findingDir := filepath.Join(projectDir, nameFindingsDir, f.Name)
	jsonPath := filepath.Join(findingDir, nameJSONFile)

	err := os.MkdirAll(findingDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	err = f.saveJSON(jsonPath)
	if err != nil {
		return err
	}

	return nil
}

func (f *Finding) saveJSON(jsonPath string) error {
	bytes, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	if err := os.WriteFile(jsonPath, bytes, 0o644); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (f *Finding) Remove(projectDir string) error {
	findingDir := filepath.Join(projectDir, nameFindingsDir, f.Name)
	err := os.RemoveAll(findingDir)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// CopyInputFileAndUpdateFinding copies the input file to the finding directory and
// the seed corpus directory and adjusts the finding logs accordingly.
func (f *Finding) CopyInputFileAndUpdateFinding(projectDir, seedCorpusDir string) error {
	// Acquire a file lock to avoid races with other cifuzz processes
	// running in parallel
	findingDir := filepath.Join(projectDir, nameFindingsDir, f.Name)
	err := os.MkdirAll(findingDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	lockFile := filepath.Join(findingDir, lockFile)
	mutex, err := filemutex.New(lockFile)
	if err != nil {
		return errors.WithStack(err)
	}
	err = mutex.Lock()
	if err != nil {
		return errors.WithStack(err)
	}

	// Actually copy the input file
	err = f.copyInputFile(projectDir, seedCorpusDir)

	// Release the file lock
	unlockErr := mutex.Close()
	if err == nil {
		return errors.WithStack(unlockErr)
	}
	if unlockErr != nil {
		log.Error(unlockErr)
	}
	return err
}

func (f *Finding) copyInputFile(projectDir, seedCorpusDir string) error {
	findingDir := filepath.Join(projectDir, nameFindingsDir, f.Name)
	path := filepath.Join(findingDir, nameCrashingInput)

	// Copy the input file to the finding dir. We don't use os.Rename to
	// avoid errors when source and target are not on the same mounted
	// filesystem.
	err := copy.Copy(f.InputFile, path)
	if err != nil {
		return errors.WithStack(err)
	}

	// Copy the input file to the seed corpus dir.
	err = os.MkdirAll(seedCorpusDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	// Different inputs can result in the same finding, so we append the
	// original basename to avoid basename collisions.
	f.seedPath = filepath.Join(seedCorpusDir, f.Name+"-"+filepath.Base(f.InputFile))
	err = copy.Copy(f.InputFile, f.seedPath)
	if err != nil {
		return errors.WithStack(err)
	}
	log.Debugf("Copied input file from %s to %s", f.InputFile, f.seedPath)

	// Replace the old filename in the finding logs. Replace it with the
	// relative path to not leak the directory structure of the current
	// user in the finding logs (which might be shared with others).
	cwd, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	relPath, err := filepath.Rel(cwd, path)
	if err != nil {
		return errors.WithStack(err)
	}
	for i, line := range f.Logs {
		f.Logs[i] = strings.ReplaceAll(line, f.InputFile, relPath)
	}
	log.Debugf("Copied input file from %s to %s", f.InputFile, path)

	// The path in the InputFile field is expected to be relative to the
	// project directory
	pathRelativeToProjectDir, err := filepath.Rel(projectDir, path)
	if err != nil {
		return errors.WithStack(err)
	}
	f.InputFile = pathRelativeToProjectDir
	return nil
}

func (f *Finding) SourceLocation() string {
	if f.StackTrace != nil && len(f.StackTrace) > 0 {
		stackFrame := f.StackTrace[0]
		// in some cases ASan/Libfuzzer do not include the column in the stack trace
		if stackFrame.Column != 0 {
			return fmt.Sprintf("%s:%d:%d", stackFrame.SourceFile, stackFrame.Line, stackFrame.Column)
		} else {
			return fmt.Sprintf("%s:%d", stackFrame.SourceFile, stackFrame.Line)
		}
	}
	return "n/a"
}

func (f *Finding) ShortDescriptionWithName() string {
	return fmt.Sprintf("[%s] %s", f.Name, f.ShortDescription())
}

func (f *Finding) ShortDescription() string {
	return strings.Join(f.ShortDescriptionColumns(), " ")
}

func (f *Finding) ShortDescriptionColumns() []string {
	var columns []string

	// TODO this is just a naive approach to get some error types.
	// This should be replace as soon as we have a list of the different error types.
	var errorType string
	switch f.Type {
	case ErrorTypeCrash:
		switch {
		case f.Details == "detected memory leaks":
			// Special vulnerabilities
			errorType = f.Details
		case strings.Contains(f.Details, "Security Issue:"):
			// Jazzer findings
			errorType = f.Details
		case f.Details == "fuzz target exited":
			// Jazzer.js findings
			errorType = f.Details
		default:
			errorType = strings.ReplaceAll(strings.Split(f.Details, " ")[0], "-", " ")
		}
	case ErrorTypeRuntimeError:
		errorType = strings.Split(f.Details, ":")[0]
	default:
		errorType = f.Details
	}

	columns = append(columns, errorType)

	// add location (file, function, line)
	if len(f.StackTrace) > 0 {
		st := f.StackTrace[0]
		location := f.SourceLocation()
		if st.Function != "" {
			columns = append(columns, fmt.Sprintf("in %s (%s)", st.Function, location))
		} else {
			columns = append(columns, fmt.Sprintf("in %s", location))
		}
	}
	return columns
}

// LocalFindings parses the JSON files of all findings and returns the
// result.
func LocalFindings(projectDir string, errorDetails []*ErrorDetails) ([]*Finding, error) {
	findingsDir := filepath.Join(projectDir, nameFindingsDir)
	entries, err := os.ReadDir(findingsDir)
	if os.IsNotExist(err) {
		return []*Finding{}, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var res []*Finding
	for _, e := range entries {
		f, err := LoadFinding(projectDir, e.Name(), errorDetails)
		if err != nil {
			return nil, err
		}
		res = append(res, f)
	}

	// Sort the findings by date, starting with the newest
	sort.SliceStable(res, func(i, j int) bool {
		return res[i].CreatedAt.After(res[j].CreatedAt)
	})

	return res, nil
}

// LoadFinding parses the JSON file of the specified finding and returns
// the result.
// If the specified finding does not exist, a NotExistError is returned.
// If the user is logged in, the error details are added to the finding.
func LoadFinding(projectDir, findingName string, errorDetails []*ErrorDetails) (*Finding, error) {
	findingDir := filepath.Join(projectDir, nameFindingsDir, findingName)
	jsonPath := filepath.Join(findingDir, nameJSONFile)
	bytes, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return nil, WrapNotExistError(err)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var f Finding
	err = json.Unmarshal(bytes, &f)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	f.Origin = "Local"
	f.EnhanceWithErrorDetails(errorDetails)

	return &f, nil
}

// EnhanceWithErrorDetails adds more details to the finding by parsing the
// error details file.
func (f *Finding) EnhanceWithErrorDetails(errorDetails []*ErrorDetails) {
	if errorDetails == nil {
		return
	}
	for _, d := range errorDetails {
		if (f.MoreDetails != nil && f.MoreDetails.ID == d.ID) ||
			strings.Contains(
				strings.ToLower(f.ShortDescriptionColumns()[0]),
				strings.ToLower(d.Name)) {

			// Store the error details but keep the original ID
			var originalID string
			if f.MoreDetails != nil {
				originalID = f.MoreDetails.ID
			}

			f.MoreDetails = d

			if originalID != "" {
				f.MoreDetails.ID = originalID
			}
			return
		}
	}

	log.Debugf("No error details found for finding %s", f.Name)
}
