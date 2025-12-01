// Package api defines shared API models used across the daemon's HTTP API.
//
// This package contains request and response types that are used by the API handlers
// but may also be imported by other packages (like cluster management) to maintain
// consistent API contracts.
package api

// ClusterStatusResponse represents the JSON response for GET /api/cluster/status.
//
// The Status field indicates the current state of the Kind cluster:
//   - "running": Cluster is active and accessible
//   - "not_running": Cluster doesn't exist or is stopped
//   - "unknown": Status could not be determined
//
// The Error field is populated only when status checking fails.
type ClusterStatusResponse struct {
	Status string `json:"status"` // One of: "Running", "Not Running", "Error"
	Error  string `json:"error,omitempty"`
}

// ClusterOperationResponse represents the JSON response for async cluster operations.
//
// Used by:
//   - POST /api/cluster/start
//   - POST /api/cluster/stop
//
// These endpoints return 202 Accepted immediately with a status of "accepted" and a
// message instructing the client to poll GET /api/cluster/status to check progress.
type ClusterOperationResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
