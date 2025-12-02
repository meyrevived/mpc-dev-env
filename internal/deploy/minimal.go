package deploy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/meyrevived/mpc-dev-env/internal/config"
)

const (
	tektonNamespace     = "tekton-pipelines"
	tektonReleaseURL    = "https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml"
	minimalStackTimeout = 10 * time.Minute

	// cert-manager constants for Kind cluster (vanilla Kubernetes)
	// In OpenShift, the service annotation automatically creates TLS certs,
	// but Kind needs cert-manager to provide this functionality
	certManagerNamespace  = "cert-manager"
	certManagerReleaseURL = "https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml"
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
//  2. cert-manager (TLS certificate management for OTP server)
//  3. MPC Operator (controller deployment from multi-platform-controller/deploy/operator)
//  4. OTP Server (one-time password service from multi-platform-controller/deploy/otp)
//
// cert-manager is required because the OTP server needs TLS certificates.
// In OpenShift, this is handled by the service.beta.openshift.io/serving-cert-secret-name annotation,
// but Kind (vanilla Kubernetes) needs cert-manager to provide this functionality.
//
// Each component is deployed sequentially and verified before proceeding to the next.
// The entire deployment typically completes in 3-5 minutes.
func (m *MinimalDeployer) DeployMinimalStack(ctx context.Context) error {
	log.Println("Starting minimal MPC stack deployment...")
	log.Println("This deployment includes:")
	log.Println("  1. Tekton Pipelines (TaskRun engine)")
	log.Println("  2. cert-manager (TLS certificates for OTP)")
	log.Println("  3. MPC Operator (controller)")
	log.Println("  4. OTP Server (one-time passwords)")
	log.Println("")

	// Step 1: Deploy Tekton Pipelines
	if err := m.DeployTekton(ctx); err != nil {
		return fmt.Errorf("failed to deploy Tekton Pipelines: %w", err)
	}

	// Step 2: Deploy cert-manager (required for OTP TLS certificates)
	if err := m.DeployCertManager(ctx); err != nil {
		return fmt.Errorf("failed to deploy cert-manager: %w", err)
	}

	// Step 3: Deploy MPC Operator
	if err := m.DeployMPCOperator(ctx); err != nil {
		return fmt.Errorf("failed to deploy MPC Operator: %w", err)
	}

	// Step 4: Deploy OTP Server (with TLS certificate)
	if err := m.DeployOTPServer(ctx); err != nil {
		return fmt.Errorf("failed to deploy OTP Server: %w", err)
	}

	log.Println("")
	log.Println("Minimal MPC stack deployed successfully!")
	log.Println("Total components: Tekton Pipelines + cert-manager + MPC Operator + OTP Server")
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

// DeployCertManager installs cert-manager for TLS certificate management.
//
// cert-manager is required in Kind clusters because:
//   - The OTP server requires TLS certificates mounted at /tls
//   - In OpenShift, the service annotation "service.beta.openshift.io/serving-cert-secret-name"
//     automatically creates TLS secrets via OpenShift's internal certificate signer
//   - Kind is vanilla Kubernetes and doesn't have this OpenShift feature
//   - cert-manager provides the same functionality via Certificate resources
//
// This method:
//  1. Applies the cert-manager release manifests
//  2. Waits for cert-manager deployments to be ready
//  3. Waits for the webhook to be ready (required before creating certificates)
func (m *MinimalDeployer) DeployCertManager(ctx context.Context) error {
	log.Println("Deploying cert-manager...")
	log.Printf("Using cert-manager release: %s", certManagerReleaseURL)

	// Apply cert-manager release YAML
	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", certManagerReleaseURL)
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply cert-manager release: %w", err)
	}

	log.Println("cert-manager manifests applied, waiting for pods to be ready...")

	// Wait for cert-manager to be ready
	if err := m.waitForCertManagerReady(ctx); err != nil {
		return fmt.Errorf("cert-manager deployment not ready: %w", err)
	}

	log.Println("✓ cert-manager deployed successfully")
	return nil
}

// waitForCertManagerReady waits for cert-manager components to be ready.
//
// This waits for all three cert-manager deployments:
//  1. cert-manager (main controller)
//  2. cert-manager-cainjector (CA injection for webhooks)
//  3. cert-manager-webhook (validates Certificate resources)
//
// The webhook is especially critical - creating Certificate resources before
// the webhook is ready will fail with validation errors.
func (m *MinimalDeployer) waitForCertManagerReady(ctx context.Context) error {
	deployments := []string{
		"cert-manager",
		"cert-manager-cainjector",
		"cert-manager-webhook",
	}

	for _, deployment := range deployments {
		log.Printf("Waiting for %s deployment...", deployment)
		cmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
			"deployment/"+deployment,
			"-n", certManagerNamespace,
			"--timeout=3m")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("timeout waiting for %s: %w", deployment, err)
		}
		log.Printf("%s is ready", deployment)
	}

	// Wait a bit for webhook to be fully operational
	// The deployment being ready doesn't mean the webhook endpoint is serving
	log.Println("Waiting for cert-manager webhook to be fully operational...")
	time.Sleep(10 * time.Second)

	log.Println("cert-manager fully ready")
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

// DeployOTPServer applies OTP server manifests and creates required TLS certificates.
//
// The OTP server requires TLS certificates mounted at /tls, provided by a secret named
// "otp-tls-secrets". In OpenShift, this secret is automatically created by the annotation
// "service.beta.openshift.io/serving-cert-secret-name" on the Service. In Kind (vanilla
// Kubernetes), we use cert-manager to create a self-signed certificate that generates
// this secret.
//
// This method:
//  1. Creates a self-signed ClusterIssuer (if not exists)
//  2. Creates a Certificate resource that generates the "otp-tls-secrets" secret
//  3. Waits for the certificate to be ready
//  4. Applies the OTP server deployment manifests
func (m *MinimalDeployer) DeployOTPServer(ctx context.Context) error {
	log.Println("Deploying OTP Server...")

	// Step 1: Create TLS certificate for OTP server
	// This must happen BEFORE applying OTP manifests because the deployment
	// mounts the secret that the certificate creates
	if err := m.createOTPTLSCertificate(ctx); err != nil {
		return fmt.Errorf("failed to create OTP TLS certificate: %w", err)
	}

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

	// Verify the deployment was created (don't wait for ready - that needs the image from Phase 5)
	if err := m.verifyOTPServerCreated(ctx); err != nil {
		return fmt.Errorf("OTP server deployment not created: %w", err)
	}

	log.Println("✓ OTP Server manifests deployed successfully")
	return nil
}

// createOTPTLSCertificate creates a self-signed TLS certificate for the OTP server.
//
// This creates:
//  1. A self-signed ClusterIssuer named "selfsigned-issuer"
//  2. A Certificate named "otp-tls-cert" that creates the "otp-tls-secrets" secret
//
// The certificate is issued for the OTP service DNS name within the cluster.
func (m *MinimalDeployer) createOTPTLSCertificate(ctx context.Context) error {
	log.Println("Creating TLS certificate for OTP server...")

	// First, ensure the MPC namespace exists (cert-manager needs the namespace to exist
	// before it can create the secret there)
	log.Println("Ensuring multi-platform-controller namespace exists...")
	nsCmd := exec.CommandContext(ctx, "kubectl", "create", "namespace", mpcNamespace)
	// Ignore error - namespace may already exist
	_ = nsCmd.Run()

	// Create a self-signed ClusterIssuer
	// Using ClusterIssuer instead of Issuer so it can be reused across namespaces if needed
	clusterIssuerYAML := `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
`

	log.Println("Creating self-signed ClusterIssuer...")
	issuerCmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	issuerCmd.Stdin = strings.NewReader(clusterIssuerYAML)
	issuerCmd.Stdout = os.Stdout
	issuerCmd.Stderr = os.Stderr

	if err := issuerCmd.Run(); err != nil {
		return fmt.Errorf("failed to create ClusterIssuer: %w", err)
	}

	// Create the Certificate resource that generates the otp-tls-secrets secret
	// The secret will contain tls.crt and tls.key which the OTP server mounts at /tls
	certificateYAML := fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: otp-tls-cert
  namespace: %s
spec:
  secretName: otp-tls-secrets
  duration: 8760h  # 1 year
  renewBefore: 720h  # 30 days
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
  commonName: multi-platform-otp-server
  dnsNames:
    - multi-platform-otp-server
    - multi-platform-otp-server.%s
    - multi-platform-otp-server.%s.svc
    - multi-platform-otp-server.%s.svc.cluster.local
  usages:
    - server auth
    - client auth
`, mpcNamespace, mpcNamespace, mpcNamespace, mpcNamespace)

	log.Println("Creating Certificate resource for OTP TLS...")
	certCmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	certCmd.Stdin = strings.NewReader(certificateYAML)
	certCmd.Stdout = os.Stdout
	certCmd.Stderr = os.Stderr

	if err := certCmd.Run(); err != nil {
		return fmt.Errorf("failed to create Certificate: %w", err)
	}

	// Wait for the certificate to be ready (i.e., the secret to be created)
	if err := m.waitForOTPCertificateReady(ctx); err != nil {
		return fmt.Errorf("certificate not ready: %w", err)
	}

	log.Println("✓ OTP TLS certificate created successfully")
	return nil
}

// waitForOTPCertificateReady waits for the OTP TLS certificate to be issued
// and the secret to be created.
func (m *MinimalDeployer) waitForOTPCertificateReady(ctx context.Context) error {
	log.Println("Waiting for OTP TLS certificate to be ready...")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(2 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("timeout waiting for OTP TLS certificate to be ready")
		case <-ticker.C:
			// Check if the secret exists (this means the certificate was issued)
			cmd := exec.CommandContext(ctx, "kubectl", "get", "secret", "otp-tls-secrets",
				"-n", mpcNamespace)
			if err := cmd.Run(); err == nil {
				log.Println("OTP TLS secret created successfully")
				return nil
			}
		}
	}
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
