package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Git Syncer Suite")
}

// Helper function to set up a git repository
func setupGitRepo(path, initialCommitMsg string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	Expect(cmd.Run()).To(Succeed())

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = path
	Expect(cmd.Run()).To(Succeed())

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = path
	Expect(cmd.Run()).To(Succeed())

	cmd = exec.Command("git", "commit", "--allow-empty", "-m", initialCommitMsg)
	cmd.Dir = path
	Expect(cmd.Run()).To(Succeed())
}

// Helper function to set up a bare git repository
func setupBareGitRepo(path string) {
	Expect(os.MkdirAll(path, 0755)).To(Succeed())
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = path
	Expect(cmd.Run()).To(Succeed())
}

var _ = Describe("Syncer", func() {
	var (
		syncer   *Syncer
		tempDir  string
		repoPath string
		ctx      context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "syncer-test-*")
		Expect(err).NotTo(HaveOccurred())

		repoPath = filepath.Join(tempDir, "test-repo")
		Expect(os.MkdirAll(repoPath, 0755)).To(Succeed())
		setupGitRepo(repoPath, "Initial commit")

		// Create a mock config
		cfg := &config.Config{
			MpcRepoPath: repoPath,
		}
		syncer = NewSyncer(cfg)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("getCurrentBranch", func() {
		It("should return the current branch name", func() {
			branch, err := syncer.getCurrentBranch(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(MatchRegexp("main|master"))
		})

		It("should return the correct name for a feature branch", func() {
			cmd := exec.Command("git", "-C", repoPath, "checkout", "-b", "feature-branch")
			Expect(cmd.Run()).To(Succeed())

			branch, err := syncer.getCurrentBranch(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(branch).To(Equal("feature-branch"))
		})
	})

	Describe("hasLocalChanges", func() {
		It("should return false for a clean repository", func() {
			changes, err := syncer.hasLocalChanges(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(changes).To(BeFalse())
		})

		It("should return true when a file is modified", func() {
			filePath := filepath.Join(repoPath, "file.txt")
			Expect(os.WriteFile(filePath, []byte("initial"), 0644)).To(Succeed())
			cmd := exec.Command("git", "-C", repoPath, "add", "file.txt")
			Expect(cmd.Run()).To(Succeed())
			cmd = exec.Command("git", "-C", repoPath, "commit", "-m", "add file")
			Expect(cmd.Run()).To(Succeed())

			// Now modify it
			Expect(os.WriteFile(filePath, []byte("modified"), 0644)).To(Succeed())

			changes, err := syncer.hasLocalChanges(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(changes).To(BeTrue())
		})

		It("should return true when a new untracked file is added", func() {
			filePath := filepath.Join(repoPath, "untracked.txt")
			Expect(os.WriteFile(filePath, []byte("untracked"), 0644)).To(Succeed())

			changes, err := syncer.hasLocalChanges(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(changes).To(BeTrue())
		})

		It("should return true when a file is staged", func() {
			filePath := filepath.Join(repoPath, "staged.txt")
			Expect(os.WriteFile(filePath, []byte("staged"), 0644)).To(Succeed())
			cmd := exec.Command("git", "-C", repoPath, "add", "staged.txt")
			Expect(cmd.Run()).To(Succeed())

			changes, err := syncer.hasLocalChanges(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(changes).To(BeTrue())
		})
	})

	Describe("SyncRepo", func() {
		var (
			originPath string
			clonePath  string
		)

		BeforeEach(func() {
			// Setup a bare repo to act as origin
			originPath = filepath.Join(tempDir, "origin.git")
			setupBareGitRepo(originPath)

			// Add it as a remote to our test repo
			cmd := exec.Command("git", "-C", repoPath, "remote", "add", "origin", originPath)
			Expect(cmd.Run()).To(Succeed())

			// Push initial commit to origin
			cmd = exec.Command("git", "-C", repoPath, "push", "-u", "origin", "HEAD")
			Expect(cmd.Run()).To(Succeed())

			// Create a separate clone to push new commits to origin
			clonePath = filepath.Join(tempDir, "clone-repo")
			cmd = exec.Command("git", "clone", originPath, clonePath)
			Expect(cmd.Run()).To(Succeed())
			setupGitRepo(clonePath, "Second commit from clone")
			cmd = exec.Command("git", "-C", clonePath, "push", "origin", "HEAD")
			Expect(cmd.Run()).To(Succeed())
		})

		It("should fast-forward merge when there are no local commits", func() {
			err := syncer.SyncRepo(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the local repo is at the same commit as the origin
			cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
			localHash, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "-C", clonePath, "rev-parse", "HEAD")
			originHash, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(localHash).To(Equal(originHash))
		})

		It("should hard reset when the local branch has diverged", func() {
			// Make a local commit in the main repo to create divergence
			setupGitRepo(repoPath, "Divergent local commit")

			err := syncer.SyncRepo(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the local repo is reset to the origin's state
			cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
			localHash, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("git", "-C", clonePath, "rev-parse", "HEAD")
			originHash, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(localHash).To(Equal(originHash))
		})

		It("should hard reset when there are local uncommitted changes", func() {
			filePath := filepath.Join(repoPath, "local-change.txt")
			Expect(os.WriteFile(filePath, []byte("local change"), 0644)).To(Succeed())

			err := syncer.SyncRepo(ctx, repoPath)
			Expect(err).NotTo(HaveOccurred())

			// After a hard reset, the untracked file should be gone.
			// The original test was flawed in its assertion.
			// We check if the file still exists.
			_, err = os.Stat(filePath)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})
})
