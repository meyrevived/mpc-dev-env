// Package state defines data models for tracking the MPC development environment state.
//
// The DevEnvironment struct is the root state object that contains all information about
// the running development environment: cluster status, repository states, MPC deployment info,
// enabled features, and current operation status. This state is exposed via the /api/status
// endpoint and is used by bash scripts to make decisions about workflow progression.
package state

import "time"

// ClusterState represents the state of the Kind cluster.
//
// Status can be "running", "paused", or "stopped". The daemon updates this state
// when cluster operations (create, destroy) complete.
type ClusterState struct {
	Name            string    `json:"name"`
	CreatedAt       time.Time `json:"created_at"`
	Status          string    `json:"status"` // "running" | "paused" | "stopped"
	KubeconfigPath  string    `json:"kubeconfig_path"`
	KonfluxDeployed bool      `json:"konflux_deployed"`
}

// RepositoryState represents the state of a Git repository.
//
// This tracks the current branch, sync status, and whether there are uncommitted changes.
// The daemon's Git manager updates these fields during sync operations.
type RepositoryState struct {
	Name                  string    `json:"name"`
	Path                  string    `json:"path"`
	CurrentBranch         string    `json:"current_branch"`
	LastSynced            time.Time `json:"last_synced"`
	CommitsBehindUpstream int       `json:"commits_behind_upstream"`
	HasLocalChanges       bool      `json:"has_local_changes"`
}

// MPCDeployment represents the MPC deployment state.
//
// This tracks which images are deployed and the Git hash of the source code they were built from.
// Updated after successful MPC builds and deployments.
type MPCDeployment struct {
	ControllerImage string    `json:"controller_image"`
	OTPImage        string    `json:"otp_image"`
	DeployedAt      time.Time `json:"deployed_at"`
	SourceGitHash   string    `json:"source_git_hash"`
}

// FeatureState represents the enabled/disabled state of cloud provider features.
//
// Currently tracks AWS and IBM cloud provider secrets. Features are enabled when
// their respective secrets are deployed to the Kubernetes cluster.
type FeatureState struct {
	AWSEnabled bool `json:"aws_enabled"`
	IBMEnabled bool `json:"ibm_enabled"`
}

// TaskRunInfo represents information about a TaskRun execution.
//
// This stores the results of the most recent TaskRun workflow. The bash scripts
// read this information via /api/status to display results and make cleanup decisions.
type TaskRunInfo struct {
	Name      string `json:"name,omitempty"`
	Status    string `json:"status,omitempty"`
	LogFile   string `json:"log_file,omitempty"`
	StartTime string `json:"start_time,omitempty"`
}

// DevEnvironment represents the top-level development environment state.
//
// This is the primary state object returned by GET /api/status. It provides a complete
// snapshot of the current development environment including:
//   - Cluster status and configuration
//   - Repository sync states
//   - MPC deployment information
//   - Enabled features (cloud providers)
//   - Current operation status (idle, rebuilding, running_taskrun, etc.)
//   - Any errors from the last operation
//   - Most recent TaskRun results
//
// The bash scripts poll this endpoint to track operation progress and make workflow decisions.
type DevEnvironment struct {
	SessionID          string                     `json:"session_id"`
	CreatedAt          time.Time                  `json:"created_at"`
	LastActive         time.Time                  `json:"last_active"`
	Cluster            ClusterState               `json:"cluster"`
	Repositories       map[string]RepositoryState `json:"repositories"`
	MPCDeployment      *MPCDeployment             `json:"mpc_deployment"`
	Features           FeatureState               `json:"features"`
	OperationStatus    string                     `json:"operation_status"`       // e.g., "idle", "rebuilding", "configuring_aws", "running_taskrun"
	LastOperationError string                     `json:"last_operation_error"`   // stores error messages from background operations
	TaskRunInfo        *TaskRunInfo               `json:"taskrun_info,omitempty"` // information about the most recent TaskRun
}

// ChangeSet represents detected changes in a repository.
//
// Used by the Git sync functionality to summarize what changed and whether
// it potentially affects the MPC deployment (requiring a rebuild).
type ChangeSet struct {
	RepoName              string   `json:"repo_name"`
	Commits               []string `json:"commits"`
	FilesChanged          []string `json:"files_changed"`
	PotentiallyAffectsMPC bool     `json:"potentially_affects_mpc"`
	ImpactSummary         string   `json:"impact_summary"`
	HasUpdates            bool     `json:"has_updates"`
}

// TestResult represents the result of running tests.
//
// Currently not actively used but reserved for future test automation features.
type TestResult struct {
	Passed          bool    `json:"passed"`
	DurationSeconds int     `json:"duration_seconds"`
	Output          string  `json:"output"`
	Error           *string `json:"error"`
}

// TaskRunResult represents the result of a Tekton TaskRun.
//
// Similar to TestResult, reserved for future use. Current TaskRun information
// is stored in TaskRunInfo instead.
type TaskRunResult struct {
	Name            string  `json:"name"`
	Succeeded       bool    `json:"succeeded"`
	DurationSeconds int     `json:"duration_seconds"`
	Logs            string  `json:"logs"`
	Error           *string `json:"error"`
}

// MetricsConfig represents the configuration for Prometheus/Grafana.
//
// Reserved for future metrics integration. The daemon currently doesn't deploy
// or configure metrics systems.
type MetricsConfig struct {
	PrometheusEnabled bool   `json:"prometheus_enabled"`
	GrafanaEnabled    bool   `json:"grafana_enabled"`
	PrometheusURL     string `json:"prometheus_url"`
	GrafanaURL        string `json:"grafana_url"`
	RetentionDays     int    `json:"retention_days"`
}
