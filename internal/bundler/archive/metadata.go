package archive

import (
	"os"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// MetadataFileName is the name of the meta information yaml file within an artifact archive.
const MetadataFileName = "bundle.yaml"

// Metadata defines meta information for artifacts contained within a fuzzing artifact archive.
type Metadata struct {
	*RunEnvironment `yaml:"run_environment"`
	CodeRevision    *CodeRevision `yaml:"code_revision,omitempty"`
	Fuzzers         []*Fuzzer     `yaml:"fuzzers"`
}

// Fuzzer specifies the type and locations of fuzzers contained in the archive.
type Fuzzer struct {
	Target    string `yaml:"target,omitempty"`
	Name      string `yaml:"name,omitempty"`
	Path      string `yaml:"path"`
	Engine    string `yaml:"engine"`
	Sanitizer string `yaml:"sanitizer,omitempty"`
	// The different YAML field name is *not* a typo: For historical reasons, the "build_dir" field is supposed to
	// include the root directory of the *source* rather than the build tree of the project. Rather than expose all
	// cifuzz devs to this inconsistency, we keep it in the serialization logic.
	ProjectDir    string        `yaml:"build_dir"`
	Dictionary    string        `yaml:"dictionary,omitempty"`
	Seeds         string        `yaml:"seeds,omitempty"`
	LibraryPaths  []string      `yaml:"library_paths,omitempty"`
	RuntimePaths  []string      `yaml:"runtime_paths,omitempty"`
	EngineOptions EngineOptions `yaml:"engine_options,omitempty"`
	MaxRunTime    uint          `yaml:"max_run_time,omitempty"`
}

// RunEnvironment specifies the environment in which the fuzzers are to be run.
type RunEnvironment struct {
	// The docker image and tag to be used: eg. debian:stable
	Docker string
}

type CodeRevision struct {
	Git *GitRevision `yaml:"git,omitempty" json:"git_revision,omitempty"`
}

type GitRevision struct {
	Commit string `yaml:"commit,omitempty" json:"commit,omitempty"`
	Branch string `yaml:"branch,omitempty" json:"branch,omitempty"`
}

type EngineOptions struct {
	Flags []string `yaml:"flags,omitempty"`
	Env   []string `yaml:"env,omitempty"`
}

func (a *Metadata) ToYaml() ([]byte, error) {
	out, err := yaml.Marshal(a)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal metadata to YAML: %+v", *a)
	}

	return out, nil
}

func (a *Metadata) FromYaml(data []byte) error {
	err := yaml.Unmarshal(data, a)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal metadata from YAML:\n%s", string(data))
	}

	return nil
}

func MetadataFromPath(path string) (*Metadata, error) {
	metadataFile, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	metadata := &Metadata{}
	err = metadata.FromYaml(metadataFile)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}
