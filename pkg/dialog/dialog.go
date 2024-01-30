package dialog

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"atomicgo.dev/keyboard/keys"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"golang.org/x/exp/maps"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/stringutil"
)

// Select offers the user a list of items (label:value) to select from and returns the value of the selected item
func Select(message string, items map[string]string, sorted bool) (string, error) {
	options := maps.Keys(items)
	if sorted {
		sort.Strings(options)
	}
	prompt := pterm.DefaultInteractiveSelect.WithOptions(options)
	prompt.DefaultText = message

	result, err := prompt.Show()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return items[result], nil
}

// MultiSelect offers the user a list of items (label:value) to select from and returns the values of the selected items
func MultiSelect(message string, items map[string]string) ([]string, error) {
	options := maps.Keys(items)
	sort.Strings(options)

	prompt := pterm.DefaultInteractiveMultiselect.WithOptions(options)
	prompt.DefaultText = message
	prompt.Filter = false
	prompt.KeyConfirm = keys.Enter
	prompt.KeySelect = keys.Space

	selectedOptions, err := prompt.Show()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sort.Strings(selectedOptions)

	var result []string
	for _, option := range selectedOptions {
		result = append(result, items[option])
	}

	return result, nil
}

func Confirm(message string, defaultValue bool) (bool, error) {
	var confirmText, rejectText string
	if defaultValue {
		confirmText = "Y"
		rejectText = "n"
	} else {
		confirmText = "y"
		rejectText = "N"
	}
	res, err := pterm.InteractiveConfirmPrinter{
		DefaultValue: defaultValue,
		DefaultText:  message,
		TextStyle:    &pterm.ThemeDefault.PrimaryStyle,
		ConfirmText:  confirmText,
		ConfirmStyle: &pterm.ThemeDefault.PrimaryStyle,
		RejectText:   rejectText,
		RejectStyle:  &pterm.ThemeDefault.PrimaryStyle,
		SuffixStyle:  &pterm.ThemeDefault.SecondaryStyle,
		OnInterruptFunc: func() {
			// Print an empty line to avoid the cursor being on the same line
			// as the confirmation prompt
			log.Print()
			// Exit with code 130 (128 + 2) to match the behavior of the
			// default interrupt signal handler
			os.Exit(130)
		},
	}.Show()
	return res, errors.WithStack(err)
}

func Input(message string) (string, error) {
	input := pterm.DefaultInteractiveTextInput.WithDefaultText(message)
	result, err := input.Show()
	if err != nil {
		return "", errors.WithStack(err)
	}
	return result, nil
}

// ReadSecret reads a secret from the user and prints * characters instead of
// the actual secret.
func ReadSecret(message string) (string, error) {
	secret, err := pterm.DefaultInteractiveTextInput.WithMask("*").Show(message)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return secret, nil
}

// askToPersistProjectChoice asks the user if they want to persist their
// project choice. If they do, it adds the project to the cifuzz.yaml file.
func AskToPersistProjectChoice(projectName string) error {
	// trim the projects/ prefix when saving the project name to cifuzz.yaml
	projectName = strings.TrimPrefix(projectName, "projects/")

	persist, err := Confirm(`Do you want to persist your choice?
This will add a 'project' entry to your cifuzz.yaml.
You can change these values later by editing the file.`, true)
	if err != nil {
		return err
	}

	if persist {
		contents, err := os.ReadFile(config.ProjectConfigFile)
		if err != nil {
			return errors.WithStack(err)
		}
		updatedContents := config.EnsureProjectEntry(string(contents), projectName)

		err = os.WriteFile(config.ProjectConfigFile, []byte(updatedContents), 0o644)
		if err != nil {
			return errors.WithStack(err)
		}
		log.Notef("Your choice has been persisted in cifuzz.yaml.")
	}
	return nil
}

// ProjectPicker lets the user select a project from a list of projects (usually fetched from the API).
// It also offers the option to create a new server project.
func ProjectPickerWithOptionNew(projects []*api.Project, prompt string, client *api.APIClient, token string) (string, error) {
	// Let the user select a project
	var displayNames []string
	var names []string
	var err error
	for _, p := range projects {
		displayNames = append(displayNames, p.DisplayName)
		names = append(names, p.Name)
	}
	maxLen := stringutil.MaxLen(displayNames)
	items := map[string]string{}
	for i := range displayNames {
		key := fmt.Sprintf("%-*s [%s]", maxLen, displayNames[i], names[i])
		items[key] = names[i]
	}

	// add option to create a new project
	items["<Create a new project>"] = "<<new>>"

	// add option to cancel
	items["<Cancel>"] = "<<cancel>>"

	projectName, err := Select(prompt, items, true)
	if err != nil {
		return "", err
	}

	switch projectName {
	case "<<new>>":
		// ask user for project name
		projectName, err = Input("Enter the name of the project you want to create")
		if err != nil {
			return "", errors.WithStack(err)
		}

		project, err := client.CreateProject(projectName, token)
		if err != nil {
			return "", err
		}
		projectName = project.Name

	case "<<cancel>>":
		return "<<cancel>>", nil
	}

	return api.ConvertProjectNameFromAPI(projectName)
}

// ProjectPicker lets the user select a project from a list of projects (usually fetched from the API).
func ProjectPicker(projects []*api.Project, prompt string) (string, error) {
	// Let the user select a project
	var displayNames []string
	var names []string
	var err error
	for _, p := range projects {
		displayNames = append(displayNames, p.DisplayName)
		names = append(names, p.Name)
	}
	maxLen := stringutil.MaxLen(displayNames)
	items := map[string]string{}
	for i := range displayNames {
		key := fmt.Sprintf("%-*s [%s]", maxLen, displayNames[i], names[i])
		items[key] = names[i]
	}

	// add option to cancel
	items["<Cancel>"] = "<<cancel>>"

	projectName, err := Select(prompt, items, true)
	if err != nil {
		return "", err
	}

	if projectName == "<<cancel>>" {
		return "<<cancel>>", nil
	}

	return api.ConvertProjectNameFromAPI(projectName)
}
