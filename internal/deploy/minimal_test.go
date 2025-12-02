package deploy

import (
	"context"
	"os"
	"path/filepath"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Note: Test suite entry point is in manager_test.go

var _ = Describe("Minimal Deployer", func() {
	var (
		deployer        *MinimalDeployer
		cfg             *config.Config
		tempDir         string
		originalPath    string
		mockKubectlPath string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "minimal-deploy-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Mock config and directory structure
		mpcRepoPath := filepath.Join(tempDir, "multi-platform-controller")
		Expect(os.MkdirAll(filepath.Join(mpcRepoPath, "deploy", "operator"), 0755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(mpcRepoPath, "deploy", "otp"), 0755)).To(Succeed())
		cfg = &config.Config{
			MpcRepoPath: mpcRepoPath,
		}
		deployer = NewMinimalDeployer(cfg)

		// Mock kubectl
		originalPath = os.Getenv("PATH")
		mockKubectlPath = filepath.Join(tempDir, "kubectl")
		script := `#!/bin/sh
echo "$@" >> ` + filepath.Join(tempDir, "kubectl_calls.log") + `
exit 0
`
		Expect(os.WriteFile(mockKubectlPath, []byte(script), 0755)).To(Succeed())
		_ = os.Setenv("PATH", tempDir+":"+originalPath)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
		_ = os.Setenv("PATH", originalPath)
	})

	Describe("DeployTekton", func() {
		It("should apply tekton release and wait for rollout", func() {
			err := deployer.DeployTekton(context.Background())
			Expect(err).NotTo(HaveOccurred())

			calls, err := os.ReadFile(filepath.Join(tempDir, "kubectl_calls.log"))
			Expect(err).NotTo(HaveOccurred())

			Expect(string(calls)).To(ContainSubstring("apply -f " + tektonReleaseURL))
			Expect(string(calls)).To(ContainSubstring("rollout status deployment/tekton-pipelines-controller -n tekton-pipelines"))
			Expect(string(calls)).To(ContainSubstring("rollout status deployment/tekton-pipelines-webhook -n tekton-pipelines"))
		})
	})

	Describe("DeployMPCOperator", func() {
		It("should apply manifests from the correct kustomize directory", func() {
			err := deployer.DeployMPCOperator(context.Background())
			Expect(err).NotTo(HaveOccurred())

			calls, err := os.ReadFile(filepath.Join(tempDir, "kubectl_calls.log"))
			Expect(err).NotTo(HaveOccurred())

			operatorDir := filepath.Join(cfg.MpcRepoPath, "deploy", "operator")
			Expect(string(calls)).To(ContainSubstring("apply -k " + operatorDir))
			Expect(string(calls)).To(ContainSubstring("get deployment multi-platform-controller -n multi-platform-controller"))
		})
	})

	Describe("DeployOTPServer", func() {
		It("should apply manifests from the correct kustomize directory", func() {
			err := deployer.DeployOTPServer(context.Background())
			Expect(err).NotTo(HaveOccurred())

			calls, err := os.ReadFile(filepath.Join(tempDir, "kubectl_calls.log"))
			Expect(err).NotTo(HaveOccurred())

			otpDir := filepath.Join(cfg.MpcRepoPath, "deploy", "otp")
			Expect(string(calls)).To(ContainSubstring("apply -k " + otpDir))
			Expect(string(calls)).To(ContainSubstring("get deployment multi-platform-otp-server -n multi-platform-controller"))
		})
	})
})
