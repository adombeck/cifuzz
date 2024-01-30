package mocks

import (
	"github.com/stretchr/testify/mock"

	"code-intelligence.com/cifuzz/pkg/runfiles"
)

type RunfilesFinderMock struct {
	mock.Mock
}

var _ runfiles.RunfilesFinder = (*RunfilesFinderMock)(nil)

func (m *RunfilesFinderMock) BazelPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) CIFuzzIncludePath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) ClangPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) CMakePath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) CMakePresetsPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) LLVMCovPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) LLVMProfDataPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) LLVMSymbolizerPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) GenHTMLPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) PerlPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) JavaPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) MavenPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) GradlePath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) Minijail0Path() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) ProcessWrapperPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) DumperPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) ReplayerSourcePath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) ListFuzzTestsJarPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) VisualStudioPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) VSCodeTasksPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) LogoPath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) JavaHomePath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *RunfilesFinderMock) NodePath() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}
