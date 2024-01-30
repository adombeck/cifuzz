package java

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/stringutil"
)

func CreateManifestJar(entries map[string]string, directory string) (string, error) {
	jarPath := filepath.Join(directory, "manifest.jar")
	jarFile, err := os.Create(jarPath)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer jarFile.Close()
	jarWriter := zip.NewWriter(jarFile)
	defer jarWriter.Close()

	// create explicit parent directory in zip file
	fh := &zip.FileHeader{
		Name: "META-INF/",
	}
	fh.SetMode(0o755)
	_, err = jarWriter.CreateHeader(fh)
	if err != nil {
		return "", errors.WithStack(err)
	}

	// add manifest file
	fh = &zip.FileHeader{
		// explicitly use forward slashes as requested from the
		// zip.FileHeader struct / zip spec
		Name: "META-INF/MANIFEST.MF",
	}
	fh.SetMode(0o644)
	manifestFile, err := jarWriter.CreateHeader(fh)
	if err != nil {
		return "", errors.WithStack(err)
	}

	err = writeManifest(entries, manifestFile)
	if err != nil {
		return "", err
	}

	err = jarWriter.Close()
	if err != nil {
		return "", errors.WithStack(err)
	}
	log.Debugf("Created manifest.jar at %s", jarPath)
	return jarPath, nil
}

func writeManifest(entries map[string]string, target io.Writer) error {
	content, err := entriesToString(entries)
	if err != nil {
		return err
	}
	_, err = target.Write([]byte(content))
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func entriesToString(entries map[string]string) (string, error) {
	var content strings.Builder
	for k, v := range entries {
		// headers are not allowed to be wrapped, so without the ": "
		// separator a header is not allowed to be longer than 70 bytes
		if len(k) > 70 {
			return "", errors.Errorf("invalid header, size > 70: %s", k)
		}

		rawEntry := fmt.Sprintf("%s: %s", k, v)
		// The max length for a row is 72 bytes, including the EOL bytes
		// and the " " separator, according to: https://stackoverflow.com/a/33144934
		// so we split after 70 to avoid " " and EOL breaking the line limit.
		// Theoretical the limit for the first (and maybe the only) chunk
		// is 71 as there can not be a " " separator. To keep it simple we
		// ignored this case as it should not have any implications
		entry := strings.Join(stringutil.SplitAfterNBytes(rawEntry, 70), "\n ")
		fmt.Fprintf(&content, "%s\n", entry)
	}

	// make sure there is always an empty line at the end of the file
	if content.Len() == 0 || content.String()[content.Len()-1] != '\n' {
		content.WriteString("\n")
	}
	return content.String(), nil
}
