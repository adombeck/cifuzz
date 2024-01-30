package config

import (
	_ "embed"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/hectane/go-acl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/fileutil"
)

var baseTempDir string

func TestMain(m *testing.M) {
	var err error
	baseTempDir, err = os.MkdirTemp("", "project-config-test-")
	if err != nil {
		log.Fatalf("Failed to create temp dir for tests: %+v", err)
	}
	defer fileutil.Cleanup(baseTempDir)

	m.Run()
}

func TestCreateProjectConfig(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	path, err := CreateProjectConfig(projectDir, "", "")
	assert.NoError(t, err)
	expectedPath := filepath.Join(projectDir, ProjectConfigFile)
	assert.Equal(t, expectedPath, path)

	// file created?
	exists, err := fileutil.Exists(expectedPath)
	assert.NoError(t, err)
	assert.True(t, exists)

	// check for content
	content, err := os.ReadFile(expectedPath)
	assert.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.Contains(t, string(content), "Configuration for")
	assert.Contains(t, string(content), "#server: https://app.code-intelligence.com")
	assert.Contains(t, string(content), "#project: ")
}

// Should create a config file with a commented out server if the server value
// is empty
func TestCreateProjectConfig_WithServerAndProject(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	path, err := CreateProjectConfig(projectDir, "https://foo.bar", "my-project")
	assert.NoError(t, err)
	expectedPath := filepath.Join(projectDir, ProjectConfigFile)
	assert.Equal(t, expectedPath, path)

	// file created?
	exists, err := fileutil.Exists(expectedPath)
	assert.NoError(t, err)
	assert.True(t, exists)

	// check for content
	content, err := os.ReadFile(expectedPath)
	assert.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.Contains(t, string(content), "Configuration for")
	assert.Contains(t, string(content), "server: https://foo.bar")
	assert.Contains(t, string(content), "project: my-project")
}

// Should return error if not allowed to write to directory
func TestCreateProjectConfig_NoPerm(t *testing.T) {
	// create read only project dir
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = acl.Chmod(projectDir, 0o555)
	require.NoError(t, err)

	path, err := CreateProjectConfig(projectDir, "", "")
	assert.Error(t, err)
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.Empty(t, path)

	// file should not exists
	exists, err := fileutil.Exists(ProjectConfigFile)
	assert.NoError(t, err)
	assert.False(t, exists)
}

// Should return error if file already exists
func TestCreateProjectConfig_Exists(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	existingPath := filepath.Join(projectDir, ProjectConfigFile)
	err = os.WriteFile(existingPath, []byte{}, 0o644)
	require.NoError(t, err)

	path, err := CreateProjectConfig(filepath.Dir(existingPath), "", "")
	assert.Error(t, err)
	// check if path of the existing config is return and the error indicates it too
	assert.ErrorIs(t, err, os.ErrExist)
	assert.Equal(t, existingPath, path)

	// file should not exists
	exists, err := fileutil.Exists(existingPath)
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestParseProjectConfig(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	opts := &struct {
		BuildSystem string `mapstructure:"build-system"`
	}{}

	configFile := filepath.Join(projectDir, ProjectConfigFile)
	err = os.WriteFile(configFile, []byte("build-system: "), 0o644)
	require.NoError(t, err)

	err = ParseProjectConfig(projectDir, opts)
	require.NoError(t, err)
	require.Equal(t, BuildSystemOther, opts.BuildSystem)

	// Set the build system to cmake
	err = os.WriteFile(configFile, []byte("build-system: cmake"), 0o644)
	require.NoError(t, err)

	// Check that ParseProjectConfig now sets the build system to cmake
	err = ParseProjectConfig(projectDir, opts)
	require.NoError(t, err)
	require.Equal(t, BuildSystemCMake, opts.BuildSystem)
}

func TestParseProjectConfigCMake(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	opts := &struct {
		BuildSystem string `mapstructure:"build-system"`
	}{}

	configFile := filepath.Join(projectDir, ProjectConfigFile)
	err = os.WriteFile(configFile, []byte("build-system: "), 0o644)
	require.NoError(t, err)

	// Create a CMakeLists.txt in the project dir, which should cause
	// the build system to be detected as CMake
	err = os.WriteFile(filepath.Join(projectDir, "CMakeLists.txt"), []byte{}, 0o644)
	require.NoError(t, err)

	err = ParseProjectConfig(projectDir, opts)
	require.NoError(t, err)

	require.Equal(t, BuildSystemCMake, opts.BuildSystem)
}

func TestDetermineBuildSystem_CMake(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = os.WriteFile(filepath.Join(projectDir, "CMakeLists.txt"), []byte{}, 0o644)
	require.NoError(t, err, "Failed to create CMakeLists.txt")
	buildSystem, err := DetermineBuildSystem(projectDir)
	require.NoError(t, err)
	assert.Equal(t, BuildSystemCMake, buildSystem)
}

func TestDetermineBuildSystem_Maven(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = os.WriteFile(filepath.Join(projectDir, "pom.xml"), []byte{}, 0o644)
	require.NoError(t, err, "Failed to create pom.xml")
	buildSystem, err := DetermineBuildSystem(projectDir)
	require.NoError(t, err)
	assert.Equal(t, BuildSystemMaven, buildSystem)
}

func TestDetermineBuildSystem_GradleGroovy(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = os.WriteFile(filepath.Join(projectDir, "build.gradle"), []byte{}, 0o644)
	require.NoError(t, err, "Failed to create build.gradle")
	buildSystem, err := DetermineBuildSystem(projectDir)
	require.NoError(t, err)
	assert.Equal(t, BuildSystemGradle, buildSystem)
}

func TestDetermineBuildSystem_GradleKotlin(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = os.WriteFile(filepath.Join(projectDir, "build.gradle.kts"), []byte{}, 0o644)
	require.NoError(t, err, "Failed to create build.gradle.kts")
	buildSystem, err := DetermineBuildSystem(projectDir)
	require.NoError(t, err)
	assert.Equal(t, BuildSystemGradle, buildSystem)
}

func TestDetermineBuildSystem_Other(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	buildSystem, err := DetermineBuildSystem(projectDir)
	require.NoError(t, err)
	assert.Equal(t, BuildSystemOther, buildSystem)
}

func TestTestTypeFileNameExtension(t *testing.T) {
	ext, found := TestTypeFileNameExtension(Java)
	assert.True(t, found)
	assert.Equal(t, ".java", ext)

	ext, found = TestTypeFileNameExtension(Kotlin)
	assert.True(t, found)
	assert.Equal(t, ".kt", ext)
}

func TestIsGradleMultiProject_Groovy(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = fileutil.Touch(filepath.Join(projectDir, "settings.gradle"))
	require.NoError(t, err, "Failed to create settings.gradle")
	isGradleMultiProject, err := IsGradleMultiProject(projectDir)
	require.NoError(t, err)
	assert.True(t, isGradleMultiProject)
}

func TestIsGradleMultiProject_Kotlin(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = fileutil.Touch(filepath.Join(projectDir, "settings.gradle.kts"))
	require.NoError(t, err, "Failed to create settings.gradle.kts")
	isGradleMultiProject, err := IsGradleMultiProject(projectDir)
	require.NoError(t, err)
	assert.True(t, isGradleMultiProject)
}

func TestIsGradleMultiProject_False(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	err = fileutil.Touch(filepath.Join(projectDir, "build.gradle.kts"))
	require.NoError(t, err, "Failed to create build.gradle.kts")
	isGradleMultiProject, err := IsGradleMultiProject(projectDir)
	require.NoError(t, err)
	assert.False(t, isGradleMultiProject)
}

func TestEnsureProjectEntry(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
		input    string
	}{
		{
			name:     "empty",
			expected: "\nproject: myProject\n",
			input:    "",
		},
		{
			name: "existing project",
			expected: `## Set URL of CI Sense
#server: https://app.code-intelligence.com

## Set the project name on CI Sense
project: myProject`,
			input: `## Set URL of CI Sense
#server: https://app.code-intelligence.com

## Set the project name on CI Sense
project: my-project-1a2b3c4d`,
		},
		{
			name: "commented out project",
			expected: `## Set URL of CI Sense
#server: https://app.code-intelligence.com

## Set the project name on CI Sense
project: myProject`,
			input: `## Set URL of CI Sense
#server: https://app.code-intelligence.com

## Set the project name on CI Sense
#project: my-project-1a2b3c4d`,
		},
		{
			name: "non empty with no project",
			expected: `## Set URL of CI Sense
#server: https://app.code-intelligence.com
project: myProject
`,
			input: `## Set URL of CI Sense
#server: https://app.code-intelligence.com`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			updatedConfigString := EnsureProjectEntry(tc.input, "myProject")
			assert.Equal(t, tc.expected, updatedConfigString)
		})
	}
}
