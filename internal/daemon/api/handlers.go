// Package api provides HTTP handlers for the MPC daemon's REST API.
//
// This package implements all API endpoints that the dev-env bash scripts use to
// interact with the MPC development environment. It follows an async operation pattern
// where long-running operations (builds, deployments, TaskRuns) return 202 Accepted
// immediately and execute in background goroutines.
//
// All handlers that perform Kubernetes or Tekton operations delegate to specialized
// packages (build, cluster, deploy, taskrun) and use the StateManager to track
// operation progress and results.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meyrevived/mpc-dev-env/internal/api"
	"github.com/meyrevived/mpc-dev-env/internal/build"
	"github.com/meyrevived/mpc-dev-env/internal/cluster"
	"github.com/meyrevived/mpc-dev-env/internal/config"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/state"
	"github.com/meyrevived/mpc-dev-env/internal/deploy"
	"github.com/meyrevived/mpc-dev-env/internal/git"
	"github.com/meyrevived/mpc-dev-env/internal/prereq"
	"github.com/meyrevived/mpc-dev-env/internal/taskrun"
)

// StateManager abstracts state management operations for tracking the development
// environment's current state and operation progress.
//
// This interface allows handlers to update and retrieve state without direct coupling
// to the state package implementation. It's primarily used for dependency injection
// in testing.
type StateManager interface {
	GetState() state.DevEnvironment
	SetOperationStatus(status string, err error)
	SetTaskRunInfo(info *state.TaskRunInfo)
	ClearTaskRunInfo()
}

// Handlers holds dependencies and state for all HTTP API handlers.
//
// The opMutex ensures that only one build/deploy/rebuild operation can run at a time,
// preventing race conditions and resource conflicts when multiple API calls are made
// concurrently.
type Handlers struct {
	StateManager   StateManager
	Config         *config.Config
	ClusterManager *cluster.Manager
	opMutex        sync.Mutex // Prevents concurrent write operations
}

// NewHandlers creates a new Handlers instance with the provided dependencies.
//
// The StateManager is typically a *state.Manager instance from the main daemon,
// and the Config contains all environment paths and configuration needed for operations.
func NewHandlers(stateManager StateManager, cfg *config.Config) *Handlers {
	return &Handlers{
		StateManager:   stateManager,
		Config:         cfg,
		ClusterManager: cluster.NewManager(cfg),
	}
}

// StatusHandler handles GET /api/status requests.
// It returns the current development environment state as JSON.
func (h *Handlers) StatusHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the current state from the StateManager
	currentState := h.StateManager.GetState()

	// Set Content-Type header
	w.Header().Set("Content-Type", "application/json")

	// Serialize the DevEnvironment struct to JSON and write to response
	if err := json.NewEncoder(w).Encode(currentState); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// RebuildHandler handles POST /api/rebuild requests.
// It triggers the MPC image rebuild asynchronously using native Go and returns 202 Accepted immediately.
// If a rebuild is already in progress, it returns 409 Conflict.
func (h *Handlers) RebuildHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try to acquire the lock. If we can't, a rebuild is already in progress.
	if !h.opMutex.TryLock() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		response := map[string]string{
			"status": "conflict",
			"error":  "A rebuild operation is already in progress",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}

	// Set the operation status to "rebuilding" before starting the goroutine
	h.StateManager.SetOperationStatus("rebuilding", nil)

	// Execute the rebuild asynchronously in a goroutine using native Go build
	// This allows the HTTP request to return immediately
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		// Ensure we unlock the mutex when the goroutine completes
		defer h.opMutex.Unlock()

		log.Println("Starting background rebuild...")

		// Create context with timeout (builds can take several minutes)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		// Call the native Go build function
		if err := build.BuildMPCImage(ctx, h.Config); err != nil {
			log.Printf("ERROR: Background rebuild failed: %v", err)

			// Update state to idle with error message
			h.StateManager.SetOperationStatus("idle", err)
			return
		}
		log.Println("Background rebuild completed successfully.")

		// Update state to idle with no error
		h.StateManager.SetOperationStatus("idle", nil)
	}()

	// Immediately return 202 Accepted with a JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status": "rebuild initiated",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		// If we fail to encode the response, we can't do much at this point
		// since we've already written the status code
		return
	}
}

// SmokeTestHandler handles POST /api/smoke-test requests.
// TODO: Implement native Go smoke test functionality
// Currently returns 501 Not Implemented as native Go implementation is pending
func (h *Handlers) SmokeTestHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return 501 Not Implemented
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)

	response := map[string]string{
		"status":  "not_implemented",
		"message": "Smoke test functionality is not yet implemented in native Go. Please run tests manually.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// DeployMetricsHandler handles POST /api/metrics/deploy requests.
// TODO: Implement native Go metrics deployment functionality
// Currently returns 501 Not Implemented as native Go implementation is pending
func (h *Handlers) DeployMetricsHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return 501 Not Implemented
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)

	response := map[string]string{
		"status":  "not_implemented",
		"message": "Metrics deployment functionality is not yet implemented in native Go. Please deploy metrics manually.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// EnableFeatureRequest represents the JSON request body for POST /api/features/enable.
//
// Currently only "aws-secrets" is supported as a feature. The credentials map should
// contain AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and SSH_KEY_PATH.
type EnableFeatureRequest struct {
	FeatureName string            `json:"feature_name"`
	Credentials map[string]string `json:"credentials"`
}

// EnableFeatureHandler handles POST /api/features/enable requests.
// It enables a feature by configuring AWS secrets using native Go implementation.
// The credentials should include AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and SSH_KEY_PATH
func (h *Handlers) EnableFeatureHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req EnableFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate feature name
	if req.FeatureName == "" {
		http.Error(w, "feature_name is required", http.StatusBadRequest)
		return
	}

	// Currently only support AWS secrets feature
	if req.FeatureName != "aws-secrets" {
		http.Error(w, fmt.Sprintf("Unsupported feature: %s. Only 'aws-secrets' is currently supported.", req.FeatureName), http.StatusBadRequest)
		return
	}

	// Execute the feature enablement asynchronously using native Go
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		log.Printf("Enabling feature: %s", req.FeatureName)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Set environment variables from credentials
		// This allows the ApplySecrets function to use them
		for key, value := range req.Credentials {
			_ = os.Setenv(key, value)
		}

		// Use the native Go secrets deployment
		deployManager := deploy.NewManager(h.Config)
		if err := deployManager.ApplySecrets(ctx); err != nil {
			log.Printf("ERROR: Feature enablement failed for %s: %v", req.FeatureName, err)
			// Clear environment variables on failure
			for key := range req.Credentials {
				_ = os.Unsetenv(key)
			}
			return
		}

		log.Printf("Feature %s enabled successfully", req.FeatureName)

		// Clear environment variables after successful deployment for security
		for key := range req.Credentials {
			_ = os.Unsetenv(key)
		}
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":       "feature enablement initiated",
		"feature_name": req.FeatureName,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// PrerequisitesHandler handles GET /api/prerequisites requests.
// It checks for all required tools and returns their installation status and versions.
func (h *Handlers) PrerequisitesHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create prerequisite checker
	checker := prereq.NewChecker(h.Config)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Run prerequisite checks
	result, err := checker.CheckAll(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check prerequisites: %v", err), http.StatusInternalServerError)
		return
	}

	// Set Content-Type header
	w.Header().Set("Content-Type", "application/json")

	// Return results as JSON
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ClusterStatusHandler handles GET /api/cluster/status requests.
// It returns the current status of the Kind cluster.
func (h *Handlers) ClusterStatusHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get cluster status
	status, err := h.ClusterManager.Status(ctx)

	// Set Content-Type header
	w.Header().Set("Content-Type", "application/json")

	// Build response
	response := api.ClusterStatusResponse{
		Status: status,
	}

	if err != nil {
		response.Error = err.Error()
	}

	// Return response as JSON
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ClusterStartHandler handles POST /api/cluster/start requests.
// It triggers cluster creation asynchronously and returns 202 Accepted immediately.
func (h *Handlers) ClusterStartHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute cluster creation asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		log.Println("Starting cluster creation...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := h.ClusterManager.Create(ctx); err != nil {
			log.Printf("ERROR: Cluster creation failed: %v", err)
			return
		}
		log.Println("Cluster created successfully")
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := api.ClusterOperationResponse{
		Status:  "accepted",
		Message: "Cluster creation initiated. Use GET /api/cluster/status to check progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// ClusterStopHandler handles POST /api/cluster/stop requests.
// It triggers cluster destruction asynchronously and returns 202 Accepted immediately.
func (h *Handlers) ClusterStopHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute cluster destruction asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		log.Println("Starting cluster destruction...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := h.ClusterManager.Destroy(ctx); err != nil {
			log.Printf("ERROR: Cluster destruction failed: %v", err)
			return
		}
		log.Println("Cluster destroyed successfully")
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := api.ClusterOperationResponse{
		Status:  "accepted",
		Message: "Cluster destruction initiated. Use GET /api/cluster/status to check progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// BuildHandler handles POST /api/mpc/build requests.
// It triggers the MPC image build asynchronously and returns 202 Accepted immediately.
// If a build is already in progress, it returns 409 Conflict.
func (h *Handlers) BuildHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try to acquire the lock. If we can't, a build is already in progress.
	if !h.opMutex.TryLock() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		response := map[string]string{
			"status": "conflict",
			"error":  "A build or rebuild operation is already in progress",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}

	// Execute the build asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		// Ensure we unlock the mutex when the goroutine completes
		defer h.opMutex.Unlock()

		log.Println("Starting MPC image build...")

		// Create context with timeout (builds can take several minutes)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		// Call the build function
		if err := build.BuildMPCImage(ctx, h.Config); err != nil {
			log.Printf("ERROR: MPC image build failed: %v", err)
			return
		}

		log.Println("MPC image build completed successfully")
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "MPC image build initiated. Check daemon logs for build progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// DeployHandler handles POST /api/mpc/deploy requests.
// It triggers the MPC deployment asynchronously and returns 202 Accepted immediately.
// If a deployment is already in progress, it returns 409 Conflict.
func (h *Handlers) DeployHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try to acquire the lock. If we can't, a deployment is already in progress.
	if !h.opMutex.TryLock() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		response := map[string]string{
			"status": "conflict",
			"error":  "A build, rebuild, or deployment operation is already in progress",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}

	// Execute the deployment asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		// Ensure we unlock the mutex when the goroutine completes
		defer h.opMutex.Unlock()

		// Set operation status to "deploying_mpc" at the start
		h.StateManager.SetOperationStatus("deploying_mpc", nil)

		log.Println("Starting MPC deployment...")

		// Create context with timeout (deployments can take several minutes)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		// Call the deploy function
		if err := deploy.DeployMPC(ctx, h.Config); err != nil {
			log.Printf("ERROR: MPC deployment failed: %v", err)
			h.StateManager.SetOperationStatus("idle", err)
			return
		}

		log.Println("MPC deployment completed successfully")
		h.StateManager.SetOperationStatus("idle", nil)
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "MPC deployment initiated. Check daemon logs for deployment progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// RebuildAndRedeployHandler handles POST /api/mpc/rebuild-and-redeploy requests.
// It orchestrates the full rebuild and redeploy workflow by calling build and deploy in sequence.
// This is the primary endpoint for the live-debugging workflow.
// If an operation is already in progress, it returns 409 Conflict.
func (h *Handlers) RebuildAndRedeployHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try to acquire the lock. If we can't, an operation is already in progress.
	if !h.opMutex.TryLock() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		response := map[string]string{
			"status": "conflict",
			"error":  "A build, rebuild, or deployment operation is already in progress",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}

	// Execute the rebuild-and-redeploy workflow asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		// Ensure we unlock the mutex when the goroutine completes
		defer h.opMutex.Unlock()

		// Set operation status to "rebuilding_and_redeploying" at the start
		h.StateManager.SetOperationStatus("rebuilding_and_redeploying", nil)

		log.Println("Starting rebuild-and-redeploy orchestration...")

		// Create context with timeout (both operations can take time)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		// Step 1: Build the MPC image
		log.Println("[Orchestration] Step 1/2: Building MPC image...")
		if err := build.BuildMPCImage(ctx, h.Config); err != nil {
			log.Printf("ERROR: Rebuild-and-redeploy failed during build: %v", err)
			h.StateManager.SetOperationStatus("idle", err)
			return
		}
		log.Println("[Orchestration] Build completed successfully")

		// Step 2: Deploy the MPC to the cluster
		log.Println("[Orchestration] Step 2/2: Deploying MPC to cluster...")
		if err := deploy.DeployMPC(ctx, h.Config); err != nil {
			log.Printf("ERROR: Rebuild-and-redeploy failed during deploy: %v", err)
			h.StateManager.SetOperationStatus("idle", err)
			return
		}
		log.Println("[Orchestration] Deploy completed successfully")

		log.Println("Rebuild-and-redeploy orchestration completed successfully!")

		// Set operation status back to idle (no error)
		h.StateManager.SetOperationStatus("idle", nil)
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "Rebuild-and-redeploy orchestration initiated. Check daemon logs for progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// GitSyncHandler handles POST /api/git/sync requests.
// It triggers Git synchronization for all configured repositories asynchronously
// and returns 202 Accepted immediately.
func (h *Handlers) GitSyncHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute Git sync asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		log.Println("Starting Git repository synchronization...")

		// Create context with timeout (sync operations can take time)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Create a new Syncer instance
		syncer := git.NewSyncer(h.Config)

		// Synchronize all repositories
		if err := syncer.SyncAllRepos(ctx); err != nil {
			log.Printf("ERROR: Git synchronization failed: %v", err)
			return
		}

		log.Println("Git repository synchronization completed successfully")
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "Git synchronization initiated. Check daemon logs for sync progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// DeploySecretsHandler handles POST /api/deploy/secrets requests.
// It triggers the deployment of AWS secrets to the Kubernetes cluster asynchronously
// and returns 202 Accepted immediately.
func (h *Handlers) DeploySecretsHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute secrets deployment asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		// Set operation status to "deploying_secrets" at the start
		h.StateManager.SetOperationStatus("deploying_secrets", nil)

		log.Println("Starting AWS secrets deployment...")

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Create deployment manager and apply secrets
		deployManager := deploy.NewManager(h.Config)
		if err := deployManager.ApplySecrets(ctx); err != nil {
			log.Printf("ERROR: Secrets deployment failed: %v", err)
			h.StateManager.SetOperationStatus("idle", err)
			return
		}

		log.Println("AWS secrets deployment completed successfully")

		// Set operation status back to idle (no error)
		h.StateManager.SetOperationStatus("idle", nil)
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "Secrets deployment initiated. Check daemon logs for progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// DeployKonfluxHandler handles POST /api/deploy/konflux requests.
// It triggers the deployment of Konflux to the Kind cluster asynchronously
// and returns 202 Accepted immediately.
func (h *Handlers) DeployKonfluxHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute Konflux deployment asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		// Set operation status to "deploying_konflux" at the start
		h.StateManager.SetOperationStatus("deploying_konflux", nil)

		log.Println("Starting Konflux deployment...")

		// Create context with timeout (Konflux deployment can take 20+ minutes)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		// Create deployment manager and apply Konflux
		deployManager := deploy.NewManager(h.Config)
		if err := deployManager.ApplyKonflux(ctx); err != nil {
			log.Printf("ERROR: Konflux deployment failed: %v", err)
			h.StateManager.SetOperationStatus("idle", err)
			return
		}

		log.Println("Konflux deployment completed successfully")

		// Set operation status back to idle (no error)
		h.StateManager.SetOperationStatus("idle", nil)
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "Konflux deployment initiated. This may take 20-30 minutes. Check daemon logs for progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// DeployMinimalStackHandler handles POST /api/deploy/minimal-stack requests.
// It triggers the deployment of the minimal MPC stack (Tekton + MPC Operator + OTP)
// to the Kind cluster asynchronously and returns 202 Accepted immediately.
func (h *Handlers) DeployMinimalStackHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute minimal stack deployment asynchronously in a goroutine
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go func() {
		// Set operation status to "deploying_minimal_stack" at the start
		h.StateManager.SetOperationStatus("deploying_minimal_stack", nil)

		log.Println("Starting minimal MPC stack deployment...")

		// Create context with timeout (minimal deployment should be fast, ~5 minutes)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Create minimal deployer and deploy the stack
		minimalDeployer := deploy.NewMinimalDeployer(h.Config)
		if err := minimalDeployer.DeployMinimalStack(ctx); err != nil {
			log.Printf("ERROR: Minimal stack deployment failed: %v", err)
			h.StateManager.SetOperationStatus("idle", err)
			return
		}

		log.Println("Minimal stack deployment completed successfully")

		// Set operation status back to idle (no error)
		h.StateManager.SetOperationStatus("idle", nil)
	}()

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "Minimal stack deployment initiated. This should take 2-3 minutes. Check daemon logs for progress.",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// TaskRunRunRequest represents the JSON request body for POST /api/taskrun/run.
//
// The YAMLPath should point to a valid Tekton TaskRun YAML file on the filesystem.
// This is typically a file in the taskruns/ directory.
type TaskRunRunRequest struct {
	YAMLPath string `json:"yaml_path"`
}

// TaskRunRunHandler handles POST /api/taskrun/run requests.
// It triggers the complete TaskRun workflow asynchronously and returns 202 Accepted immediately.
// The workflow includes: applying TaskRun, monitoring status, streaming logs to file, and updating state.
func (h *Handlers) TaskRunRunHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req TaskRunRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate YAML path
	if req.YAMLPath == "" {
		http.Error(w, "yaml_path is required", http.StatusBadRequest)
		return
	}

	// Start async operation
	//nolint:contextcheck // Using Background context intentionally - request context would cancel when response is sent
	go h.runTaskRunWorkflow(context.Background(), req.YAMLPath)

	// Immediately return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := map[string]string{
		"status":  "accepted",
		"message": "TaskRun workflow started",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// runTaskRunWorkflow runs the complete TaskRun workflow asynchronously.
//
// This method coordinates the entire TaskRun lifecycle:
//  1. Updates state to "running_taskrun" and clears previous TaskRun info
//  2. Generates log filename and ensures logs directory exists
//  3. Creates TaskRun manager and delegates to its RunTaskRunWorkflow method
//  4. Updates state with final results (name, status, log location)
//
// All Kubernetes and Tekton operations are handled by the taskrun.Manager.
// This handler only orchestrates the workflow and manages state updates.
func (h *Handlers) runTaskRunWorkflow(ctx context.Context, yamlPath string) {
	// Update operation status to running_taskrun
	h.StateManager.SetOperationStatus("running_taskrun", nil)
	h.StateManager.ClearTaskRunInfo() // Clear previous TaskRun info

	// Generate log filename based on TaskRun YAML filename
	logFilename := generateLogFilename(yamlPath)
	logPath := filepath.Join(os.Getenv("MPC_DEV_ENV_PATH"), "logs", logFilename)

	// Ensure logs directory exists
	logsDir := filepath.Join(os.Getenv("MPC_DEV_ENV_PATH"), "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("ERROR: Failed to create logs directory: %v", err)
		h.StateManager.SetOperationStatus("idle", err)
		h.StateManager.SetTaskRunInfo(&state.TaskRunInfo{
			Status: "Error",
		})
		return
	}

	// Create TaskRun manager
	mgr, err := taskrun.NewManager()
	if err != nil {
		errMsg := fmt.Errorf("failed to create TaskRun manager: %w", err)
		log.Printf("ERROR: %v", errMsg)
		h.StateManager.SetOperationStatus("idle", errMsg)
		return
	}

	// Run the workflow
	log.Printf("Starting TaskRun workflow for: %s", yamlPath)
	name, status, err := mgr.RunTaskRunWorkflow(ctx, yamlPath, logPath)

	// Update state with results
	if err != nil {
		errMsg := fmt.Errorf("TaskRun workflow failed: %w", err)
		log.Printf("ERROR: %v", errMsg)
		h.StateManager.SetOperationStatus("idle", errMsg)
		return
	}

	// Success - store TaskRun info
	log.Printf("TaskRun workflow completed: name=%s, status=%s", name, status)
	h.StateManager.SetOperationStatus("idle", nil)
	h.StateManager.SetTaskRunInfo(&state.TaskRunInfo{
		Name:      name,
		Status:    status,
		LogFile:   logPath,
		StartTime: time.Now().Format(time.RFC3339),
	})

	if status == "Failed" {
		errMsg := fmt.Errorf("TaskRun '%s' failed - check logs at %s", name, logPath)
		h.StateManager.SetOperationStatus("idle", errMsg)
	}
}

// generateLogFilename generates a timestamped log filename from the TaskRun YAML path.
//
// The format is: <yaml-basename>_YYYYMMDD_HHMMSS.log
// For example: localhost_test_20251130_143052.log
func generateLogFilename(yamlPath string) string {
	base := filepath.Base(yamlPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s.log", base, timestamp)
}
