// Package git provides Git repository synchronization functionality.
//
// It handles keeping the local multi-platform-controller repository synchronized
// with its upstream source. The package performs automatic fetching and merging,
// with fallback to hard reset when local changes prevent fast-forward merges.
//
// This functionality replaces the Python-based UpstreamChangeDetector and provides
// automatic repository updates without user intervention.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	"github.com/meyrevived/mpc-dev-env/internal/logger"
)

// Syncer provides Git synchronization functionality for repositories.
// It synchronizes repositories with their upstream sources to ensure
// the local codebase is always up-to-date.
type Syncer struct {
	config *config.Config
}

// NewSyncer creates a new Git Syncer instance.
//
// Args:
//
//	cfg: The configuration struct containing repository paths
//
// Returns:
//
//	A new Syncer instance
func NewSyncer(cfg *config.Config) *Syncer {
	return &Syncer{
		config: cfg,
	}
}

// SyncRepo synchronizes a single Git repository with its upstream.
// This function:
//   - Determines the current branch
//   - Fetches from origin
//   - Performs a fast-forward merge to update the local branch
//   - If fast-forward fails, falls back to git reset --hard origin/<branch>
//
// Args:
//
//	ctx: Context for cancellation and timeout
//	repoPath: Absolute path to the Git repository
//
// Returns:
//
//	An error if synchronization fails
func (s *Syncer) SyncRepo(ctx context.Context, repoPath string) error {
	logger.Info("starting synchronization", "path", repoPath)

	// Step 1: Get current branch
	currentBranch, err := s.getCurrentBranch(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	logger.Info("current branch", "branch", currentBranch)

	// Step 2: Fetch from origin
	logger.Info("fetching from origin")
	if err := s.fetchOrigin(ctx, repoPath); err != nil {
		return fmt.Errorf("failed to fetch from origin: %w", err)
	}
	logger.Info("fetch completed successfully")

	// Step 3: Check for local changes
	hasLocalChanges, err := s.hasLocalChanges(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("failed to check for local changes: %w", err)
	}

	if hasLocalChanges {
		logger.Info("repository has local changes, using hard reset strategy")
		// Use git reset --hard to forcefully sync with upstream
		if err := s.resetHard(ctx, repoPath, currentBranch); err != nil {
			return fmt.Errorf("failed to reset repository: %w", err)
		}
		logger.Info("repository reset to origin", "branch", currentBranch)
	} else {
		// Step 4: Try fast-forward merge
		logger.Info("attempting fast-forward merge")
		if err := s.fastForwardMerge(ctx, repoPath, currentBranch); err != nil {
			// If fast-forward fails, use reset --hard as fallback
			logger.Info("fast-forward merge failed, falling back to hard reset")
			if err := s.resetHard(ctx, repoPath, currentBranch); err != nil {
				return fmt.Errorf("failed to reset repository after merge failure: %w", err)
			}
			logger.Info("repository reset to origin", "branch", currentBranch)
		} else {
			logger.Info("fast-forward merge completed successfully")
		}
	}

	logger.Info("synchronization completed successfully", "path", repoPath)
	return nil
}

// SyncAllRepos synchronizes all configured repositories.
// Currently, this only includes multi-platform-controller.
//
// Args:
//
//	ctx: Context for cancellation and timeout
//
// Returns:
//
//	An error if any synchronization fails
func (s *Syncer) SyncAllRepos(ctx context.Context) error {
	logger.Info("starting synchronization for all repositories")

	repos := []struct {
		name string
		path string
	}{
		{"multi-platform-controller", s.config.GetMpcRepoPath()},
	}

	var syncErrors []string
	for _, repo := range repos {
		logger.Info("syncing repository", "name", repo.name)
		if err := s.SyncRepo(ctx, repo.path); err != nil {
			errMsg := fmt.Sprintf("%s: %v", repo.name, err)
			syncErrors = append(syncErrors, errMsg)
			logger.Error(err, "failed to sync repository", "name", repo.name)
		} else {
			logger.Info("successfully synced repository", "name", repo.name)
		}
	}

	if len(syncErrors) > 0 {
		return fmt.Errorf("failed to sync repositories: %s", strings.Join(syncErrors, "; "))
	}

	logger.Info("all repositories synchronized successfully")
	return nil
}

// getCurrentBranch returns the current branch name for the repository.
// It uses "git rev-parse --abbrev-ref HEAD" to determine the active branch.
// Returns an error if the repository is in detached HEAD state or if the command fails.
func (s *Syncer) getCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w, stderr: %s", err, stderr.String())
	}

	branch := strings.TrimSpace(stdout.String())
	if branch == "" {
		return "", errors.New("empty branch name")
	}

	return branch, nil
}

// fetchOrigin fetches latest changes from the origin remote.
// It runs "git fetch origin" to download new commits and refs without merging.
// Output (both stdout and stderr) is logged for visibility.
func (s *Syncer) fetchOrigin(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "origin")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git fetch failed: %w, stderr: %s", err, stderr.String())
	}

	// Log the fetch output for visibility
	if out := stdout.String(); out != "" {
		logger.Debug("fetch stdout", "output", out)
	}
	if errOut := stderr.String(); errOut != "" {
		logger.Debug("fetch stderr", "output", errOut)
	}

	return nil
}

// hasLocalChanges checks if the repository has uncommitted or unstaged changes.
// It uses "git status --porcelain" which produces machine-readable output.
// Returns true if any changes are detected (modified, added, deleted, or untracked files).
func (s *Syncer) hasLocalChanges(ctx context.Context, repoPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git status failed: %w", err)
	}

	// If there's any output, there are changes
	return strings.TrimSpace(stdout.String()) != "", nil
}

// fastForwardMerge attempts a fast-forward only merge with origin/<branch>.
// It uses "git merge --ff-only" which succeeds only if the local branch can be
// fast-forwarded (i.e., no divergent commits). This preserves local commit history
// when possible. If the merge cannot be done with fast-forward, it returns an error.
func (s *Syncer) fastForwardMerge(ctx context.Context, repoPath, branch string) error {
	remoteBranch := "origin/" + branch
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "merge", "--ff-only", remoteBranch)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git merge --ff-only failed: %w, stderr: %s", err, stderr.String())
	}

	// Log the merge output
	if out := stdout.String(); out != "" {
		logger.Debug("merge stdout", "output", out)
	}

	return nil
}

// resetHard performs a hard reset to origin/<branch>, discarding all local changes.
// This is a destructive operation that:
//   - Resets the HEAD to match origin/<branch>
//   - Discards all uncommitted changes (staged and unstaged)
//   - Resets the index to match the remote branch
//   - Removes all untracked files and directories
//
// This is used as a fallback when fast-forward merge fails or when local changes exist.
func (s *Syncer) resetHard(ctx context.Context, repoPath, branch string) error {
	remoteBranch := "origin/" + branch
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "reset", "--hard", remoteBranch)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git reset --hard failed: %w, stderr: %s", err, stderr.String())
	}

	// Log the reset output
	if out := stdout.String(); out != "" {
		logger.Debug("reset stdout", "output", out)
	}

	// Remove untracked files and directories
	// -f: force removal of untracked files
	// -d: remove untracked directories
	cleanCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "clean", "-fd")
	var cleanStdout bytes.Buffer
	var cleanStderr bytes.Buffer
	cleanCmd.Stdout = &cleanStdout
	cleanCmd.Stderr = &cleanStderr

	if err := cleanCmd.Run(); err != nil {
		return fmt.Errorf("git clean failed: %w, stderr: %s", err, cleanStderr.String())
	}

	// Log the clean output if any files were removed
	if out := cleanStdout.String(); out != "" {
		logger.Debug("clean stdout", "output", out)
	}

	return nil
}
