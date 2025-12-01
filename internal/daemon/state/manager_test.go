package state_test

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meyrevived/mpc-dev-env/internal/daemon/state"
)

// MockGitManager is a mock implementation of the GitManager interface for testing
type MockGitManager struct {
	CheckRepoStateFunc func(repoPath string) (*state.RepositoryState, error)
}

func (m *MockGitManager) CheckRepoState(repoPath string) (*state.RepositoryState, error) {
	if m.CheckRepoStateFunc != nil {
		return m.CheckRepoStateFunc(repoPath)
	}
	return &state.RepositoryState{
		Name:                  "test-repo",
		Path:                  repoPath,
		CurrentBranch:         "main",
		CommitsBehindUpstream: 0,
		HasLocalChanges:       false,
	}, nil
}

// MockClusterManager is a mock implementation of the cluster.Manager for testing
type MockClusterManager struct {
	StatusFunc func(ctx context.Context) (string, error)
}

func (m *MockClusterManager) Status(ctx context.Context) (string, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx)
	}
	return "running", nil
}

func (m *MockClusterManager) Create(ctx context.Context) error {
	return nil
}

func (m *MockClusterManager) Destroy(ctx context.Context) error {
	return nil
}

var _ = Describe("StateManager", func() {
	var (
		mockGitManager     *MockGitManager
		mockClusterManager *MockClusterManager
		config             *state.StateManagerConfig
	)

	BeforeEach(func() {
		mockGitManager = &MockGitManager{}
		mockClusterManager = &MockClusterManager{}

		config = &state.StateManagerConfig{
			GitManager:     mockGitManager,
			ClusterManager: mockClusterManager,
			RepoPaths: map[string]string{
				"multi-platform-controller": "/home/user/mpc",
			},
			KubeconfigPath: "/home/user/.kube/config",
		}
	})

	Describe("NewStateManager", func() {
		It("should create a new StateManager successfully", func() {
			manager, err := state.NewStateManager(config)

			Expect(err).ToNot(HaveOccurred())
			Expect(manager).ToNot(BeNil())
		})

		It("should return an error if GitManager is nil", func() {
			config.GitManager = nil

			manager, err := state.NewStateManager(config)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("GitManager is required"))
			Expect(manager).To(BeNil())
		})

		It("should return an error if ClusterManager is nil", func() {
			config.ClusterManager = nil

			manager, err := state.NewStateManager(config)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ClusterManager is required"))
			Expect(manager).To(BeNil())
		})

		It("should perform initial state scan on creation", func() {
			gitCheckCalled := false
			mockGitManager.CheckRepoStateFunc = func(repoPath string) (*state.RepositoryState, error) {
				gitCheckCalled = true
				return &state.RepositoryState{
					Name:                  "multi-platform-controller",
					Path:                  repoPath,
					CurrentBranch:         "main",
					CommitsBehindUpstream: 0,
					HasLocalChanges:       false,
				}, nil
			}

			clusterStatusCalled := false
			mockClusterManager.StatusFunc = func(ctx context.Context) (string, error) {
				clusterStatusCalled = true
				return "running", nil
			}

			manager, err := state.NewStateManager(config)

			Expect(err).ToNot(HaveOccurred())
			Expect(manager).ToNot(BeNil())
			Expect(gitCheckCalled).To(BeTrue())
			Expect(clusterStatusCalled).To(BeTrue())
		})

		It("should initialize state with session ID and timestamps", func() {
			manager, err := state.NewStateManager(config)

			Expect(err).ToNot(HaveOccurred())

			currentState := manager.GetState()
			Expect(currentState.SessionID).ToNot(BeEmpty())
			Expect(currentState.CreatedAt).ToNot(BeZero())
			Expect(currentState.LastActive).ToNot(BeZero())
		})

		It("should handle repository check failures gracefully during initialization", func() {
			mockGitManager.CheckRepoStateFunc = func(repoPath string) (*state.RepositoryState, error) {
				return nil, errors.New("repository not found")
			}

			manager, err := state.NewStateManager(config)

			// Should not fail, but repositories map should be empty
			Expect(err).ToNot(HaveOccurred())
			Expect(manager).ToNot(BeNil())

			currentState := manager.GetState()
			Expect(currentState.Repositories).To(BeEmpty())
		})

		It("should handle cluster check failures gracefully during initialization", func() {
			mockClusterManager.StatusFunc = func(ctx context.Context) (string, error) {
				return "", errors.New("cluster check failed")
			}

			manager, err := state.NewStateManager(config)

			// Should not fail, but cluster status should be unknown (error treated as cluster state unknown)
			Expect(err).ToNot(HaveOccurred())
			Expect(manager).ToNot(BeNil())

			currentState := manager.GetState()
			Expect(currentState.Cluster.Status).To(Equal("unknown"))
		})
	})

	Describe("GetState", func() {
		It("should return the current state", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			currentState := manager.GetState()

			Expect(currentState.SessionID).ToNot(BeEmpty())
			Expect(currentState.Repositories).ToNot(BeNil())
		})

		It("should return a copy of the state (not a reference)", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			state1 := manager.GetState()
			state2 := manager.GetState()

			// Modifying state1 should not affect state2
			state1.SessionID = "modified"
			Expect(state2.SessionID).ToNot(Equal("modified"))
		})

		It("should be thread-safe (concurrent reads)", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			var wg sync.WaitGroup
			numReaders := 10

			for i := 0; i < numReaders; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = manager.GetState()
				}()
			}

			wg.Wait()
			// If we reach here without panicking, the test passes
		})
	})

	Describe("RefreshState", func() {
		It("should update the state by querying dependencies", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			// Get initial state
			initialState := manager.GetState()
			initialLastActive := initialState.LastActive

			// Wait a bit to ensure timestamp changes
			time.Sleep(10 * time.Millisecond)

			// Refresh state
			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			// Get updated state
			updatedState := manager.GetState()

			// LastActive should be updated
			Expect(updatedState.LastActive.After(initialLastActive)).To(BeTrue())
		})

		It("should call GitManager.CheckRepoState for each repository", func() {
			checkCallCount := 0
			mockGitManager.CheckRepoStateFunc = func(repoPath string) (*state.RepositoryState, error) {
				checkCallCount++
				return &state.RepositoryState{
					Name:                  "multi-platform-controller",
					Path:                  repoPath,
					CurrentBranch:         "main",
					CommitsBehindUpstream: 0,
					HasLocalChanges:       false,
				}, nil
			}

			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			// Reset counter after initialization
			checkCallCount = 0

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			// Should have called CheckRepoState once for the single repository
			Expect(checkCallCount).To(Equal(1))
		})

		It("should call ClusterManager.Status to check cluster state", func() {
			clusterCheckCalled := false
			mockClusterManager.StatusFunc = func(ctx context.Context) (string, error) {
				clusterCheckCalled = true
				return "running", nil
			}

			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			// Reset flag after initialization
			clusterCheckCalled = false

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			Expect(clusterCheckCalled).To(BeTrue())
		})

		It("should check MPC deployment status", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			// MPC deployment check is currently a no-op (returns nil)
			// This test verifies it doesn't cause errors
			currentState := manager.GetState()
			Expect(currentState.MPCDeployment).To(BeNil())
		})

		It("should update repository state when changes are detected", func() {
			// Initial state: no local changes
			mockGitManager.CheckRepoStateFunc = func(repoPath string) (*state.RepositoryState, error) {
				return &state.RepositoryState{
					Name:                  "multi-platform-controller",
					Path:                  repoPath,
					CurrentBranch:         "main",
					CommitsBehindUpstream: 0,
					HasLocalChanges:       false,
				}, nil
			}

			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			initialState := manager.GetState()
			Expect(initialState.Repositories["multi-platform-controller"].HasLocalChanges).To(BeFalse())

			// Change mock to return local changes
			mockGitManager.CheckRepoStateFunc = func(repoPath string) (*state.RepositoryState, error) {
				return &state.RepositoryState{
					Name:                  "multi-platform-controller",
					Path:                  repoPath,
					CurrentBranch:         "feature-branch",
					CommitsBehindUpstream: 5,
					HasLocalChanges:       true,
				}, nil
			}

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			updatedState := manager.GetState()
			Expect(updatedState.Repositories["multi-platform-controller"].HasLocalChanges).To(BeTrue())
			Expect(updatedState.Repositories["multi-platform-controller"].CurrentBranch).To(Equal("feature-branch"))
			Expect(updatedState.Repositories["multi-platform-controller"].CommitsBehindUpstream).To(Equal(5))
		})

		It("should handle repository check failures gracefully", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			// Make git check fail
			mockGitManager.CheckRepoStateFunc = func(repoPath string) (*state.RepositoryState, error) {
				return nil, errors.New("repository not accessible")
			}

			err = manager.RefreshState()
			// Should not return an error, but repository should be removed from state
			Expect(err).ToNot(HaveOccurred())

			updatedState := manager.GetState()
			Expect(updatedState.Repositories).To(BeEmpty())
		})

		It("should handle cluster check failures gracefully", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			// Make cluster check fail
			mockClusterManager.StatusFunc = func(ctx context.Context) (string, error) {
				return "", errors.New("cluster check failed")
			}

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			updatedState := manager.GetState()
			// Error is treated as cluster not available, so status becomes "unknown" on refresh
			Expect(updatedState.Cluster.Status).To(Equal("unknown"))
		})

		It("should be thread-safe (concurrent reads and writes)", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			var wg sync.WaitGroup
			numReaders := 5
			numWriters := 5

			// Start readers
			for i := 0; i < numReaders; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 10; j++ {
						_ = manager.GetState()
					}
				}()
			}

			// Start writers
			for i := 0; i < numWriters; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 10; j++ {
						_ = manager.RefreshState()
					}
				}()
			}

			wg.Wait()
			// If we reach here without panicking or deadlocking, the test passes
		})

		It("should update LastSynced timestamp for repositories", func() {
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			initialState := manager.GetState()
			initialLastSynced := initialState.Repositories["multi-platform-controller"].LastSynced

			time.Sleep(10 * time.Millisecond)

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			updatedState := manager.GetState()
			updatedLastSynced := updatedState.Repositories["multi-platform-controller"].LastSynced

			Expect(updatedLastSynced.After(initialLastSynced)).To(BeTrue())
		})

		It("should handle multiple repositories correctly", func() {
			config.RepoPaths = map[string]string{
				"multi-platform-controller": "/home/user/mpc",
				"konflux-ci":                "/home/user/konflux",
				"infra-deployments":         "/home/user/infra",
			}

			checkCount := 0
			mockGitManager.CheckRepoStateFunc = func(repoPath string) (*state.RepositoryState, error) {
				checkCount++
				repoName := ""
				switch repoPath {
				case "/home/user/mpc":
					repoName = "multi-platform-controller"
				case "/home/user/konflux":
					repoName = "konflux-ci"
				case "/home/user/infra":
					repoName = "infra-deployments"
				}

				return &state.RepositoryState{
					Name:                  repoName,
					Path:                  repoPath,
					CurrentBranch:         "main",
					CommitsBehindUpstream: 0,
					HasLocalChanges:       false,
				}, nil
			}

			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			// Reset counter after initialization
			checkCount = 0

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			// Should have called CheckRepoState for all 3 repositories
			Expect(checkCount).To(Equal(3))

			updatedState := manager.GetState()
			Expect(updatedState.Repositories).To(HaveLen(3))
			Expect(updatedState.Repositories).To(HaveKey("multi-platform-controller"))
			Expect(updatedState.Repositories).To(HaveKey("konflux-ci"))
			Expect(updatedState.Repositories).To(HaveKey("infra-deployments"))
		})

		It("should set cluster status to running when cluster manager returns running", func() {
			mockClusterManager.StatusFunc = func(ctx context.Context) (string, error) {
				return "running", nil
			}

			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			currentState := manager.GetState()
			Expect(currentState.Cluster.Status).To(Equal("running"))
			Expect(currentState.Cluster.KonfluxDeployed).To(BeFalse())
		})

		It("should set cluster status to unknown when cluster manager returns error", func() {
			mockClusterManager.StatusFunc = func(ctx context.Context) (string, error) {
				return "", errors.New("cluster not found")
			}

			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			currentState := manager.GetState()
			Expect(currentState.Cluster.Status).To(Equal("unknown"))
		})

		It("should handle MPC deployment state", func() {
			// MPC deployment check is currently a no-op (returns nil)
			manager, err := state.NewStateManager(config)
			Expect(err).ToNot(HaveOccurred())

			initialState := manager.GetState()
			Expect(initialState.MPCDeployment).To(BeNil())

			err = manager.RefreshState()
			Expect(err).ToNot(HaveOccurred())

			updatedState := manager.GetState()
			// MPC deployment check returns nil (not yet implemented)
			Expect(updatedState.MPCDeployment).To(BeNil())
		})
	})
})
