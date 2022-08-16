package integrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	copy2 "github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type integrateCmd struct {
	*cobra.Command

	tools []string
}

func supportedTools() []string {
	return []string{"git", "clion", "vscode"}
}

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "integrate <git|clion|vscode>",
		Short: "Add integrations for the following tools: Git, CLion, VS Code",
		Long: `Add integrations for Git, CLion and VS Code:

Add files generated by cifuzz to .gitignore:

    cifuzz integrate git

Provide integration with CLion by adding CMake presets to your
CMakeUserPresets.json file:

    cifuzz integrate clion

Provide integration with VS Code by adding CMake presets to your
CMakeUserPresets.json file and by adding tasks to your tasks.json:

    cifuzz integrate vscode

Missing files are generated automatically.
`,
		ValidArgs: supportedTools(),
		Args:      cobra.MatchAll(cobra.MaximumNArgs(3), cobra.OnlyValidArgs),
		RunE: func(c *cobra.Command, args []string) error {
			cmd := integrateCmd{
				Command: c,
				tools:   args,
			}

			return cmd.run()
		},
	}

	return cmd
}

func (c *integrateCmd) run() error {
	var err error

	if len(c.tools) == 0 {
		c.tools, err = selectTools()
	}

	projectDir, err := config.FindProjectDir()
	if errors.Is(err, os.ErrNotExist) {
		// The project directory doesn't exist, this is an expected
		// error, so we print it and return a silent error to avoid
		// printing a stack trace
		log.Error(err, fmt.Sprintf("%s\nUse 'cifuzz init' to set up a project for use with cifuzz.", err.Error()))
		return cmdutils.ErrSilent
	}
	if err != nil {
		return err
	}

	for _, tool := range c.tools {
		switch tool {
		case "git":
			err = setupGitIgnore(projectDir)
			if err != nil {
				return err
			}
		case "clion":
			err = setupCMakePresets(projectDir, runfiles.Finder)
			if err != nil {
				return err
			}
		case "vscode":
			err = setupCMakePresets(projectDir, runfiles.Finder)
			if err != nil {
				return err
			}
			err = setupVSCodeTasks(projectDir, runfiles.Finder)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// selectTools lets the user select the desired tools via an interactive multiselect dialog
func selectTools() ([]string, error) {
	result, err := pterm.DefaultInteractiveMultiselect.WithOptions(supportedTools()).Show()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return result, nil
}

func setupGitIgnore(projectDir string) error {
	filesToIgnore := []string{
		"/.cifuzz-build/",
		"/.cifuzz-corpus/",
		"/.cifuzz-findings/",
		"/CMakeUserPresets.json",
	}

	gitIgnorePath := filepath.Join(projectDir, ".gitignore")
	hasGitIgnore, err := fileutil.Exists(gitIgnorePath)
	if err != nil {
		return err
	}

	if !hasGitIgnore {
		err = os.WriteFile(gitIgnorePath, []byte(strings.Join(filesToIgnore, "\n")), 0644)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		bytes, err := os.ReadFile(gitIgnorePath)
		if err != nil {
			return errors.WithStack(err)
		}
		existingFilesToIgnore := strings.Split(string(bytes), "\n")

		gitIgnore, err := os.OpenFile(gitIgnorePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return errors.WithStack(err)
		}
		defer gitIgnore.Close()

		for _, fileToIgnore := range filesToIgnore {
			if !stringutil.Contains(existingFilesToIgnore, fileToIgnore) {
				_, err = gitIgnore.WriteString(fileToIgnore + "\n")
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	}
	log.Printf(`
Added files generated by cifuzz to .gitignore.`)

	return nil
}

func setupVSCodeTasks(projectDir string, finder runfiles.RunfilesFinder) error {
	tasksSrcPath, err := finder.VSCodeTasksPath()
	if err != nil {
		return err
	}
	tasksDestPath := filepath.Join(projectDir, ".vscode", "tasks.json")
	hasTasks, err := fileutil.Exists(tasksDestPath)
	if err != nil {
		return err
	}

	if !hasTasks {
		// Situation: The user doesn't have a tasks.json file set up and
		// may thus be unaware of this functionality. Create one and tell
		// them about it.
		err = copy2.Copy(tasksSrcPath, tasksDestPath)
		if err != nil {
			return errors.WithStack(err)
		}
		log.Printf(`
tasks.json has been created in .vscode to provide easy access to command
line workflows. It enables you to launch coverage runs from within
VS Code. You can use the Coverage Gutters extension to visualize the
generated coverage report. To learn more about tasks in VS Code, visit:

	https://code.visualstudio.com/docs/editor/tasks

You can download the Coverage Gutters extension from:

	https://marketplace.visualstudio.com/items?itemName=ryanluker.vscode-coverage-gutters`)
	} else {
		// Situation: The user does have a tasks.json file set up, so we
		// assume them to know about the benefits. We suggest to the user
		// that they add our task to the existing tasks.json.
		presetsSrc, err := os.ReadFile(tasksSrcPath)
		if err != nil {
			return errors.WithStack(err)
		}

		log.Printf(`
Add the following task to your tasks.json to provide easy access to
cifuzz coverage runs from within VS Code. You can use the Coverage
Gutters extension to visualize the generated coverage report.
%s

You can download the Coverage Gutters extension from:

	https://marketplace.visualstudio.com/items?itemName=ryanluker.vscode-coverage-gutters
`, presetsSrc)
	}

	return nil
}

func setupCMakePresets(projectDir string, finder runfiles.RunfilesFinder) error {
	presetsSrcPath, err := finder.CMakePresetsPath()
	if err != nil {
		return err
	}
	presetsDestPath := filepath.Join(projectDir, "CMakeUserPresets.json")
	hasPresets, err := fileutil.Exists(presetsDestPath)
	if err != nil {
		return err
	}

	if !hasPresets {
		// Situation: The user doesn't have a CMake user preset set up and
		// may thus be unaware of this functionality. Create one and tell
		// them about it.
		err = copy2.Copy(presetsSrcPath, presetsDestPath)
		if err != nil {
			return errors.WithStack(err)
		}
		log.Printf(`
CMakeUserPresets.json has been created to provide integration with IDEs
such as CLion and Visual Studio Code. It enables you to run regression
tests and measure code coverage right from your IDE.
This file should not be checked in to version control systems.
To learn more about CMake presets, visit:

    https://github.com/microsoft/vscode-cmake-tools/blob/main/docs/cmake-presets.md
    https://www.jetbrains.com/help/clion/cmake-presets.html`)
	} else {
		// Situation: The user does have a CMake user preset set up, so we
		// assume them to know about the benefits. We suggest to the user
		// that they add our preset to the existing CMakeUserPresets.json.
		presetsSrc, err := os.ReadFile(presetsSrcPath)
		if err != nil {
			return errors.WithStack(err)
		}

		log.Printf(`
Add the following presets to your CMakeUserPresets.json to be able to
run regression tests and measure code coverage right from your IDE:

%s`, presetsSrc)
	}

	return nil
}
