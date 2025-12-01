package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
)

func TestDeploy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deploy Manager Suite")
}

var _ = Describe("Deploy Manager", func() {
	var (
		manager *Manager
		cfg     *config.Config
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "deploy-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create a mock config
		cfg = &config.Config{
			TempDir: tempDir,
		}
		manager = NewManager(cfg)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
		_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
		_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		_ = os.Unsetenv("SSH_KEY_PATH")
	})

	Describe("generateMinimalHostConfig", func() {
		It("should generate a valid ConfigMap YAML", func() {
			outputPath := filepath.Join(tempDir, "host-config.yaml")
			err := manager.generateMinimalHostConfig(outputPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify the file was created
			_, err = os.Stat(outputPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify the content is valid YAML and a ConfigMap
			content, err := os.ReadFile(outputPath)
			Expect(err).NotTo(HaveOccurred())

			var cm struct {
				APIVersion string `yaml:"apiVersion"`
				Kind       string `yaml:"kind"`
				Metadata   struct {
					Name      string `yaml:"name"`
					Namespace string `yaml:"namespace"`
				} `yaml:"metadata"`
				Data map[string]string `yaml:"data"`
			}
			err = yaml.Unmarshal(content, &cm)
			Expect(err).NotTo(HaveOccurred())

			Expect(cm.APIVersion).To(Equal("v1"))
			Expect(cm.Kind).To(Equal("ConfigMap"))
			Expect(cm.Metadata.Name).To(Equal("host-config"))
			Expect(cm.Metadata.Namespace).To(Equal("multi-platform-controller"))
			Expect(cm.Data).To(HaveKey("dynamic-platforms"))
		})
	})

	Describe("Functions with kubectl", func() {
		var (
			originalPath    string
			mockKubectlPath string
		)

		BeforeEach(func() {
			originalPath = os.Getenv("PATH")
			mockKubectlPath = filepath.Join(tempDir, "kubectl")

			// Create a mock kubectl script that records its arguments and simulates resource state
			logFile := filepath.Join(tempDir, "kubectl_calls.log")
			stateFile := filepath.Join(tempDir, "kubectl_state.log")
			script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s

# Initialize state if not present
if [ ! -f %s ]; then
  echo "namespaces=" > %s
  echo "configmaps=" >> %s
  echo "secrets=" >> %s
fi

# Helper to check if a resource exists in state
resource_exists() {
  RESOURCE_TYPE=$1
  RESOURCE_NAME=$2
  grep -q "${RESOURCE_TYPE}=.*\b${RESOURCE_NAME}\b.*" %s
}

# Helper to add a resource to state
add_resource() {
  RESOURCE_TYPE=$1
  RESOURCE_NAME=$2
  sed -i "/^${RESOURCE_TYPE}=/s/$/${RESOURCE_NAME},/" %s
}

# Handle 'kubectl create namespace'
if [ "$1" = "create" ] && [ "$2" = "namespace" ]; then
  NAMESPACE_NAME=$3
  if ! resource_exists "namespaces" "${NAMESPACE_NAME}"; then
    add_resource "namespaces" "${NAMESPACE_NAME}"
    echo "namespace/${NAMESPACE_NAME} created"
    exit 0
  else
    echo "Error from server (AlreadyExists): namespaces \"${NAMESPACE_NAME}\" already exists"
    exit 1
  fi
fi

# Handle 'kubectl get namespace'
if [ "$1" = "get" ] && [ "$2" = "namespace" ]; then
  NAMESPACE_NAME=$3
  if resource_exists "namespaces" "${NAMESPACE_NAME}"; then
    echo "${NAMESPACE_NAME} Active 1h" # Simulate existing
    exit 0
  else
    exit 1 # Not found
  fi
fi

# Handle 'kubectl get configmap'
if [ "$1" = "get" ] && [ "$2" = "configmap" ]; then
  CONFIGMAP_NAME=$3
  if resource_exists "configmaps" "${CONFIGMAP_NAME}"; then
    echo "${CONFIGMAP_NAME} 1 1h" # Simulate existing
    exit 0
  else
    exit 1 # Not found
  fi
fi

# Handle 'kubectl delete configmap'
if [ "$1" = "delete" ] && [ "$2" = "configmap" ]; then
  CONFIGMAP_NAME=$3
  sed -i "s/,${CONFIGMAP_NAME},/,/g" %s
  echo "configmap \"${CONFIGMAP_NAME}\" deleted"
  exit 0
fi

# Handle 'kubectl apply -f' for configmap
if [ "$1" = "apply" ] && [ "$2" = "-f" ] && [[ "$3" == *host-config.yaml* ]]; then
  add_resource "configmaps" "host-config"
  echo "configmap/host-config configured"
  exit 0
fi

# Handle 'kubectl create secret'
if [ "$1" = "create" ] && [ "$2" = "secret" ]; then
  SECRET_NAME=$4
  if ! resource_exists "secrets" "${SECRET_NAME}"; then
    add_resource "secrets" "${SECRET_NAME}"
    echo "secret/${SECRET_NAME} created"
    exit 0
  else
    echo "Error from server (AlreadyExists): secrets \"${SECRET_NAME}\" already exists"
    exit 1
  fi
fi

# Handle 'kubectl get secret'
if [ "$1" = "get" ] && [ "$2" = "secret" ]; then
  SECRET_NAME=$3
  if resource_exists "secrets" "${SECRET_NAME}"; then
    echo "${SECRET_NAME} Opaque 1 1h" # Simulate existing
    exit 0
  else
    exit 1 # Not found
  fi
fi

# Default exit for other commands
exit 0
`, logFile, stateFile, stateFile, stateFile, stateFile, stateFile, stateFile, stateFile) //nolint:lll
			Expect(os.WriteFile(mockKubectlPath, []byte(script), 0755)).To(Succeed())
			_ = os.Setenv("PATH", tempDir+":"+originalPath)
		})

		AfterEach(func() {
			_ = os.Setenv("PATH", originalPath)
		})

		Describe("deployHostConfig", func() {
			It("should auto-generate host-config and apply it via kubectl", func() {
				err := manager.deployHostConfig(context.Background())
				Expect(err).NotTo(HaveOccurred())

				// Verify kubectl was called correctly
				calls, err := os.ReadFile(filepath.Join(tempDir, "kubectl_calls.log"))
				Expect(err).NotTo(HaveOccurred())

				Expect(string(calls)).To(ContainSubstring("create namespace multi-platform-controller"))
				Expect(string(calls)).To(ContainSubstring("get configmap host-config -n multi-platform-controller"))
				Expect(string(calls)).To(ContainSubstring("apply -f " + filepath.Join(tempDir, "host-config.yaml") + " -n multi-platform-controller"))
			})
		})

		Describe("ApplySecrets", func() {
			var sshKeyPath string

			BeforeEach(func() {
				sshKeyPath = filepath.Join(tempDir, "id_rsa")
				Expect(os.WriteFile(sshKeyPath, []byte("fake-ssh-key"), 0600)).To(Succeed())
				_ = os.Setenv("AWS_ACCESS_KEY_ID", "test-key-id")
				_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
				_ = os.Setenv("SSH_KEY_PATH", sshKeyPath)
			})

			AfterEach(func() {
				_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
				_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
				_ = os.Unsetenv("SSH_KEY_PATH")
			})

			It("should create aws and ssh secrets via kubectl", func() {
				err := manager.ApplySecrets(context.Background())
				Expect(err).NotTo(HaveOccurred())

				calls, err := os.ReadFile(filepath.Join(tempDir, "kubectl_calls.log"))
				Expect(err).NotTo(HaveOccurred())

				Expect(string(calls)).To(ContainSubstring("create secret generic aws-account --from-literal=access-key-id=test-key-id --from-literal=secret-access-key=test-secret-key --namespace multi-platform-controller"))
				Expect(string(calls)).To(ContainSubstring("create secret generic aws-ssh-key --from-file=id_rsa=" + sshKeyPath + " --namespace multi-platform-controller"))
				Expect(string(calls)).To(ContainSubstring("get secret aws-account -n multi-platform-controller"))
			})
		})
	})
})
