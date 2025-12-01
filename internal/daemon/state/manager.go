package state

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// GitManager abstracts Git operations for repository state checking.
//
// This interface allows the StateManager to query Git repository state without
// direct coupling to the git package implementation.
type GitManager interface {
	CheckRepoState(repoPath string) (*RepositoryState, error)
}

// ClusterManager abstracts cluster operations for status checking.
//
// This interface allows the StateManager to query Kind cluster status without
// direct coupling to the cluster package implementation.
type ClusterManager interface {
	Status(ctx context.Context) (string, error)
}

// StateManager manages the in-memory development environment state.
//
// Unlike the Python version, this manager queries the live environment on demand
// rather than loading from disk. It uses mutexes to ensure thread-safe concurrent
// access from multiple HTTP request handlers.
//
// The manager maintains a single DevEnvironment struct that is updated through
// RefreshState() calls and operation status updates (SetOperationStatus, SetTaskRunInfo).
// State is exposed to HTTP handlers via GetState() which returns a copy to prevent
// external modifications.
type StateManager struct {
	mu sync.RWMutex

	// Current in-memory state
	state DevEnvironment

	// Dependencies
	gitManager     GitManager
	clusterManager ClusterManager
	repoPaths      map[string]string // map[repoName]repoPath
	kubeconfigPath string
}

// StateManagerConfig holds configuration for creating a StateManager.
//
// All fields are required except RepoPaths which can be empty if no repositories
// need to be tracked.
type StateManagerConfig struct {
	GitManager     GitManager
	ClusterManager ClusterManager
	RepoPaths      map[string]string // map[repoName]repoPath (e.g., "multi-platform-controller" -> "/home/user/mpc/...")
	KubeconfigPath string
}

// NewStateManager creates a new StateManager instance and performs an initial
// scan of the environment to populate the DevEnvironment state.
//
// Args:
//
//	config: Configuration containing all required dependencies and paths
//
// Returns:
//
//	A new StateManager instance with initial state populated
//	An error if the initial state scan fails
//
// Example:
//
//	config := &StateManagerConfig{
//	    GitManager:   gitManager,
//	    ClusterManager: clusterManager,
//	    RepoPaths: map[string]string{
//	        "multi-platform-controller": "/home/user/mpc/multi-platform-controller",
//	    },
//	    KubeconfigPath: "/home/user/.kube/config",
//	}
//	manager, err := NewStateManager(config)
func NewStateManager(config *StateManagerConfig) (*StateManager, error) {
	if config.GitManager == nil {
		return nil, errors.New("GitManager is required")
	}
	if config.ClusterManager == nil {
		return nil, errors.New("ClusterManager is required")
	}

	manager := &StateManager{
		gitManager:     config.GitManager,
		clusterManager: config.ClusterManager,
		repoPaths:      config.RepoPaths,
		kubeconfigPath: config.KubeconfigPath,
	}

	// Perform initial state scan
	if err := manager.initialScan(); err != nil {
		return nil, fmt.Errorf("failed to perform initial state scan: %w", err)
	}

	return manager, nil
}

// initialScan performs the first scan of the environment on startup to populate
// the initial DevEnvironment state.
//
// This method:
//  1. Generates a new session ID (UUID)
//  2. Checks cluster status
//  3. Scans all configured Git repositories
//  4. Checks MPC deployment status
//  5. Initializes feature states to disabled
//
// Unlike RefreshState, initialScan sets up the entire state structure from scratch.
func (m *StateManager) initialScan() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize a new DevEnvironment with a fresh session ID
	sessionID := uuid.New().String()
	now := time.Now()

	newState := DevEnvironment{
		SessionID:          sessionID,
		CreatedAt:          now,
		LastActive:         now,
		Repositories:       make(map[string]RepositoryState),
		OperationStatus:    "idle",
		LastOperationError: "",
	}

	// Check cluster state
	clusterState, err := m.checkClusterState()
	if err != nil {
		// If cluster check fails, set a default empty state
		newState.Cluster = ClusterState{
			Name:   "",
			Status: "unknown",
		}
	} else {
		newState.Cluster = clusterState
	}

	// Check repository states
	for repoName, repoPath := range m.repoPaths {
		repoState, err := m.gitManager.CheckRepoState(repoPath)
		if err != nil {
			// Skip repositories that fail to check (they might not exist yet)
			continue
		}
		repoState.LastSynced = now
		newState.Repositories[repoName] = *repoState
	}

	// Check MPC deployment state
	mpcDeployment, err := m.checkMPCDeployment()
	if err != nil {
		// If MPC deployment check fails, set to nil (not deployed)
		newState.MPCDeployment = nil
	} else {
		newState.MPCDeployment = mpcDeployment
	}

	// Initialize feature state (default: disabled)
	newState.Features = FeatureState{
		AWSEnabled: false,
		IBMEnabled: false,
	}

	m.state = newState
	return nil
}

// GetState returns a copy of the current DevEnvironment state.
// This method is thread-safe and uses a read lock.
//
// Returns:
//
//	A copy of the current DevEnvironment struct
func (m *StateManager) GetState() DevEnvironment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modifications
	return m.state
}

// RefreshState queries the live environment and updates the in-memory state.
// This is the core logic of the StateManager. It calls GitManager to check
// repository states and native Go cluster manager to check cluster and MPC deployment states.
// This method is thread-safe and uses a write lock.
//
// Returns:
//
//	An error if any critical checks fail
//
// Example:
//
//	if err := manager.RefreshState(); err != nil {
//	    log.Printf("Failed to refresh state: %v", err)
//	}
func (m *StateManager) RefreshState() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Update LastActive timestamp
	m.state.LastActive = now

	// Check cluster state
	clusterState, err := m.checkClusterState()
	if err != nil {
		// Log the error but don't fail the entire refresh
		// Set cluster status to unknown
		m.state.Cluster.Status = "unknown"
	} else {
		m.state.Cluster = clusterState
	}

	// Check repository states
	for repoName, repoPath := range m.repoPaths {
		repoState, err := m.gitManager.CheckRepoState(repoPath)
		if err != nil {
			// If a repo check fails, keep the old state or remove it
			delete(m.state.Repositories, repoName)
			continue
		}
		repoState.LastSynced = now
		m.state.Repositories[repoName] = *repoState
	}

	// Check MPC deployment state
	mpcDeployment, err := m.checkMPCDeployment()
	if err != nil {
		// If MPC deployment check fails, set to nil (not deployed)
		m.state.MPCDeployment = nil
	} else {
		m.state.MPCDeployment = mpcDeployment
	}

	return nil
}

// checkClusterState queries the cluster status using the native Go cluster manager.
//
// This is a private helper method called by RefreshState and initialScan. It uses
// the ClusterManager interface to get the current Kind cluster status with a 10-second timeout.
func (m *StateManager) checkClusterState() (ClusterState, error) {
	// Use the native Go cluster manager to get status
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, err := m.clusterManager.Status(ctx)
	if err != nil {
		// Return error so caller can decide how to handle it
		return ClusterState{}, err
	}

	// Parse status to determine cluster state
	// Status can be: "running", "not_running", or an error message
	clusterState := ClusterState{
		Name:            "konflux",  // Kind cluster name
		CreatedAt:       time.Now(), // TODO: Get actual creation time from cluster
		Status:          status,
		KubeconfigPath:  m.kubeconfigPath,
		KonfluxDeployed: false, // TODO: Check if Konflux is deployed
	}

	return clusterState, nil
}

// checkMPCDeployment queries the MPC deployment status.
//
// This is a private helper method called by RefreshState and initialScan.
//
// TODO: Implement native Go deployment status checking using kubectl commands.
// Future implementation should:
//  1. Use kubectl to check if multi-platform-controller deployment exists
//  2. Get deployment image tags
//  3. Get deployment timestamps
func (m *StateManager) checkMPCDeployment() (*MPCDeployment, error) {
	// TODO: Implement native Go deployment status checking
	// For now, return nil to indicate no deployment information available
	// Future implementation should:
	// 1. Use kubectl to check if multi-platform-controller deployment exists
	// 2. Get deployment image tags
	// 3. Get deployment timestamps
	return nil, nil
}

// SetOperationStatus updates the operation status and error message in the state.
// This method is thread-safe and uses a write lock.
//
// Args:
//
//	status: The new operation status (e.g., "idle", "rebuilding", "configuring_aws")
//	err: An error object if the operation failed, or nil if successful
//
// Example:
//
//	manager.SetOperationStatus("rebuilding", nil)
//	manager.SetOperationStatus("idle", fmt.Errorf("rebuild failed"))
func (m *StateManager) SetOperationStatus(status string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.OperationStatus = status
	if err != nil {
		m.state.LastOperationError = err.Error()
	} else {
		m.state.LastOperationError = ""
	}
	m.state.LastActive = time.Now()
}

// SetTaskRunInfo updates the TaskRun information in the state.
// This method is thread-safe and uses a write lock.
//
// Args:
//
//	info: A pointer to TaskRunInfo containing the TaskRun details
//
// Example:
//
//	manager.SetTaskRunInfo(&TaskRunInfo{
//	    Name:      "my-taskrun",
//	    Status:    "Succeeded",
//	    LogFile:   "/path/to/logs/my-taskrun.log",
//	    StartTime: "2025-11-27T14:30:52Z",
//	})
func (m *StateManager) SetTaskRunInfo(info *TaskRunInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.TaskRunInfo = info
	m.state.LastActive = time.Now()
}

// ClearTaskRunInfo clears the TaskRun information from the state.
//
// This is typically called at the start of a new TaskRun workflow to ensure
// previous TaskRun data doesn't interfere. This method is thread-safe and uses
// a write lock.
func (m *StateManager) ClearTaskRunInfo() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.TaskRunInfo = nil
	m.state.LastActive = time.Now()
}
