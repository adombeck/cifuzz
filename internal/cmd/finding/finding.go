package finding

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type options struct {
	PrintJSON   bool   `mapstructure:"print-json"`
	ProjectDir  string `mapstructure:"project-dir"`
	ConfigDir   string `mapstructure:"config-dir"`
	Interactive bool   `mapstructure:"interactive"`
	Server      string `mapstructure:"server"`
	Project     string `mapstructure:"project"`
}

type findingCmd struct {
	*cobra.Command
	opts *options
}

func New() *cobra.Command {
	return newWithOptions(&options{})
}

func newWithOptions(opts *options) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:               "finding [name]",
		Aliases:           []string{"findings"},
		Short:             "List and show findings",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completion.ValidFindings,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				return err
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts.Interactive = viper.GetBool("interactive")
			// Command should not be interactive when stdin is not a terminal.
			// TODO: Should this be global? Set on a Viper level for all commands?
			if opts.Interactive {
				opts.Interactive = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
			}
			opts.Server = viper.GetString("server")

			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}
			cmd := findingCmd{Command: c, opts: opts}
			return cmd.run(args)
		},
	}

	// Note: If a flag should be configurable via viper as well (i.e.
	//       via cifuzz.yaml and CIFUZZ_* environment variables), bind
	//       it to viper in the PreRun function.
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddProjectFlag,
	)

	return cmd
}

func (cmd *findingCmd) run(args []string) error {
	errorDetails, token, err := auth.TryGetErrorDetailsAndToken(cmd.opts.Server)
	if err != nil {
		return err
	}

	var remoteAPIFindings api.Findings

	if token != "" {
		log.Info("Note that findings that have already been uploaded to CI Sense will only be shown if you are authenticated and 'project' is set.")

		apiClient := api.NewClient(cmd.opts.Server)

		// get remote findings if project is set and user is authenticated
		if cmd.opts.Project != "" {
			remoteAPIFindings, err = apiClient.DownloadRemoteFindings(cmd.opts.Project, token)
			if err != nil {
				return err
			}
		} else if cmd.opts.Interactive { // let the user select a project
			remoteProjects, err := apiClient.ListProjects(token)
			if err != nil {
				return err
			}
			cmd.opts.Project, err = dialog.ProjectPicker(remoteProjects, "Select a remote project:")
			if err != nil {
				return err
			}
			if cmd.opts.Project != "<<cancel>>" {
				remoteAPIFindings, err = apiClient.DownloadRemoteFindings(cmd.opts.Project, token)
				if err != nil {
					return err
				}

				err = dialog.AskToPersistProjectChoice(cmd.opts.Project)
				if err != nil {
					return err
				}
			}
		} else {
			log.Warnf(`You are authenticated but did not specify a remote project.
Skipping remote findings because running in non-interactive mode.`)
		}
	}

	localFindings, err := finding.LocalFindings(cmd.opts.ProjectDir, errorDetails)
	if err != nil {
		return err
	}

	// store remote findings in a slice of finding.Finding so that we can search
	// them individually later. These won't be stored on disk.
	var remoteFindings []*finding.Finding
	for i := range remoteAPIFindings.Findings {
		// we access the element via index to avoid copying the struct
		rf := remoteAPIFindings.Findings[i]

		timeStamp, err := time.Parse(time.RFC3339, rf.Timestamp)
		if err != nil {
			return errors.Wrapf(err, "Could not parse timestamp %s", rf.Timestamp)
		}
		displayName := api.ConvertProjectNameForUseWithAPIV1V2(cmd.opts.Project)
		remoteFindings = append(remoteFindings, &finding.Finding{
			Origin:             "CI Sense",
			Name:               strings.TrimPrefix(rf.Name, fmt.Sprintf("%s/findings/", displayName)),
			Type:               finding.ErrorType(rf.ErrorReport.Type),
			InputData:          rf.ErrorReport.InputData,
			Logs:               rf.ErrorReport.Logs,
			Details:            rf.ErrorReport.Details,
			HumanReadableInput: string(rf.ErrorReport.InputData),
			MoreDetails:        rf.ErrorReport.MoreDetails,
			Tag:                rf.ErrorReport.Tag,
			CreatedAt:          timeStamp,
			FuzzTest:           rf.FuzzTargetDisplayName,
			StackTrace: []*stacktrace.StackFrame{
				{
					Function:   rf.ErrorReport.DebuggingInfo.BreakPoints[0].Function,
					SourceFile: rf.ErrorReport.DebuggingInfo.BreakPoints[0].SourceFilePath,
					Line:       rf.ErrorReport.DebuggingInfo.BreakPoints[0].Location.Line,
					Column:     rf.ErrorReport.DebuggingInfo.BreakPoints[0].Location.Column,
				},
			},
		})
	}

	if len(args) == 0 {
		// If called without arguments, `cifuzz findings` lists short
		// descriptions of all findings
		allFindings := append(localFindings, remoteFindings...)

		if cmd.opts.PrintJSON {
			s, err := stringutil.ToJSONString(allFindings)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), s)
			return nil
		}

		if len(allFindings) == 0 {
			log.Print("This project doesn't have any findings yet")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 1, ' ', 0)

		data := [][]string{
			{"Origin", "Severity", "Name", "Description", "Fuzz Test", "Location"},
		}

		for _, f := range allFindings {
			score := "n/a"
			locationInfo := f.SourceLocation()
			// check if MoreDetails exists to avoid nil pointer errors
			if f.MoreDetails != nil {
				// check if we have a severity and if we have a severity score
				if f.MoreDetails.Severity != nil {
					colorFunc := getColorFunctionForSeverity(f.MoreDetails.Severity.Score)
					score = colorFunc(fmt.Sprintf("%.1f", f.MoreDetails.Severity.Score))
				}
			}
			data = append(data, []string{
				f.Origin,
				score,
				f.Name,
				// FIXME: replace f.ShortDescriptionColumns()[0] with
				// f.MoreDetails.Name once we cover all bugs with our
				// error-details.json
				f.ShortDescriptionColumns()[0],
				f.FuzzTest,
				locationInfo,
			})
		}
		err = pterm.DefaultTable.WithHasHeader().WithData(data).Render()
		if err != nil {
			return errors.WithStack(err)
		}

		err = w.Flush()
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	// If called with one argument, `cifuzz finding <finding name>`
	// prints the information available for the specified finding
	findingName := args[0]

	// check if the finding is a remote finding...
	for i := range remoteFindings {
		f := remoteFindings[i]
		if strings.TrimPrefix(f.Name, fmt.Sprintf("projects/%s/findings/", cmd.opts.Project)) == findingName {
			return cmd.printFinding(f)
		}
	}

	// ...if the finding is not a remote finding, check if it is a local finding
	f, err := finding.LoadFinding(cmd.opts.ProjectDir, findingName, errorDetails)
	if finding.IsNotExistError(err) {
		return errors.WithMessagef(err, "Finding %s does not exist", findingName)
	}
	if err != nil {
		return err
	}
	return cmd.printFinding(f)
}

func (cmd *findingCmd) printFinding(f *finding.Finding) error {
	if cmd.opts.PrintJSON {
		s, err := stringutil.ToJSONString(f)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), s)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		s := pterm.Style{pterm.Reset, pterm.Bold}.Sprint(f.ShortDescriptionWithName())
		s += fmt.Sprintf("\nDate: %s\n", f.CreatedAt)
		s += fmt.Sprintf("\n  %s\n", strings.Join(f.Logs, "\n  "))
		_, err := fmt.Fprint(cmd.OutOrStdout(), s)
		if err != nil {
			return errors.WithStack(err)
		}
		PrintMoreDetails(f)
	}
	return nil
}

func PrintMoreDetails(f *finding.Finding) {
	if f.MoreDetails == nil {
		return
	}
	// the finding might have non-nil MoreDetails, but no information
	if f.MoreDetails.Name == "" || f.MoreDetails.Severity == nil {
		return
	}

	log.Info("\ncifuzz found more extensive information about this finding:")
	log.Debugf("Error ID: %s", f.MoreDetails.ID)
	data := [][]string{
		{"Name", f.MoreDetails.Name},
	}

	if f.MoreDetails.Severity != nil {
		data = append(data, []string{"Severity Level", string(f.MoreDetails.Severity.Level)})
		data = append(data, []string{"Severity Score", fmt.Sprintf("%.1f", f.MoreDetails.Severity.Score)})

	}
	if f.MoreDetails.Links != nil {
		for _, link := range f.MoreDetails.Links {
			data = append(data, []string{link.Description, link.URL})
		}
	}
	if f.MoreDetails.OwaspDetails != nil {
		if f.MoreDetails.OwaspDetails.Description != "" {
			data = append(data, []string{"OWASP Name", f.MoreDetails.OwaspDetails.Name})
			data = append(data, []string{"OWASP Description", wrapLongStringToMultiline(f.MoreDetails.OwaspDetails.Description, 80)})
		}
	}
	if f.MoreDetails.CweDetails != nil {
		if f.MoreDetails.CweDetails.Description != "" {
			data = append(data, []string{"CWE Name", f.MoreDetails.CweDetails.Name})
			data = append(data, []string{"CWE Description", wrapLongStringToMultiline(f.MoreDetails.CweDetails.Description, 80)})
		}
	}

	tableString, err := pterm.DefaultTable.WithData(data).WithBoxed().Srender()
	if err != nil {
		log.Error(err)
	}
	log.Print(tableString)

	if f.MoreDetails.Description != "" {
		log.Print(pterm.Blue("Description:"))
		log.Print(f.MoreDetails.Description)
	}
	if f.MoreDetails.Mitigation != "" {
		log.Print(pterm.Blue("\nMitigation:"))
		log.Print(f.MoreDetails.Mitigation)
	}
}

func getColorFunctionForSeverity(severity float32) func(a ...interface{}) string {
	switch {
	case severity >= 7.0:
		return pterm.Red
	case severity >= 4.0:
		return pterm.Yellow
	default:
		return pterm.Gray
	}
}

// wrapLongStringToMultiline wraps a long string to multiple lines.
// It tries to wrap at the last space before the maxLineLength to avoid
// breaking words.
func wrapLongStringToMultiline(s string, maxLineLength int) string {
	var result string
	var currentLine string
	var currentLineLength int

	for _, word := range strings.Split(s, " ") {
		if currentLineLength+len(word)+1 > maxLineLength {
			result += currentLine + "\n"
			currentLine = ""
			currentLineLength = 0
		}
		currentLine += word + " "
		currentLineLength += len(word) + 1
	}
	result += currentLine
	return result
}
