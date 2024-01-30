package maven

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseXML_BuildDirs(t *testing.T) {

	in := strings.NewReader(`
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <build>
    <directory>/target</directory>
    <outputDirectory>/target/classes</outputDirectory>
    <testOutputDirectory>/target/test-classes</testOutputDirectory>
    <resources>
      <resource>
        <directory>/main/resources1</directory>
      </resource>
      <resource>
        <directory>/main/resources2</directory>
      </resource>
    </resources>
    <testResources>
      <testResource>
        <directory>/test/resources</directory>
      </testResource>
    </testResources>
    </build>
</project>
  `)
	project, err := parseXML(in)
	require.NoError(t, err)
	require.NotEmpty(t, project)

	assert.Equal(t, "/target", project.Build.Directory)
	assert.Equal(t, "/target/classes", project.Build.OutputDirectory)
	assert.Equal(t, "/target/test-classes", project.Build.TestOutputDirectory)

	assert.Len(t, project.Build.Resources.Resource, 2)
	assert.Equal(t, "/main/resources1", project.Build.Resources.Resource[0].Directory)
	assert.Equal(t, "/main/resources2", project.Build.Resources.Resource[1].Directory)

	assert.Len(t, project.Build.TestResources.TestResource, 1)
	assert.Equal(t, "/test/resources", project.Build.TestResources.TestResource[0].Directory)
}

func TestParseXML_BuildDirs_WithPollutedInput(t *testing.T) {
	in := strings.NewReader(`xxxxx
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <build>
    <directory>/target</directory>
  </build>
</project>
xxxxx`)
	project, err := parseXML(in)
	require.NoError(t, err)
	require.NotEmpty(t, project)

	assert.Equal(t, "/target", project.Build.Directory)
}

// TestParseXML_TestDir checks that testSourceDirectory is extracted correctly out of the xml.
func TestParseXML_TestDir(t *testing.T) {
	in := strings.NewReader(`
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <build>
    <testSourceDirectory>src/test</testSourceDirectory>
  </build>
</project>
`)
	project, err := parseXML(in)
	require.NoError(t, err)
	require.NotEmpty(t, project)

	assert.Equal(t, "src/test", project.Build.TestSourceDirectory)
}
