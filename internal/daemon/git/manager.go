// Package git provides Git repository management for the daemon's state tracking.
//
// This package handles fork-aware Git operations where repositories have:
//   - 'origin' remote pointing to the user's fork
//   - 'upstream' remote pointing to the original repository
//
// It provides functionality to check repository state (commits behind upstream, local changes)
// and synchronize with upstream. All operations use exec.Command to run Git natively.
//
// This is distinct from internal/git which handles repository synchronization for
// keeping local repos up-to-date. This package focuses on state tracking for the daemon.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/meyrevived/mpc-dev-env/internal/daemon/state"
)

// GitManager provides Git operations for repository management with fork-aware logic.
// It is designed to work with forked repositories where:
// - 'origin' is the user's fork
// - 'upstream' is the original repository
// All operations are implemented natively in Go using exec.Command for Git.
type GitManager struct {
	// Empty struct - all methods operate on repositories via their paths
}

// NewGitManager creates a new GitManager instance.
//
// Returns:
//
//	A new GitManager instance
func NewGitManager() *GitManager {
	return &GitManager{}
}

// CheckRepoState checks the Git state of a repository using ONLY local data.
// This function performs NO network operations and reads only from the local Git repository.
// It compares the local branch against the locally cached 'upstream/main' ref.
//
// IMPORTANT: This method assumes Sync() has been called to fetch upstream changes.
// If Sync() has never been called, CommitsBehindUpstream may be inaccurate.
//
// Args:
//
//	repoPath: The absolute path to the Git repository
//
// Returns:
//
//	A RepositoryState struct containing the repository state
//	An error if the operation fails
//
// Example:
//
//	manager := NewGitManager()
//	repoState, err := manager.CheckRepoState("/home/user/multi-platform-controller")
//	if err != nil {
//	    log.Printf("Failed to check repo state: %v", err)
//	}
func (m *GitManager) CheckRepoState(repoPath string) (*state.RepositoryState, error) {
	// Verify this is a Git repository
	if err := m.verifyGitRepo(repoPath); err != nil {
		return nil, err
	}

	// Get the current branch
	currentBranch, err := m.getCurrentBranch(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Check if there are local changes
	hasLocalChanges, err := m.hasLocalChanges(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check for local changes: %w", err)
	}

	// Compare against upstream/main to get commits ahead/behind (using locally cached refs)
	commitsBehindUpstream, err := m.getCommitsBehindUpstream(repoPath, currentBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get upstream commits: %w", err)
	}

	// Extract repository name from path
	repoName := m.extractRepoName(repoPath)

	// Build the RepositoryState struct
	repoState := &state.RepositoryState{
		Name:                  repoName,
		Path:                  repoPath,
		CurrentBranch:         currentBranch,
		CommitsBehindUpstream: commitsBehindUpstream,
		HasLocalChanges:       hasLocalChanges,
	}

	return repoState, nil
}

// Sync performs network operations to synchronize the repository with upstream.
// This method:
// - Ensures the 'upstream' remote exists
// - Fetches the latest changes from the 'upstream' remote
//
// This method should be called periodically in the background to keep the local
// cache of upstream refs up to date. CheckRepoState relies on these cached refs.
//
// Args:
//
//	repoPath: The absolute path to the Git repository
//
// Returns:
//
//	An error if the operation fails
//
// Example:
//
//	manager := NewGitManager()
//	if err := manager.Sync("/home/user/multi-platform-controller"); err != nil {
//	    log.Printf("Failed to sync repository: %v", err)
//	}
func (m *GitManager) Sync(repoPath string) error {
	// Verify this is a Git repository
	if err := m.verifyGitRepo(repoPath); err != nil {
		return err
	}

	// Ensure upstream remote exists
	if err := m.ensureUpstreamRemote(repoPath); err != nil {
		return fmt.Errorf("failed to ensure upstream remote: %w", err)
	}

	// Fetch from upstream
	if err := m.fetchUpstream(repoPath); err != nil {
		return fmt.Errorf("failed to fetch from upstream: %w", err)
	}

	return nil
}

// verifyGitRepo checks if the given path is a valid Git repository.
// It uses "git rev-parse --git-dir" which succeeds only if the path contains a .git directory.
// Returns an error if the path is not a Git repository.
func (m *GitManager) verifyGitRepo(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not a git repository: %s", repoPath)
	}
	return nil
}

// getCurrentBranch returns the name of the current branch using "git rev-parse --abbrev-ref HEAD".
// Returns an error if in detached HEAD state or if the command fails.
func (m *GitManager) getCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	branch := strings.TrimSpace(stdout.String())
	if branch == "" {
		return "", errors.New("empty branch name")
	}

	return branch, nil
}

// ensureUpstreamRemote verifies that the 'upstream' remote is configured.
// It uses "git remote get-url upstream" to check if the remote exists.
// Returns an error if the upstream remote is not configured, with instructions
// on how to add it manually.
//
// Future enhancement: Could automatically add upstream remote based on repository name.
func (m *GitManager) ensureUpstreamRemote(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "upstream")
	if err := cmd.Run(); err != nil {
		return errors.New("upstream remote not configured (use: git remote add upstream <url>)")
	}
	return nil
}

// fetchUpstream fetches the latest changes from the upstream remote.
// It runs "git fetch upstream" to download new commits and update the locally
// cached upstream refs (e.g., upstream/main). This does not modify the working directory.
func (m *GitManager) fetchUpstream(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "fetch", "upstream")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch upstream: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// hasLocalChanges checks if there are uncommitted or unstaged changes in the working directory.
// It uses "git status --porcelain" which provides machine-readable output.
// Returns true if there are any modified, added, deleted, or untracked files.
func (m *GitManager) hasLocalChanges(repoPath string) (bool, error) {
	cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("failed to check status: %w", err)
	}

	// If there's any output, there are changes
	output := strings.TrimSpace(stdout.String())
	return output != "", nil
}

// getCommitsBehindUpstream returns the number of commits that the local branch is behind upstream/main.
// It uses "git rev-list --count HEAD..upstream/main" to count commits that exist on upstream/main
// but not in the current branch.
//
// Returns 0 if upstream/main doesn't exist (e.g., on first run before Sync() is called).
// This method relies on locally cached refs - Sync() must be called periodically to keep them updated.
func (m *GitManager) getCommitsBehindUpstream(repoPath, currentBranch string) (int, error) {
	// Use git rev-list to count commits in upstream/main that are not in the current branch
	// Format: git rev-list --count upstream/main..HEAD (commits ahead)
	// Format: git rev-list --count HEAD..upstream/main (commits behind - what we want)

	cmd := exec.Command("git", "-C", repoPath, "rev-list", "--count", currentBranch+"..upstream/main")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// If upstream/main doesn't exist, return 0 (no commits ahead)
		return 0, nil
	}

	countStr := strings.TrimSpace(stdout.String())
	var count int
	_, err := fmt.Sscanf(countStr, "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("failed to parse commit count: %w", err)
	}

	return count, nil
}

// extractRepoName extracts the repository name from the absolute path.
// It returns the last component of the path after splitting on "/".
//
// Examples:
//   - "/home/user/multi-platform-controller" -> "multi-platform-controller"
//   - "/var/repos/mpc" -> "mpc"
func (m *GitManager) extractRepoName(repoPath string) string {
	parts := strings.Split(strings.TrimRight(repoPath, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return repoPath
}
