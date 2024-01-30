package auth

import (
	"net/url"
	"os"

	"github.com/pkg/browser"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
)

// NoValidTokenError indicates that no valid API access token is configured.
type NoValidTokenError struct {
	err error
}

func (e NoValidTokenError) Error() string {
	return e.err.Error()
}

func (e NoValidTokenError) Unwrap() error {
	return e.err
}

// GetToken returns the API access token for the given server.
func GetToken(server string) (string, error) {
	// Try the environment variable
	token := os.Getenv("CIFUZZ_API_TOKEN")
	if token != "" {
		log.Print("Using token from $CIFUZZ_API_TOKEN")
		return token, nil
	}

	// Try the access tokens config file
	return tokenstorage.Get(server)
}

func GetValidToken(server string) (string, error) {
	token, err := GetToken(server)
	if err != nil {
		return "", err
	}

	isValid, err := IsValidToken(server, token)
	if err != nil {
		return "", err
	}
	if !isValid {
		return "", &NoValidTokenError{errors.New("Please log in with a valid API access token")}
	}
	return token, nil
}

// HasValidToken returns true if a valid API access token is configured
// for the given server.
func HasValidToken(server string) (bool, error) {
	token, err := GetToken(server)
	if err != nil {
		return false, err
	}

	return IsValidToken(server, token)
}

func IsValidToken(server, token string) (bool, error) {
	if token == "" {
		return false, nil
	}

	apiClient := api.NewClient(server)
	return apiClient.IsTokenValid(token)
}

func readToken(server string) (string, error) {
	path, err := url.JoinPath(server, "dashboard", "settings", "account", "tokens")
	if err != nil {
		return "", errors.WithStack(err)
	}

	values := url.Values{}
	values.Set("create", "")
	values.Add("origin", "cli")

	url, err := url.Parse(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	url.RawQuery = values.Encode()

	log.Printf("You need an API access token which can be generated here:\n%s", url.String())

	openBrowser, err := dialog.Confirm("Open browser to generate a new token?", true)
	if err != nil {
		return "", err
	}

	if openBrowser {
		err = browser.OpenURL(url.String())
		if err != nil {
			err = errors.WithStack(err)
			log.Errorf(err, "Failed to open browser: %v", err)
		}
	}

	return dialog.ReadSecret("Paste your access token")
}

func StoreToken(server, token string) error {
	err := tokenstorage.Set(server, token)
	if err != nil {
		return err
	}
	tokenFilePath, err := tokenstorage.GetTokenFilePath()
	if err != nil {
		return err
	}
	log.Successf("Successfully authenticated with %s", server)
	log.Infof("Your API access token is stored in %s", tokenFilePath)
	return nil
}

func EnsureValidToken(server string) (string, error) {
	token, err := GetToken(server)
	if err != nil {
		return "", err
	}

	apiClient := api.NewClient(server)

	if token != "" {
		isValid, err := apiClient.IsTokenValid(token)
		if err != nil {
			return "", err
		}
		if isValid {
			log.Success("You are authenticated.")
			return token, nil
		}

		log.Warn(`Failed to authenticate with the configured API access token.
It's possible that the token has been revoked.`)
	}

	if !viper.GetBool("interactive") || !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", &NoValidTokenError{errors.New("Please log in with a valid API access token")}
	}

	var isValid bool

	for !isValid {
		var loginMsg string
		if token != "" {
			loginMsg = "Do you want to log in with another token?"
		} else {
			loginMsg = "Do you want to log in?"
		}
		tryAgain, err := dialog.Confirm(loginMsg, true)
		if err != nil {
			return "", err
		}
		if !tryAgain {
			return "", &NoValidTokenError{errors.New("Please log in with a valid API access token")}
		}

		token, err = readToken(server)
		if err != nil {
			return "", err
		}

		isValid, err = apiClient.IsTokenValid(token)
		if err != nil {
			return "", err
		}

		if !isValid {
			log.Warn(`Failed to authenticate with the entered API access token.`)
		}
	}

	err = StoreToken(server, token)
	if err != nil {
		return "", err
	}

	return token, nil
}
