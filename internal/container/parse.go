package container

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/pkg/log"
)

// parseImageBuildOutput parses the output of a docker image build, updates the
// progress spinner with the current step and returns the image ID of the built
// image.
func parseImageBuildOutput(r io.Reader) (string, error) {
	var id string
	decoder := json.NewDecoder(r)
	for {
		var jsonMessage jsonmessage.JSONMessage
		err := decoder.Decode(&jsonMessage)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", errors.WithStack(err)
		}

		// we can stop if an error occurred
		if jsonMessage.Error != nil {
			return "", errors.WithStack(jsonMessage.Error)
		}

		if viper.GetBool("verbose") {
			err = jsonMessage.Display(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
			if err != nil {
				return "", errors.WithStack(err)
			}
		} else {
			// If jsonMessage.Stream matches a regex for "Step X/Y" then extract the
			// current step X and all steps Y
			stepRegex := regexp.MustCompile(`^Step (?P<currentStep>\d+)/(?P<totalSteps>\d+)`)
			if stepRegex.MatchString(jsonMessage.Stream) {
				matches := stepRegex.FindStringSubmatch(jsonMessage.Stream)
				stepString := fmt.Sprintf("%s (Step %s/%s)", log.ContainerBuildInProgressMsg, matches[1], matches[2])
				log.UpdateCurrentSpinnerPrinter(stepString)
			}
		}
		// extract the image ID from the Aux field
		if jsonMessage.Aux != nil {
			var res types.BuildResult
			if err := json.Unmarshal(*jsonMessage.Aux, &res); err != nil {
				log.Errorf(err, "Failed to unmarshal Aux JSON message: %s\n%v", *jsonMessage.Aux, err)
			} else if strings.HasPrefix(res.ID, "sha256:") {
				id = res.ID[7:]
			}
		}
	}

	return id, nil
}
