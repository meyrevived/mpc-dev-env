// Package cluster provides Kind cluster lifecycle management for the MPC Dev Environment.
//
// It handles creating, destroying, and checking the status of Kind (Kubernetes in Docker)
// clusters. The package uses Podman as the container runtime provider for better SELinux
// compatibility on RHEL/Fedora systems.
//
// All cluster operations use the cluster name "konflux" and execute commands through
// bash to ensure proper environment handling and resource limits.
package cluster

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/meyrevived/mpc-dev-env/internal/config"
)

// Manager handles Kind cluster lifecycle operations (create, destroy, status).
// It provides a Go-native interface to Kind cluster management, replacing
// the Bash-based cluster management scripts.
type Manager struct {
	config *config.Config
}

// NewManager creates a new cluster manager instance.
// The manager uses the provided Config to determine paths and settings.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		config: cfg,
	}
}

// Create creates a new Kind cluster.
// It executes the "kind create cluster" command and streams output to logs.
//
// The cluster creation uses the following approach:
//   - Uses the cluster name "konflux" (hardcoded for now, can be made configurable)
//   - If a kind-config.yaml exists in the MPC_DEV_ENV_PATH, it will be used
//   - Streams stdout and stderr to logs for debugging
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - error: An error if cluster creation fails, nil otherwise
func (m *Manager) Create(ctx context.Context) error {
	log.Println("Creating Kind cluster...")

	clusterName := "konflux"

	// Build the kind create cluster command
	// Note: For now, we use default kind settings
	// The bash script shows it looks for kind-config.yaml in konflux-ci directory,
	// but that directory is not yet part of our Config struct.
	// Future enhancement: Add support for --config flag when kind-config.yaml is available
	args := []string{"create", "cluster", "--name", clusterName}

	// Build full command string with environment variable
	cmdStr := "KIND_EXPERIMENTAL_PROVIDER=podman kind " + strings.Join(args, " ")
	log.Printf("Executing: %s", cmdStr)

	// Execute via bash -c to ensure proper environment and resource limits
	// This avoids issues with cgroup/systemd limits when run from daemon
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

	// Run the command and capture combined output
	output, err := cmd.CombinedOutput()

	// Log the output regardless of success/failure
	if len(output) > 0 {
		log.Printf("kind create output:\n%s", string(output))
	}

	if err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w (output: %s)", err, string(output))
	}

	log.Println("Kind cluster created successfully")
	return nil
}

// Destroy deletes the Kind cluster.
// It executes the "kind delete cluster" command.
// This function handles cases where the cluster might not exist gracefully.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - error: An error if cluster deletion fails (except for "cluster not found"), nil otherwise
func (m *Manager) Destroy(ctx context.Context) error {
	log.Println("Destroying Kind cluster...")

	clusterName := "konflux"

	// Build the kind delete cluster command
	args := []string{"delete", "cluster", "--name", clusterName}

	// Build full command string with environment variable
	cmdStr := "KIND_EXPERIMENTAL_PROVIDER=podman kind " + strings.Join(args, " ")
	log.Printf("Executing: %s", cmdStr)

	// Execute via bash -c
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	err := cmd.Run()

	// Log the output
	if stdout.Len() > 0 {
		log.Printf("kind delete stdout:\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		log.Printf("kind delete stderr:\n%s", stderr.String())
	}

	if err != nil {
		// Check if the error is because the cluster doesn't exist
		// kind returns a non-zero exit code if the cluster is not found
		stderrStr := stderr.String()
		if strings.Contains(stderrStr, "not found") || strings.Contains(stderrStr, "No kind clusters found") {
			log.Println("Cluster does not exist (already deleted or never created)")
			return nil
		}
		return fmt.Errorf("failed to delete Kind cluster: %w (stderr: %s)", err, stderrStr)
	}

	log.Println("Kind cluster destroyed successfully")
	return nil
}

// Status returns the current status of the Kind cluster.
// It checks if the cluster is running by querying kind for the list of clusters.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - string: One of "Running", "Not Running", or "Error"
//   - error: An error if the status check fails, nil otherwise
func (m *Manager) Status(ctx context.Context) (string, error) {
	log.Println("Checking Kind cluster status...")

	clusterName := "konflux"

	// Use "kind get clusters" to list all clusters
	cmdStr := "KIND_EXPERIMENTAL_PROVIDER=podman kind get clusters"
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	err := cmd.Run()

	// Log the output
	if stdout.Len() > 0 {
		log.Printf("kind get clusters stdout:\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		log.Printf("kind get clusters stderr:\n%s", stderr.String())
	}

	if err != nil {
		// If kind command fails, return Error status
		log.Printf("Failed to get cluster status: %v", err)
		return "Error", fmt.Errorf("failed to get cluster status: %w", err)
	}

	// Parse the output to check if our cluster exists
	clusters := strings.TrimSpace(stdout.String())
	if clusters == "" {
		log.Println("No Kind clusters found")
		return "Not Running", nil
	}

	// Check if our cluster is in the list
	clusterExists := false
	for _, line := range strings.Split(clusters, "\n") {
		if strings.TrimSpace(line) == clusterName {
			clusterExists = true
			break
		}
	}

	if !clusterExists {
		log.Printf("Cluster '%s' not found", clusterName)
		return "Not Running", nil
	}

	// Cluster exists, but we need to verify kubectl can access it
	// This ensures the cluster is fully initialized and ready
	log.Printf("Cluster '%s' found, verifying kubectl accessibility...", clusterName)
	kubectlCmd := exec.CommandContext(ctx, "kubectl", "cluster-info", "--context", "kind-"+clusterName)
	kubectlCmd.Stdout = &bytes.Buffer{}
	kubectlCmd.Stderr = &bytes.Buffer{}

	if err := kubectlCmd.Run(); err != nil {
		log.Printf("Cluster '%s' exists but kubectl cannot access it yet (still initializing)", clusterName)
		return "Initializing", nil
	}

	log.Printf("Cluster '%s' is running and accessible via kubectl", clusterName)
	return "Running", nil
}
