package container

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	cliconfig "github.com/docker/cli/cli/config"
	containerRegistry "github.com/docker/docker/api/types/registry"
	"github.com/pkg/errors"
)

func RegistryAuth(registry string) (string, error) {
	// TODO check if this works on Windows
	homedir, err := os.UserHomeDir()
	if err != nil {
		return "", errors.WithStack(err)
	}
	cfg, err := cliconfig.Load(filepath.Join(homedir, ".docker"))
	if err != nil {
		return "", errors.WithStack(err)
	}

	// Strip the repository name from the registry
	registry = strings.Split(registry, "/")[0]
	a, err := cfg.GetAuthConfig(registry)
	if err != nil {
		return "", errors.WithStack(err)
	}

	ac := containerRegistry.AuthConfig(a)
	encodedConfig, err := json.Marshal(ac)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return base64.URLEncoding.EncodeToString(encodedConfig), nil
}
