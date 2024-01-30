package tokenstorage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

var accessTokens map[string]string

var accessTokensFilePath string
var readErr error
var filePathErr error

func init() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		// If we can't get the user config directory, we can't read the access
		// tokens file. We only log the error on debug level here because it's
		// not clear yet if the access tokens are needed. We return the error
		// in the Get/Set functions.
		filePathErr = errors.Wrap(err, "Error determining access tokens file path")
		readErr = filePathErr
		log.Debug(filePathErr.Error())
		return
	}
	accessTokensFilePath = filepath.Join(configDir, "cifuzz", "access_tokens.json")

	migrateOldTokens()

	bytes, err := os.ReadFile(accessTokensFilePath)
	if err != nil && os.IsNotExist(err) {
		// The access tokens file doesn't exist, so we initialize the
		// access tokens with an empty map
		accessTokens = map[string]string{}
		return
	}
	if err != nil {
		readErr = errors.WithStack(err)
		log.Errorf(err, "Error reading access tokens file: %v", err.Error())
		return
	}
	err = json.Unmarshal(bytes, &accessTokens)
	if err != nil {
		readErr = errors.WithStack(err)
		log.Errorf(err, "Error parsing access tokens file %s: %v", accessTokensFilePath, err.Error())
		return
	}
}

func Set(target, token string) error {
	if filePathErr != nil {
		return errors.WithMessage(filePathErr, "Can't set access token")
	}

	// Ensure that the parent directory exists
	err := os.MkdirAll(filepath.Dir(accessTokensFilePath), 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	accessTokens[target] = token

	// Convert the access tokens to JSON
	bytes, err := json.MarshalIndent(accessTokens, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	// Write the JSON to file
	err = os.WriteFile(accessTokensFilePath, bytes, 0o600)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get returns the access token for the given target
// If the given target doesn't exist, try to add or remove a trailing slash
// and return the access token for that target
func Get(target string) (string, error) {
	if readErr != nil {
		return "", errors.WithMessage(readErr, "Can't get access token")
	}

	if token, ok := accessTokens[target]; ok {
		return token, nil
	}
	if token, ok := accessTokens[strings.TrimSuffix(target, "/")]; ok {
		return token, nil
	}
	if token, ok := accessTokens[target+"/"]; ok {
		return token, nil
	}
	return "", nil
}

func GetTokenFilePath() (string, error) {
	return accessTokensFilePath, filePathErr
}

// migrateOldTokens migrates the old access tokens file to the new location
func migrateOldTokens() {
	oldTokensFilePath := os.ExpandEnv("$HOME/.config/cifuzz/access_tokens.json")
	if oldTokensFilePath == accessTokensFilePath {
		return
	}

	exists, err := fileutil.Exists(oldTokensFilePath)
	if err != nil {
		log.Errorf(err, "Error checking if old tokens file exists: %v", err.Error())
		return
	}
	if !exists {
		return
	}

	// make sure that new tokens file directory exists
	err = os.MkdirAll(filepath.Dir(accessTokensFilePath), 0o755)
	if err != nil {
		log.Errorf(err, "Error creating config directory: %v", err.Error())
		return
	}

	log.Infof("Migrating old tokens file to new location: %s", accessTokensFilePath)
	err = os.Rename(oldTokensFilePath, accessTokensFilePath)
	if err != nil {
		log.Errorf(err, "Error migrating old tokens file: %v", err.Error())
	}
}
