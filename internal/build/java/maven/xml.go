package maven

import (
	"bytes"
	"encoding/xml"
	"io"

	"github.com/pkg/errors"
)

type Resource struct {
	Directory string `xml:"directory"`
}

// this struct is a abbreviated representation of an actual pom.xml
type Project struct {
	XMLName     xml.Name `xml:"project"`
	GroupId     string   `xml:"groupId"`
	Version     string   `xml:"version"`
	Name        string   `xml:"name"`
	Description string   `xml:"description"`
	Properties  struct {
		Text                string `xml:",chardata"`
		MavenCompilerTarget string `xml:"maven.compiler.target"`
		MavenCompilerSource string `xml:"maven.compiler.source"`
	} `xml:"properties"`
	Dependencies struct {
		Dependency []struct {
			GroupId    string `xml:"groupId"`
			ArtifactId string `xml:"artifactId"`
			Version    string `xml:"version"`
			Scope      string `xml:"scope"`
		} `xml:"dependency"`
	} `xml:"dependencies"`
	Build struct {
		SourceDirectory       string `xml:"sourceDirectory"`
		ScriptSourceDirectory string `xml:"scriptSourceDirectory"`
		TestSourceDirectory   string `xml:"testSourceDirectory"`
		OutputDirectory       string `xml:"outputDirectory"`
		TestOutputDirectory   string `xml:"testOutputDirectory"`
		Resources             struct {
			Resource []Resource `xml:"resource"`
		} `xml:"resources"`
		TestResources struct {
			TestResource []Resource `xml:"testResource"`
		} `xml:"testResources"`
		Directory string `xml:"directory"`
	} `xml:"build"`
	Reporting struct {
		OutputDirectory string `xml:"outputDirectory"`
	} `xml:"reporting"`
}

func parseXML(in io.Reader) (*Project, error) {
	output, err := io.ReadAll(in)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to read pom.xml")
	}

	// Only parse output contained in <project></project> tags.
	// It can happend that the output contains some leading
	// and trailing bytes, which prevents successful parsing.
	// This issue occurs on Ubuntu 23.04 with Maven 3.8.7.
	startTag := []byte("<project")
	endTag := []byte("</project>")
	startIdx := bytes.Index(output, startTag)
	endIdx := bytes.Index(output, endTag) + len(endTag)

	var project Project
	err = xml.Unmarshal(output[startIdx:endIdx], &project)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to unmarshal pom.xml")
	}

	return &project, nil
}
