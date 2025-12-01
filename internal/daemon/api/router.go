package api

import (
	"net/http"
)

// NewRouter creates and configures a new HTTP router with all API endpoints.
// It registers handlers for the correct paths and HTTP methods.
//
// Args:
//
//	handlers: A Handlers instance containing the StateManager and Config dependencies
//
// Returns:
//
//	A configured *http.ServeMux with all routes registered
//
// Example:
//
//	handlers := NewHandlers(stateManager, cfg)
//	router := NewRouter(handlers)
//	http.ListenAndServe(":8765", router)
func NewRouter(handlers *Handlers) *http.ServeMux {
	mux := http.NewServeMux()

	// Register GET /api/status - Returns current environment state
	mux.HandleFunc("/api/status", handlers.StatusHandler)

	// Register POST /api/rebuild - Triggers rebuild asynchronously
	mux.HandleFunc("/api/rebuild", handlers.RebuildHandler)

	// Register POST /api/smoke-test - Triggers smoke test asynchronously
	mux.HandleFunc("/api/smoke-test", handlers.SmokeTestHandler)

	// Register POST /api/metrics/deploy - Deploys metrics stack (Prometheus + Grafana)
	mux.HandleFunc("/api/metrics/deploy", handlers.DeployMetricsHandler)

	// Register POST /api/features/enable - Enables a feature with credentials
	mux.HandleFunc("/api/features/enable", handlers.EnableFeatureHandler)

	// Register GET /api/prerequisites - Returns prerequisite check results
	mux.HandleFunc("/api/prerequisites", handlers.PrerequisitesHandler)

	// Register GET /api/cluster/status - Returns cluster status
	mux.HandleFunc("/api/cluster/status", handlers.ClusterStatusHandler)

	// Register POST /api/cluster/start - Starts the cluster asynchronously
	mux.HandleFunc("/api/cluster/start", handlers.ClusterStartHandler)

	// Register POST /api/cluster/stop - Stops the cluster asynchronously
	mux.HandleFunc("/api/cluster/stop", handlers.ClusterStopHandler)

	// Register POST /api/mpc/build - Builds MPC container image asynchronously
	mux.HandleFunc("/api/mpc/build", handlers.BuildHandler)

	// Register POST /api/mpc/deploy - Deploys MPC to the cluster asynchronously
	mux.HandleFunc("/api/mpc/deploy", handlers.DeployHandler)

	// Register POST /api/mpc/rebuild-and-redeploy - Orchestrates build and deploy workflow asynchronously
	mux.HandleFunc("/api/mpc/rebuild-and-redeploy", handlers.RebuildAndRedeployHandler)

	// Register POST /api/git/sync - Synchronizes all Git repositories asynchronously
	mux.HandleFunc("/api/git/sync", handlers.GitSyncHandler)

	// Register POST /api/deploy/secrets - Deploys AWS secrets to the cluster asynchronously
	mux.HandleFunc("/api/deploy/secrets", handlers.DeploySecretsHandler)

	// Register POST /api/deploy/konflux - Deploys Konflux to the cluster asynchronously
	mux.HandleFunc("/api/deploy/konflux", handlers.DeployKonfluxHandler)

	// Register POST /api/deploy/minimal-stack - Deploys minimal MPC stack (Tekton + MPC + OTP) asynchronously
	mux.HandleFunc("/api/deploy/minimal-stack", handlers.DeployMinimalStackHandler)

	// Register POST /api/taskrun/run - Runs a TaskRun workflow asynchronously
	mux.HandleFunc("/api/taskrun/run", handlers.TaskRunRunHandler)

	return mux
}
