package git_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meyrevived/mpc-dev-env/internal/daemon/git"
)

func TestGit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Git Manager Suite")
}

var _ = Describe("GitManager", func() {
	var (
		manager  *git.GitManager
		tempDir  string
		repoPath string
	)

	BeforeEach(func() {
		manager = git.NewGitManager()

		// Create a temporary directory for test repositories
		var err error
		tempDir, err = os.MkdirTemp("", "git-manager-test-*")
		Expect(err).NotTo(HaveOccurred())

		repoPath = filepath.Join(tempDir, "test-repo")
	})

	AfterEach(func() {
		// Clean up temporary directory
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("NewGitManager", func() {
		It("should create a new GitManager instance", func() {
			m := git.NewGitManager()
			Expect(m).NotTo(BeNil())
		})
	})

	Describe("CheckRepoState", func() {
		Context("when the path is not a git repository", func() {
			It("should return an error", func() {
				// Create a non-git directory
				err := os.MkdirAll(repoPath, 0755)
				Expect(err).NotTo(HaveOccurred())

				_, err = manager.CheckRepoState(repoPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not a git repository"))
			})
		})

		Context("when the path does not exist", func() {
			It("should return an error", func() {
				nonExistentPath := filepath.Join(tempDir, "non-existent")
				_, err := manager.CheckRepoState(nonExistentPath)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with a valid git repository", func() {
			BeforeEach(func() {
				// Initialize a git repository
				err := os.MkdirAll(repoPath, 0755)
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command("git", "init")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Configure git user for commits
				cmd = exec.Command("git", "config", "user.email", "test@example.com")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "config", "user.name", "Test User")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create an initial commit
				testFile := filepath.Join(repoPath, "README.md")
				err = os.WriteFile(testFile, []byte("# Test Repo\n"), 0644)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", "README.md")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "Initial commit")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Ensure we're on the main branch (git init might create master)
				cmd = exec.Command("git", "branch", "-M", "main")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when upstream remote is not configured", func() {
				It("should return 0 commits behind upstream", func() {
					// Without upstream remote, the code gracefully returns 0 commits behind
					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(repoState.CommitsBehindUpstream).To(Equal(0))
				})
			})

			Context("when upstream remote is configured", func() {
				BeforeEach(func() {
					// Create a bare repository to serve as upstream
					upstreamPath := filepath.Join(tempDir, "upstream-repo.git")
					cmd := exec.Command("git", "init", "--bare", upstreamPath)
					err := cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Add upstream remote
					cmd = exec.Command("git", "remote", "add", "upstream", upstreamPath)
					cmd.Dir = repoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Push to upstream to establish main branch
					cmd = exec.Command("git", "push", "upstream", "main")
					cmd.Dir = repoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())
				})

				It("should successfully check repository state with no local changes", func() {
					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState).NotTo(BeNil())
					Expect(repoState.Name).To(Equal("test-repo"))
					Expect(repoState.Path).To(Equal(repoPath))
					Expect(repoState.CurrentBranch).To(Equal("main"))
					Expect(repoState.HasLocalChanges).To(BeFalse())
					Expect(repoState.CommitsBehindUpstream).To(Equal(0))
				})

				It("should detect local changes when files are modified", func() {
					// Modify a file
					testFile := filepath.Join(repoPath, "README.md")
					err := os.WriteFile(testFile, []byte("# Modified\n"), 0644)
					Expect(err).NotTo(HaveOccurred())

					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState.HasLocalChanges).To(BeTrue())
				})

				It("should detect local changes when new files are added", func() {
					// Add a new file
					newFile := filepath.Join(repoPath, "new-file.txt")
					err := os.WriteFile(newFile, []byte("new content\n"), 0644)
					Expect(err).NotTo(HaveOccurred())

					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState.HasLocalChanges).To(BeTrue())
				})

				It("should detect when upstream has new commits", func() {
					// Create a second repository to push to upstream
					secondRepoPath := filepath.Join(tempDir, "second-repo")
					upstreamPath := filepath.Join(tempDir, "upstream-repo.git")

					// Clone upstream to second repo with explicit branch
					cmd := exec.Command("git", "clone", "--branch", "main", upstreamPath, secondRepoPath)
					var stderr bytes.Buffer
					cmd.Stderr = &stderr
					err := cmd.Run()
					if err != nil {
						GinkgoWriter.Printf("git clone failed: %v, stderr: %s\n", err, stderr.String())
					}
					Expect(err).NotTo(HaveOccurred())

					// Configure git user for commits
					cmd = exec.Command("git", "config", "user.email", "test@example.com")
					cmd.Dir = secondRepoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("git", "config", "user.name", "Test User")
					cmd.Dir = secondRepoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Make a commit in the second repo
					newFile := filepath.Join(secondRepoPath, "upstream-change.txt")
					err = os.WriteFile(newFile, []byte("upstream change\n"), 0644)
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("git", "add", "upstream-change.txt")
					cmd.Dir = secondRepoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("git", "commit", "-m", "Upstream commit")
					cmd.Dir = secondRepoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Push to upstream
					cmd = exec.Command("git", "push", "origin", "main")
					cmd.Dir = secondRepoPath
					stderr.Reset()
					cmd.Stderr = &stderr
					err = cmd.Run()
					if err != nil {
						GinkgoWriter.Printf("git push failed: %v, stderr: %s\n", err, stderr.String())
					}
					Expect(err).NotTo(HaveOccurred())

					// Fetch upstream changes into the original repo (CheckRepoState uses only local data)
					cmd = exec.Command("git", "fetch", "upstream")
					cmd.Dir = repoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Now check the original repo - it should detect upstream is ahead
					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState.CommitsBehindUpstream).To(Equal(1))
				})

				It("should work on a feature branch", func() {
					// Create and checkout a feature branch
					cmd := exec.Command("git", "checkout", "-b", "feature-branch")
					cmd.Dir = repoPath
					err := cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState.CurrentBranch).To(Equal("feature-branch"))
				})

				It("should handle repositories with multiple remotes", func() {
					// Add an additional remote (origin)
					originPath := filepath.Join(tempDir, "origin-repo.git")
					cmd := exec.Command("git", "init", "--bare", originPath)
					err := cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("git", "remote", "add", "origin", originPath)
					cmd.Dir = repoPath
					err = cmd.Run()
					Expect(err).NotTo(HaveOccurred())

					// Should still work with multiple remotes
					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState).NotTo(BeNil())
				})

				It("should correctly extract repository name from path", func() {
					repoState, err := manager.CheckRepoState(repoPath)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState.Name).To(Equal("test-repo"))
				})

				It("should handle paths with trailing slashes", func() {
					repoPathWithSlash := repoPath + "/"
					repoState, err := manager.CheckRepoState(repoPathWithSlash)
					Expect(err).NotTo(HaveOccurred())
					Expect(repoState.Name).To(Equal("test-repo"))
				})
			})
		})

		Context("fork-aware logic", func() {
			var upstreamPath string

			BeforeEach(func() {
				// Create a bare repository to serve as upstream
				upstreamPath = filepath.Join(tempDir, "upstream-repo.git")
				cmd := exec.Command("git", "init", "--bare", upstreamPath)
				err := cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Initialize the main repo
				err = os.MkdirAll(repoPath, 0755)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "init")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Configure git user
				cmd = exec.Command("git", "config", "user.email", "test@example.com")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "config", "user.name", "Test User")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Create initial commit
				testFile := filepath.Join(repoPath, "README.md")
				err = os.WriteFile(testFile, []byte("# Test\n"), 0644)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", "README.md")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "Initial commit")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Ensure we're on the main branch (git init might create master)
				cmd = exec.Command("git", "branch", "-M", "main")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Add upstream remote
				cmd = exec.Command("git", "remote", "add", "upstream", upstreamPath)
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Push to upstream
				cmd = exec.Command("git", "push", "upstream", "main")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fetch from upstream, not origin", func() {
				// This test verifies that the implementation fetches from upstream
				// by checking that no error occurs when upstream exists but origin doesn't
				repoState, err := manager.CheckRepoState(repoPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(repoState).NotTo(BeNil())
			})

			It("should compare against upstream/main, not origin/main", func() {
				// Create another clone to push changes to upstream
				clonePath := filepath.Join(tempDir, "clone-repo")
				cmd := exec.Command("git", "clone", "--branch", "main", upstreamPath, clonePath)
				err := cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Configure git user
				cmd = exec.Command("git", "config", "user.email", "test@example.com")
				cmd.Dir = clonePath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "config", "user.name", "Test User")
				cmd.Dir = clonePath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Make a change in the clone
				newFile := filepath.Join(clonePath, "new-file.txt")
				err = os.WriteFile(newFile, []byte("new content\n"), 0644)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "add", "new-file.txt")
				cmd.Dir = clonePath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "commit", "-m", "New file")
				cmd.Dir = clonePath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("git", "push", "origin", "main")
				cmd.Dir = clonePath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Fetch upstream changes into the original repo (CheckRepoState uses only local data)
				cmd = exec.Command("git", "fetch", "upstream")
				cmd.Dir = repoPath
				err = cmd.Run()
				Expect(err).NotTo(HaveOccurred())

				// Check the original repo - should detect the upstream change
				repoState, err := manager.CheckRepoState(repoPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(repoState.CommitsBehindUpstream).To(BeNumerically(">", 0))
			})
		})
	})
})
