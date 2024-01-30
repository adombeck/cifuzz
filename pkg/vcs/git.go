package vcs

import (
	"os/exec"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
)

// GitCommit returns the full SHA of the current commit if the working directory is contained in a Git repository.
func GitCommit() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	commit, err := cmd.Output()
	if err != nil {
		return "", errors.WithStack(err)
	}
	log.Debugf("Current Git commit: %s", string(commit))
	return strings.TrimSpace(string(commit)), nil
}

// GitBranch returns the name of the current branch if the working directory is contained in a Git repository.
func GitBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branch, err := cmd.Output()
	if err != nil {
		return "", errors.WithStack(err)
	}
	log.Debugf("Current Git branch: %s", string(branch))
	return strings.TrimSpace(string(branch)), nil
}

// GitIsDirty returns true if and only if the current working directory is contained in a Git repository that has
// uncommitted changes and/or untracked files.
func GitIsDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	commit, err := cmd.CombinedOutput()
	if err != nil {
		log.Debugf("failed to run git status --porcelain: %+v", err)
	}
	return len(strings.TrimSpace(string(commit))) != 0
}

// CodeRevision tries to read the current revision from git. If this is not possible the functions returns
// nil instead of an error.
func CodeRevision() *archive.CodeRevision {
	revision := &archive.CodeRevision{
		Git: &archive.GitRevision{},
	}

	commit, err := GitCommit()
	if err != nil {
		// if this returns an error (e.g. if users don't have git installed), we
		// don't want to fail the process (for example bundle creation or finding upload), so we just log that we
		// couldn't get the git commit and branch and continue without it.
		log.Debugf("failed to get Git commit. continuing without Git commit and branch. error: %+v",
			cmdutils.WrapSilentError(err))
		return nil
	} else {
		revision.Git.Commit = commit
	}

	branch, err := GitBranch()
	if err != nil {
		log.Debugf("failed to get Git branch. continuing without Git commit and branch. error: %+v",
			cmdutils.WrapSilentError(err))
		return nil
	} else {
		revision.Git.Branch = branch
	}

	if GitIsDirty() {
		log.Warnf("The Git repository has uncommitted changes. (Archive) Metadata may be inaccurate.")
	}

	return revision
}
