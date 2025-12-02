// Package deploy provides deployment management for the MPC development environment.
//
// This package handles deploying and configuring the Multi-Platform Controller and related
// components to the Kind cluster:
//   - MPC deployment configuration and image patching
//   - OTP (One-Time Password) server deployment
//   - Host configuration (host-config.yaml) for build platforms
//   - AWS secrets deployment for cloud-based builds
//   - Minimal MPC stack (Tekton Pipelines + MPC Operator + OTP Server)
//
// The Manager coordinates kubectl commands to apply manifests, wait for deployments,
// and verify that the MPC is running with the correct locally-built images.
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
	// MPC deployment constants derived from the bash script
	mpcNamespace        = "multi-platform-controller"
	mpcDeploymentName   = "multi-platform-controller"
	otpDeploymentName   = "multi-platform-otp-server"
	hostConfigName      = "host-config"
	deployTimeout       = 10 * time.Minute
	deploymentWaitRetry = 60 // 2 minutes with 2 second intervals
)

// Manager handles deployment operations for the multi-platform-controller.
//
// It uses kubectl commands to interact with the Kubernetes cluster and maintains
// configuration for repository paths and deployment settings.
type Manager struct {
	config *config.Config
}

// NewManager creates a new deployment manager instance.
//
// The manager is configured with paths to the MPC repository and other settings
// needed for deployments.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		config: cfg,
	}
}

// DeployMPC deploys the multi-platform-controller to the Kind cluster.
//
// This function orchestrates the complete MPC deployment workflow:
//  1. Deploys the host-config ConfigMap (auto-generates if not exists)
//  2. Applies MPC deployment manifests to the cluster
//  3. Waits for the MPC deployment to be ready
//  4. Patches the deployment to use locally-built custom images
//  5. Restarts deployments to apply the image changes
//  6. Verifies that the correct images are running
//
// This is the primary entry point for MPC deployments, called by API handlers.
func DeployMPC(ctx context.Context, cfg *config.Config) error {
	manager := NewManager(cfg)
	return manager.Deploy(ctx)
}

// Deploy executes the full deployment workflow.
//
// This is the internal implementation of the deployment sequence, broken down into
// distinct steps for clarity and error handling. Each step is logged and errors are
// wrapped with context about which step failed.
func (m *Manager) Deploy(ctx context.Context) error {
	log.Println("Starting MPC deployment...")

	// Step 1: Deploy host-config ConfigMap
	if err := m.deployHostConfig(ctx); err != nil {
		return fmt.Errorf("failed to deploy host-config: %w", err)
	}

	// Step 2: Apply MPC deployment manifests
	if err := m.applyMPCManifests(ctx); err != nil {
		return fmt.Errorf("failed to apply MPC manifests: %w", err)
	}

	// Step 3: Wait for MPC deployment to be ready
	if err := m.waitForMPCDeployment(ctx); err != nil {
		return fmt.Errorf("MPC deployment not ready: %w", err)
	}

	// Step 4: Wait for OTP deployment to be ready
	if err := m.waitForOTPDeployment(ctx); err != nil {
		return fmt.Errorf("OTP deployment not ready: %w", err)
	}

	// Step 5: Patch MPC deployment with custom images
	if err := m.patchMPCDeployment(ctx); err != nil {
		return fmt.Errorf("failed to patch MPC deployment: %w", err)
	}

	// Step 6: Patch OTP deployment with custom images
	if err := m.patchOTPDeployment(ctx); err != nil {
		return fmt.Errorf("failed to patch OTP deployment: %w", err)
	}

	// Step 7: Restart deployments to apply changes
	if err := m.restartDeployments(ctx); err != nil {
		return fmt.Errorf("failed to restart deployments: %w", err)
	}

	// Step 8: Verify deployment images
	if err := m.verifyDeploymentImages(ctx); err != nil {
		return fmt.Errorf("image verification failed: %w", err)
	}

	log.Println("MPC deployment completed successfully!")
	return nil
}

// generateMinimalHostConfig generates a minimal host-config.yaml for local development.
//
// This creates a ConfigMap with:
//   - 4 AWS dynamic platforms (linux/arm64, linux/amd64, linux-mlarge/arm64, linux-mlarge/amd64)
//   - 3 local platforms (linux/x86_64, local, localhost)
//   - 2 static hosts for testing (S390X and PPC64LE pointing to localhost)
//
// The generated config is written to the specified outputPath (typically temp/host-config.yaml).
// This auto-generation allows developers to start testing immediately without manually creating
// the configuration file.
func (m *Manager) generateMinimalHostConfig(outputPath string) error {
	// Minimal host-config with 4 AWS platforms, 1 s390x, and 1 ppc64le host
	minimalConfig := `apiVersion: v1
kind: ConfigMap
metadata:
  labels:
    build.appstudio.redhat.com/multi-platform-config: hosts
  name: host-config
  namespace: multi-platform-controller
data:
  local-platforms: "\
    linux/x86_64,\
    local,\
    localhost,\
    "
  dynamic-platforms: "\
    linux/arm64,\
    linux/amd64,\
    linux-mlarge/arm64,\
    linux-mlarge/amd64\
    "
  instance-tag: "mpc-dev-env"

  # ARM64 - Basic (m6g.large)
  dynamic.linux-arm64.type: "aws"
  dynamic.linux-arm64.region: "us-east-1"
  dynamic.linux-arm64.ami: "ami-03d8261904652a19c"
  dynamic.linux-arm64.instance-type: "m6g.large"
  dynamic.linux-arm64.instance-tag: "dev-arm64"
  dynamic.linux-arm64.key-name: "mpc-dev-key"
  dynamic.linux-arm64.aws-secret: "aws-account"
  dynamic.linux-arm64.ssh-secret: "aws-ssh-key"
  dynamic.linux-arm64.security-group-id: "sg-default"
  dynamic.linux-arm64.max-instances: "10"
  dynamic.linux-arm64.subnet-id: "subnet-default"
  dynamic.linux-arm64.allocation-timeout: "600"

  # ARM64 - Medium (m6g.large)
  dynamic.linux-mlarge-arm64.type: "aws"
  dynamic.linux-mlarge-arm64.region: "us-east-1"
  dynamic.linux-mlarge-arm64.ami: "ami-03d8261904652a19c"
  dynamic.linux-mlarge-arm64.instance-type: "m6g.large"
  dynamic.linux-mlarge-arm64.instance-tag: "dev-arm64-mlarge"
  dynamic.linux-mlarge-arm64.key-name: "mpc-dev-key"
  dynamic.linux-mlarge-arm64.aws-secret: "aws-account"
  dynamic.linux-mlarge-arm64.ssh-secret: "aws-ssh-key"
  dynamic.linux-mlarge-arm64.security-group-id: "sg-default"
  dynamic.linux-mlarge-arm64.max-instances: "10"
  dynamic.linux-mlarge-arm64.subnet-id: "subnet-default"
  dynamic.linux-mlarge-arm64.allocation-timeout: "600"

  # AMD64 - Basic (m6a.large)
  dynamic.linux-amd64.type: "aws"
  dynamic.linux-amd64.region: "us-east-1"
  dynamic.linux-amd64.ami: "ami-0c02fb55b1a47c3c8"
  dynamic.linux-amd64.instance-type: "m6a.large"
  dynamic.linux-amd64.instance-tag: "dev-amd64"
  dynamic.linux-amd64.key-name: "mpc-dev-key"
  dynamic.linux-amd64.aws-secret: "aws-account"
  dynamic.linux-amd64.ssh-secret: "aws-ssh-key"
  dynamic.linux-amd64.security-group-id: "sg-default"
  dynamic.linux-amd64.max-instances: "10"
  dynamic.linux-amd64.subnet-id: "subnet-default"
  dynamic.linux-amd64.allocation-timeout: "600"

  # AMD64 - Medium (m6a.large)
  dynamic.linux-mlarge-amd64.type: "aws"
  dynamic.linux-mlarge-amd64.region: "us-east-1"
  dynamic.linux-mlarge-amd64.ami: "ami-0c02fb55b1a47c3c8"
  dynamic.linux-mlarge-amd64.instance-type: "m6a.large"
  dynamic.linux-mlarge-amd64.instance-tag: "dev-amd64-mlarge"
  dynamic.linux-mlarge-amd64.key-name: "mpc-dev-key"
  dynamic.linux-mlarge-amd64.aws-secret: "aws-account"
  dynamic.linux-mlarge-amd64.ssh-secret: "aws-ssh-key"
  dynamic.linux-mlarge-amd64.security-group-id: "sg-default"
  dynamic.linux-mlarge-amd64.max-instances: "10"
  dynamic.linux-mlarge-amd64.subnet-id: "subnet-default"
  dynamic.linux-mlarge-amd64.allocation-timeout: "600"

  # S390X - Static host for development
  host.s390x-dev.address: "127.0.0.1"
  host.s390x-dev.platform: "linux/s390x"
  host.s390x-dev.user: "root"
  host.s390x-dev.secret: "ibm-s390x-ssh-key"
  host.s390x-dev.concurrency: "4"

  # PPC64LE - Static host for development
  host.ppc64le-dev.address: "127.0.0.1"
  host.ppc64le-dev.platform: "linux/ppc64le"
  host.ppc64le-dev.user: "root"
  host.ppc64le-dev.secret: "ibm-ppc64le-ssh-key"
  host.ppc64le-dev.concurrency: "4"
`

	// Ensure the temp directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Write the minimal config to file
	if err := os.WriteFile(outputPath, []byte(minimalConfig), 0644); err != nil {
		return fmt.Errorf("failed to write host-config file: %w", err)
	}

	log.Printf("Generated minimal host-config.yaml at: %s", outputPath)
	return nil
}

// deployHostConfig deploys the host-config ConfigMap
func (m *Manager) deployHostConfig(ctx context.Context) error {
	log.Println("Deploying host-config ConfigMap...")

	// Ensure namespace exists first
	if err := m.ensureNamespace(ctx); err != nil {
		return fmt.Errorf("failed to ensure namespace exists: %w", err)
	}

	// Construct path to host-config file
	// Based on the bash script, this should be in temp/host-config.yaml
	hostConfigPath := filepath.Join(m.config.GetTempDir(), "host-config.yaml")

	// Check if the file exists, if not generate a minimal one
	if _, err := os.Stat(hostConfigPath); err != nil {
		if os.IsNotExist(err) {
			log.Println("host-config.yaml not found, generating minimal configuration for local development...")
			if err := m.generateMinimalHostConfig(hostConfigPath); err != nil {
				return fmt.Errorf("failed to generate host-config: %w", err)
			}
			log.Println("Minimal host-config.yaml generated successfully")
		} else {
			return fmt.Errorf("cannot access host-config file: %w", err)
		}
	}

	// Check if ConfigMap already exists
	checkCmd := exec.CommandContext(ctx, "kubectl", "get", "configmap", hostConfigName,
		"-n", mpcNamespace)
	if err := checkCmd.Run(); err == nil {
		// ConfigMap exists, delete it first
		log.Println("ConfigMap 'host-config' already exists, replacing...")
		deleteCmd := exec.CommandContext(ctx, "kubectl", "delete", "configmap", hostConfigName,
			"-n", mpcNamespace)
		if err := deleteCmd.Run(); err != nil {
			log.Printf("WARNING: Failed to delete existing ConfigMap: %v", err)
		}
	}

	// Apply the ConfigMap
	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", hostConfigPath,
		"-n", mpcNamespace)
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply host-config ConfigMap: %w", err)
	}

	log.Println("Host-config ConfigMap deployed successfully")
	return nil
}

// waitForMPCDeployment waits for the MPC deployment to be created by Argo CD
func (m *Manager) waitForMPCDeployment(ctx context.Context) error {
	log.Println("Waiting for multi-platform-controller deployment...")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(2 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("timeout waiting for multi-platform-controller deployment")
		case <-ticker.C:
			cmd := exec.CommandContext(ctx, "kubectl", "get", "deployment", mpcDeploymentName,
				"-n", mpcNamespace)
			if err := cmd.Run(); err == nil {
				log.Println("Multi-platform-controller deployment found")
				return nil
			}
		}
	}
}

// waitForOTPDeployment waits for the OTP deployment to be created.
//
// The OTP server is a required component for MPC to function properly.
// It provides one-time passwords for secure access to build hosts.
func (m *Manager) waitForOTPDeployment(ctx context.Context) error {
	log.Println("Waiting for OTP server deployment...")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(2 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("timeout waiting for OTP server deployment")
		case <-ticker.C:
			cmd := exec.CommandContext(ctx, "kubectl", "get", "deployment", otpDeploymentName,
				"-n", mpcNamespace)
			if err := cmd.Run(); err == nil {
				log.Println("OTP server deployment found")
				return nil
			}
		}
	}
}

// patchMPCDeployment patches the controller deployment to use custom images
func (m *Manager) patchMPCDeployment(ctx context.Context) error {
	log.Println("Patching multi-platform-controller deployment...")

	// Use the locally built image that was loaded into Kind cluster
	// The image is built as "multi-platform-controller:latest" and Podman tags it as "localhost/multi-platform-controller:latest"
	controllerImage := "localhost/multi-platform-controller:latest"
	log.Printf("Patching with image: %s", controllerImage)

	// Create JSON patch to update image and imagePullPolicy
	// Use "Never" to ensure Kubernetes uses the locally loaded image instead of trying to pull
	patchJSON := fmt.Sprintf(`[
  {
    "op": "replace",
    "path": "/spec/template/spec/containers/0/image",
    "value": "%s"
  },
  {
    "op": "replace",
    "path": "/spec/template/spec/containers/0/imagePullPolicy",
    "value": "Never"
  }
]`, controllerImage)

	// Apply the patch
	cmd := exec.CommandContext(ctx, "kubectl", "patch", "deployment", mpcDeploymentName,
		"-n", mpcNamespace,
		"--type=json",
		"--patch", patchJSON)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to patch controller deployment: %w", err)
	}

	log.Println("Controller deployment patched successfully")
	return nil
}

// patchOTPDeployment patches the OTP server deployment to use custom images.
//
// This patches the OTP deployment to use the locally built image with imagePullPolicy: Never
// so Kubernetes uses the image that was loaded into the Kind cluster.
func (m *Manager) patchOTPDeployment(ctx context.Context) error {
	log.Println("Patching OTP server deployment...")

	// Use the locally built image that was loaded into Kind cluster
	// The image is built as "multi-platform-otp:latest" and Podman tags it as "localhost/multi-platform-otp:latest"
	otpImage := "localhost/multi-platform-otp:latest"
	log.Printf("Patching OTP with image: %s", otpImage)

	// Create JSON patch to update image and imagePullPolicy
	// Use "Never" to ensure Kubernetes uses the locally loaded image instead of trying to pull
	patchJSON := fmt.Sprintf(`[
  {
    "op": "replace",
    "path": "/spec/template/spec/containers/0/image",
    "value": "%s"
  },
  {
    "op": "replace",
    "path": "/spec/template/spec/containers/0/imagePullPolicy",
    "value": "Never"
  }
]`, otpImage)

	// Apply the patch
	cmd := exec.CommandContext(ctx, "kubectl", "patch", "deployment", otpDeploymentName,
		"-n", mpcNamespace,
		"--type=json",
		"--patch", patchJSON)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to patch OTP deployment: %w", err)
	}

	log.Println("OTP server deployment patched successfully")
	return nil
}

// restartDeployments restarts the MPC and OTP deployments to apply changes.
//
// Both deployments are restarted and we wait for both to be ready before returning.
func (m *Manager) restartDeployments(ctx context.Context) error {
	log.Println("Restarting deployments to apply changes...")

	// Restart controller deployment
	restartCmd := exec.CommandContext(ctx, "kubectl", "rollout", "restart",
		"deployment/"+mpcDeploymentName,
		"-n", mpcNamespace)
	restartCmd.Stdout = os.Stdout
	restartCmd.Stderr = os.Stderr

	if err := restartCmd.Run(); err != nil {
		return fmt.Errorf("failed to restart controller deployment: %w", err)
	}

	log.Println("Controller deployment restarted")

	// Restart OTP deployment
	otpRestartCmd := exec.CommandContext(ctx, "kubectl", "rollout", "restart",
		"deployment/"+otpDeploymentName,
		"-n", mpcNamespace)
	otpRestartCmd.Stdout = os.Stdout
	otpRestartCmd.Stderr = os.Stderr

	if err := otpRestartCmd.Run(); err != nil {
		return fmt.Errorf("failed to restart OTP deployment: %w", err)
	}

	log.Println("OTP server deployment restarted")

	// Wait for controller to be ready
	log.Println("Waiting for controller to be ready...")
	waitCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/"+mpcDeploymentName,
		"-n", mpcNamespace,
		"--timeout=5m")
	waitCmd.Stdout = os.Stdout
	waitCmd.Stderr = os.Stderr

	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("failed to wait for controller rollout: %w", err)
	}

	// Wait for OTP to be ready
	log.Println("Waiting for OTP server to be ready...")
	otpWaitCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/"+otpDeploymentName,
		"-n", mpcNamespace,
		"--timeout=5m")
	otpWaitCmd.Stdout = os.Stdout
	otpWaitCmd.Stderr = os.Stderr

	if err := otpWaitCmd.Run(); err != nil {
		return fmt.Errorf("failed to wait for OTP rollout: %w", err)
	}

	log.Println("Deployments restarted successfully")
	return nil
}

// verifyDeploymentImages verifies that deployments are using the correct images
func (m *Manager) verifyDeploymentImages(ctx context.Context) error {
	log.Println("Verifying deployment images...")

	// The expected image is what we built and patched with
	// Builder creates "multi-platform-controller:latest" and Podman tags it as "localhost/multi-platform-controller:latest"
	expectedControllerImage := "localhost/multi-platform-controller:latest"

	// Get actual controller image from deployment
	cmd := exec.CommandContext(ctx, "kubectl", "get", "deployment", mpcDeploymentName,
		"-n", mpcNamespace,
		"-o", "jsonpath={.spec.template.spec.containers[0].image}")

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get controller image: %w", err)
	}

	actualImage := string(output)
	if actualImage != expectedControllerImage {
		return fmt.Errorf("controller using wrong image: %s (expected: %s)", actualImage, expectedControllerImage)
	}

	log.Printf("✓ Controller using correct image: %s", actualImage)
	return nil
}

// ApplySecrets applies AWS secrets to the Kubernetes cluster
// This creates the necessary secrets for the multi-platform-controller to access AWS resources
func (m *Manager) ApplySecrets(ctx context.Context) error {
	log.Println("Applying AWS secrets to Kubernetes cluster...")

	// Ensure namespace exists
	if err := m.ensureNamespace(ctx); err != nil {
		return fmt.Errorf("failed to ensure namespace exists: %w", err)
	}

	// Step 1: Create aws-account secret
	if err := m.createAWSAccountSecret(ctx); err != nil {
		return fmt.Errorf("failed to create aws-account secret: %w", err)
	}

	// Step 2: Create aws-ssh-key secret
	if err := m.createAWSSSHKeySecret(ctx); err != nil {
		return fmt.Errorf("failed to create aws-ssh-key secret: %w", err)
	}

	// Step 3: Verify secrets exist
	if err := m.verifySecrets(ctx); err != nil {
		return fmt.Errorf("secret verification failed: %w", err)
	}

	log.Println("AWS secrets applied successfully!")
	return nil
}

// ensureNamespace creates the MPC namespace if it doesn't exist
func (m *Manager) ensureNamespace(ctx context.Context) error {
	log.Printf("Ensuring namespace exists: %s...", mpcNamespace)

	// Check if namespace exists
	checkCmd := exec.CommandContext(ctx, "kubectl", "get", "namespace", mpcNamespace)
	if err := checkCmd.Run(); err == nil {
		log.Printf("Namespace %s already exists", mpcNamespace)
		return nil
	}

	// Create namespace
	createCmd := exec.CommandContext(ctx, "kubectl", "create", "namespace", mpcNamespace)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	log.Printf("Namespace %s created successfully", mpcNamespace)
	return nil
}

// applyMPCManifests applies the MPC deployment manifests from the multi-platform-controller repository
func (m *Manager) applyMPCManifests(ctx context.Context) error {
	log.Println("Applying MPC deployment manifests...")

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
		return fmt.Errorf("failed to apply MPC manifests: %w", err)
	}

	log.Println("MPC manifests applied successfully")
	return nil
}

// createAWSAccountSecret creates the aws-account Kubernetes secret
func (m *Manager) createAWSAccountSecret(ctx context.Context) error {
	log.Println("Creating aws-account secret...")

	// Get AWS credentials from environment
	awsAccessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	awsSecretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if awsAccessKeyID == "" || awsSecretAccessKey == "" {
		return errors.New("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables must be set")
	}

	// Check if secret already exists
	checkCmd := exec.CommandContext(ctx, "kubectl", "get", "secret", "aws-account", "-n", mpcNamespace)
	if err := checkCmd.Run(); err == nil {
		log.Println("Secret 'aws-account' already exists, replacing...")
		deleteCmd := exec.CommandContext(ctx, "kubectl", "delete", "secret", "aws-account", "-n", mpcNamespace)
		if err := deleteCmd.Run(); err != nil {
			log.Printf("WARNING: Failed to delete existing secret: %v", err)
		}
	}

	// Create the secret
	createCmd := exec.CommandContext(ctx, "kubectl", "create", "secret", "generic", "aws-account",
		"--from-literal=access-key-id="+awsAccessKeyID,
		"--from-literal=secret-access-key="+awsSecretAccessKey,
		"--namespace", mpcNamespace)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create aws-account secret: %w", err)
	}

	log.Println("aws-account secret created successfully")
	return nil
}

// createAWSSSHKeySecret creates the aws-ssh-key Kubernetes secret
func (m *Manager) createAWSSSHKeySecret(ctx context.Context) error {
	log.Println("Creating aws-ssh-key secret...")

	// Get SSH key path from environment
	sshKeyPath := os.Getenv("SSH_KEY_PATH")
	if sshKeyPath == "" {
		return errors.New("SSH_KEY_PATH environment variable must be set")
	}

	// Validate that the SSH key file exists
	if _, err := os.Stat(sshKeyPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("SSH key file not found: %s", sshKeyPath)
		}
		return fmt.Errorf("cannot access SSH key file: %w", err)
	}

	// Check if secret already exists
	checkCmd := exec.CommandContext(ctx, "kubectl", "get", "secret", "aws-ssh-key", "-n", mpcNamespace)
	if err := checkCmd.Run(); err == nil {
		log.Println("Secret 'aws-ssh-key' already exists, replacing...")
		deleteCmd := exec.CommandContext(ctx, "kubectl", "delete", "secret", "aws-ssh-key", "-n", mpcNamespace)
		if err := deleteCmd.Run(); err != nil {
			log.Printf("WARNING: Failed to delete existing secret: %v", err)
		}
	}

	// Create the secret
	createCmd := exec.CommandContext(ctx, "kubectl", "create", "secret", "generic", "aws-ssh-key",
		"--from-file=id_rsa="+sshKeyPath,
		"--namespace", mpcNamespace)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create aws-ssh-key secret: %w", err)
	}

	log.Println("aws-ssh-key secret created successfully")
	return nil
}

// verifySecrets verifies that all required secrets exist
func (m *Manager) verifySecrets(ctx context.Context) error {
	log.Println("Verifying secrets...")

	requiredSecrets := []string{"aws-account", "aws-ssh-key"}

	for _, secretName := range requiredSecrets {
		cmd := exec.CommandContext(ctx, "kubectl", "get", "secret", secretName, "-n", mpcNamespace)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("secret '%s' not found in namespace %s", secretName, mpcNamespace)
		}
		log.Printf("✓ Secret exists: %s", secretName)
	}

	log.Println("All required secrets exist")
	return nil
}

// ApplyKonflux deploys Konflux to the Kind cluster
// This runs the necessary scripts from the konflux-ci repository
func (m *Manager) ApplyKonflux(ctx context.Context) error {
	log.Println("Deploying Konflux to Kind cluster...")

	// Get the konflux-ci directory path (sibling to mpc_dev_env)
	konfluxCIDir := filepath.Join(filepath.Dir(m.config.GetMpcDevEnvPath()), "konflux-ci")

	// Verify konflux-ci directory exists
	if _, err := os.Stat(konfluxCIDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("konflux-ci directory not found: %s (please clone konflux-ci repository)", konfluxCIDir)
		}
		return fmt.Errorf("cannot access konflux-ci directory: %w", err)
	}

	// Step 1: Deploy dependencies (Tekton, Argo CD, etc.)
	log.Println("Step 1/3: Deploying Konflux dependencies...")
	if err := m.runKonfluxScript(ctx, konfluxCIDir, "deploy-deps.sh"); err != nil {
		return fmt.Errorf("failed to deploy Konflux dependencies: %w", err)
	}

	// Step 2: Deploy Konflux components
	log.Println("Step 2/3: Deploying Konflux components...")
	if err := m.runKonfluxScript(ctx, konfluxCIDir, "deploy-konflux.sh"); err != nil {
		return fmt.Errorf("failed to deploy Konflux components: %w", err)
	}

	// Step 3: Deploy test resources
	log.Println("Step 3/3: Deploying Konflux test resources...")
	if err := m.runKonfluxScript(ctx, konfluxCIDir, "deploy-test-resources.sh"); err != nil {
		return fmt.Errorf("failed to deploy Konflux test resources: %w", err)
	}

	log.Println("Konflux deployed successfully!")
	log.Println("")
	log.Println("Konflux UI will be available at: https://localhost:9443")
	log.Println("Default login:")
	log.Println("  Username: user2@konflux.dev")
	log.Println("  Password: password")
	log.Println("")

	return nil
}

// runKonfluxScript executes a Konflux deployment script in the konflux-ci directory
func (m *Manager) runKonfluxScript(ctx context.Context, konfluxCIDir, scriptName string) error {
	scriptPath := filepath.Join(konfluxCIDir, scriptName)

	// Verify script exists
	if _, err := os.Stat(scriptPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("script not found: %s", scriptPath)
		}
		return fmt.Errorf("cannot access script: %w", err)
	}

	log.Printf("Running %s...", scriptName)

	// Execute the script
	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Dir = konfluxCIDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("script %s failed: %w", scriptName, err)
	}

	log.Printf("Script %s completed successfully", scriptName)
	return nil
}
