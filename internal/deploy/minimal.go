package deploy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/meyrevived/mpc-dev-env/internal/config"
)

const (
	tektonNamespace     = "tekton-pipelines"
	tektonReleaseURL    = "https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml"
	minimalStackTimeout = 10 * time.Minute
)

// MinimalDeployer handles deployment of the minimal MPC stack.
//
// The minimal stack consists of only the essential components needed for MPC to function:
//   - Tekton Pipelines (TaskRun execution engine)
//   - MPC Operator (controller deployment)
//   - OTP Server (one-time password service)
//
// This replaces the bloated Konflux deployment that includes many unnecessary components
// like ArgoCD, Kyverno, Dex, etc. The minimal deployment is faster (~3-5 minutes vs 30-45 minutes).
type MinimalDeployer struct {
	config *config.Config
}

// NewMinimalDeployer creates a new minimal deployment manager instance.
//
// The manager uses the provided config to locate MPC repository paths and manifests.
func NewMinimalDeployer(cfg *config.Config) *MinimalDeployer {
	return &MinimalDeployer{
		config: cfg,
	}
}

// DeployMinimalStack orchestrates the full minimal deployment.
//
// This method deploys only what MPC actually needs to function, in order:
//  1. Tekton Pipelines (TaskRun execution engine)
//  2. MPC Operator (controller deployment from multi-platform-controller/deploy/operator)
//  3. OTP Server (one-time password service from multi-platform-controller/deploy/otp)
//
// Each component is deployed sequentially and verified before proceeding to the next.
// The entire deployment typically completes in 3-5 minutes.
func (m *MinimalDeployer) DeployMinimalStack(ctx context.Context) error {
	log.Println("Starting minimal MPC stack deployment...")
	log.Println("This deployment includes:")
	log.Println("  1. Tekton Pipelines (TaskRun engine)")
	log.Println("  2. MPC Operator (controller)")
	log.Println("  3. OTP Server (one-time passwords)")
	log.Println("")

	// Step 1: Deploy Tekton Pipelines
	if err := m.DeployTekton(ctx); err != nil {
		return fmt.Errorf("failed to deploy Tekton Pipelines: %w", err)
	}

	// Step 2: Deploy MPC Operator
	if err := m.DeployMPCOperator(ctx); err != nil {
		return fmt.Errorf("failed to deploy MPC Operator: %w", err)
	}

	// Step 3: Deploy OTP Server
	if err := m.DeployOTPServer(ctx); err != nil {
		return fmt.Errorf("failed to deploy OTP Server: %w", err)
	}

	log.Println("")
	log.Println("Minimal MPC stack deployed successfully!")
	log.Println("Total components: Tekton Pipelines + MPC Operator + OTP Server")
	return nil
}

// DeployTekton installs Tekton Pipelines from the official release.
//
// This method:
//  1. Applies the latest Tekton Pipelines release YAML from storage.googleapis.com
//  2. Waits for both the controller and webhook deployments to be ready
//
// The webhook wait is critical - the MPC operator creates Tekton Tasks which require
// webhook validation. Without waiting for the webhook, Task creation fails with
// "connection refused" errors.
func (m *MinimalDeployer) DeployTekton(ctx context.Context) error {
	log.Println("Deploying Tekton Pipelines...")
	log.Printf("Using Tekton release: %s", tektonReleaseURL)

	// Apply Tekton release YAML
	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", tektonReleaseURL)
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply Tekton release: %w", err)
	}

	log.Println("Tekton manifests applied, waiting for pods to be ready...")

	// Wait for Tekton controller to be ready
	if err := m.waitForTektonReady(ctx); err != nil {
		return fmt.Errorf("tekton deployment not ready: %w", err)
	}

	log.Println("✓ Tekton Pipelines deployed successfully")
	return nil
}

// waitForTektonReady waits for Tekton Pipelines to be ready.
//
// This method waits for BOTH the controller and webhook deployments to be ready:
//  1. tekton-pipelines-controller (manages TaskRun execution)
//  2. tekton-pipelines-webhook (validates TaskRun/Task resources)
//
// Both are required for MPC to function correctly. The webhook is especially critical
// as it validates Tasks created by the MPC operator.
func (m *MinimalDeployer) waitForTektonReady(ctx context.Context) error {
	log.Println("Waiting for Tekton controller deployment...")

	// Wait for tekton-pipelines-controller deployment
	controllerCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/tekton-pipelines-controller",
		"-n", tektonNamespace,
		"--timeout=3m")
	controllerCmd.Stdout = os.Stdout
	controllerCmd.Stderr = os.Stderr

	if err := controllerCmd.Run(); err != nil {
		return fmt.Errorf("timeout waiting for Tekton controller: %w", err)
	}

	log.Println("Tekton Pipelines controller is ready")

	// Wait for tekton-pipelines-webhook deployment
	// This is critical - MPC operator creates Tekton Tasks which need webhook validation
	log.Println("Waiting for Tekton webhook deployment...")

	webhookCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/tekton-pipelines-webhook",
		"-n", tektonNamespace,
		"--timeout=3m")
	webhookCmd.Stdout = os.Stdout
	webhookCmd.Stderr = os.Stderr

	if err := webhookCmd.Run(); err != nil {
		return fmt.Errorf("timeout waiting for Tekton webhook: %w", err)
	}

	log.Println("Tekton Pipelines webhook is ready")
	log.Println("Tekton Pipelines fully ready (controller + webhook)")
	return nil
}

// DeployMPCOperator applies MPC operator manifests from the MPC repository.
//
// This method applies all manifests in the multi-platform-controller/deploy/operator directory
// using `kubectl apply -Rf`. The operator manages the MPC controller deployment and creates
// the necessary Tekton Tasks for multi-platform builds.
func (m *MinimalDeployer) DeployMPCOperator(ctx context.Context) error {
	log.Println("Deploying MPC Operator...")

	// Get the MPC repository path from config
	mpcRepoPath := m.config.GetMpcRepoPath()

	// Construct path to deploy/operator directory
	operatorDir := filepath.Join(mpcRepoPath, "deploy", "operator")

	// Verify the directory exists
	if _, err := os.Stat(operatorDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("MPC operator deployment directory not found: %s", operatorDir)
		}
		return fmt.Errorf("cannot access MPC operator deployment directory: %w", err)
	}

	// Apply using kustomize (kubectl apply -k)
	log.Printf("Applying manifests from: %s", operatorDir)
	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "-k", operatorDir)
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply MPC operator manifests: %w", err)
	}

	log.Println("MPC Operator manifests applied")
	log.Println("Note: MPC operator will not be ready until Phase 5 builds and loads the image")

	// Don't wait for deployment to be ready here - it needs the image from Phase 5
	// Just verify the deployment was created
	if err := m.verifyMPCOperatorCreated(ctx); err != nil {
		return fmt.Errorf("MPC Operator deployment not created: %w", err)
	}

	log.Println("✓ MPC Operator manifests deployed successfully")
	return nil
}

// verifyMPCOperatorCreated verifies that the MPC operator deployment was created
// (but doesn't wait for it to be ready - that happens in Phase 5)
func (m *MinimalDeployer) verifyMPCOperatorCreated(ctx context.Context) error {
	log.Println("Verifying multi-platform-controller deployment was created...")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("timeout waiting for multi-platform-controller deployment to be created")
		case <-ticker.C:
			cmd := exec.CommandContext(ctx, "kubectl", "get", "deployment", mpcDeploymentName,
				"-n", mpcNamespace)
			if err := cmd.Run(); err == nil {
				log.Println("Multi-platform-controller deployment created successfully")
				return nil
			}
		}
	}
}

// DeployOTPServer applies OTP server manifests
func (m *MinimalDeployer) DeployOTPServer(ctx context.Context) error {
	log.Println("Deploying OTP Server...")

	// Get the MPC repository path from config
	mpcRepoPath := m.config.GetMpcRepoPath()

	// Construct path to deploy/otp directory
	otpDir := filepath.Join(mpcRepoPath, "deploy", "otp")

	// Verify the directory exists
	if _, err := os.Stat(otpDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("OTP server deployment directory not found: %s", otpDir)
		}
		return fmt.Errorf("cannot access OTP server deployment directory: %w", err)
	}

	// Apply using kustomize (kubectl apply -k)
	log.Printf("Applying manifests from: %s", otpDir)
	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "-k", otpDir)
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply OTP server manifests: %w", err)
	}

	log.Println("OTP Server manifests applied")
	log.Println("Note: OTP server will not be ready until Phase 5 builds and loads the image")

	// Don't wait for deployment to be ready here - it needs the image from Phase 5
	// Just verify the deployment was created (OTP is optional, so don't fail if it doesn't exist)
	if err := m.verifyOTPServerCreated(ctx); err != nil {
		log.Printf("WARNING: OTP server deployment not created: %v", err)
		log.Println("Continuing anyway (OTP is optional)")
		return nil
	}

	log.Println("✓ OTP Server manifests deployed successfully")
	return nil
}

// verifyOTPServerCreated verifies that the OTP server deployment was created
// (but doesn't wait for it to be ready - that happens in Phase 5)
func (m *MinimalDeployer) verifyOTPServerCreated(ctx context.Context) error {
	log.Println("Verifying OTP server deployment was created...")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("timeout waiting for OTP server deployment to be created")
		case <-ticker.C:
			cmd := exec.CommandContext(ctx, "kubectl", "get", "deployment", otpDeploymentName,
				"-n", mpcNamespace)
			if err := cmd.Run(); err == nil {
				log.Println("OTP server deployment created successfully")
				return nil
			}
		}
	}
}
