package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBuilder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Builder Suite")
}

var _ = Describe("Builder", func() {
	var (
		builder *Builder
		cfg     *config.Config
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "builder-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create a mock config
		cfg = &config.Config{
			MpcRepoPath: tempDir,
		}
		builder = NewBuilder(cfg)
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
		_ = os.Unsetenv("DOCKER_CLI")
	})

	Describe("detectContainerRuntime", func() {
		var originalPath string

		BeforeEach(func() {
			originalPath = os.Getenv("PATH")
		})

		AfterEach(func() {
			_ = os.Setenv("PATH", originalPath)
		})

		It("should prefer DOCKER_CLI environment variable if set and valid", func() {
			// Create a fake docker executable
			fakeDockerPath := filepath.Join(tempDir, "fake-docker")
			Expect(os.WriteFile(fakeDockerPath, []byte("#!/bin/sh\necho 'Docker version 20.10.7'"), 0755)).To(Succeed())
			_ = os.Setenv("DOCKER_CLI", fakeDockerPath)

			runtime, err := builder.detectContainerRuntime()
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime).To(Equal(fakeDockerPath))
		})

		It("should detect podman if available", func() {
			// This test relies on podman being in the PATH
			if _, err := exec.LookPath("podman"); err != nil {
				Skip("podman not found in PATH")
			}
			runtime, err := builder.detectContainerRuntime()
			Expect(err).NotTo(HaveOccurred())
			Expect(runtime).To(Equal("podman"))
		})

		It("should return an error if no runtime is found", func() {
			_ = os.Setenv("PATH", "/non-existent-path")
			_, err := builder.detectContainerRuntime()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("build", func() {
		BeforeEach(func() {
			// Create a dummy Dockerfile
			dockerfilePath := filepath.Join(tempDir, "Dockerfile")
			dockerfileContent := "FROM scratch\nCMD echo 'hello'"
			Expect(os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)).To(Succeed())

			// Create a mock kind executable
			mockKindPath := filepath.Join(tempDir, "kind")
			mockKindScript := "#!/bin/sh\necho 'Mock kind load image-archive'\nexit 0"
			Expect(os.WriteFile(mockKindPath, []byte(mockKindScript), 0755)).To(Succeed())

			// Prepend the temp dir to PATH to ensure our mock is used
			_ = os.Setenv("PATH", tempDir+":"+os.Getenv("PATH"))
		})

		It("should execute the build and load-to-kind process", func() {
			// This test is more of an integration test for the build function's orchestration.
			// It relies on a real container runtime being present.
			runtime, err := builder.detectContainerRuntime()
			if err != nil {
				Skip("No container runtime found, skipping build test.")
			}
			if runtime == "kind" { // Our mock 'kind' could be detected as the runtime
				Skip("Skipping because mock 'kind' is detected as runtime.")
			}

			err = builder.buildImage(context.Background(), "Dockerfile", "multi-platform-controller:latest")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
