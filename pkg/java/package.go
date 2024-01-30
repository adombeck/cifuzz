package java

import (
	"bufio"
	"io"
	"strings"
)

// GetPackageFromSource returns the package name of a jvm class file
func GetPackageFromSource(sourceReader io.Reader) string {
	scanner := bufio.NewScanner(sourceReader)
	var line string
	inBlockComment := false
	for scanner.Scan() {
		line = scanner.Text()
		line = strings.TrimSpace(line)

		if len(line) == 0 {
			continue
		}

		if strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(line, "/*") {
			inBlockComment = true
			continue
		}

		if inBlockComment && strings.HasSuffix(line, "*/") {
			inBlockComment = false
			continue
		}

		if inBlockComment {
			continue
		}

		break
	}

	if !strings.HasPrefix(line, "package ") {
		return ""
	}

	line = strings.TrimPrefix(line, "package ")
	return strings.TrimSuffix(line, ";")
}
