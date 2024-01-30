package cmdutils

import (
	"net/url"

	"github.com/pkg/errors"
)

func BuildURLFromParts(server string, parts ...string) (string, error) {
	path, err := url.JoinPath(server, parts...)
	if err != nil {
		return "", errors.WithStack(err)
	}

	values := url.Values{}
	values.Add("origin", "cli")

	validURL, err := url.Parse(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	validURL.RawQuery = values.Encode()

	return validURL.String(), nil

}
