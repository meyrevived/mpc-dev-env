package prereq

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPrereq(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prerequisite Checker Suite")
}

var _ = Describe("Prerequisite Checker", func() {
	Describe("extractVersion", func() {
		It("should extract version from standard go version output", func() {
			output := "go version go1.24.0 linux/amd64"
			regex := `go(\d+\.\d+(?:\.\d+)?)`
			version := extractVersion(output, regex)
			Expect(version).To(Equal("1.24.0"))
		})

		It("should extract version from kind version output", func() {
			output := "kind v0.26.0 darwin/amd64"
			regex := `(\d+\.\d+\.\d+)`
			version := extractVersion(output, regex)
			Expect(version).To(Equal("0.26.0"))
		})

		It("should extract version from kubectl version output", func() {
			output := `Client Version: v1.31.1`
			regex := `v?(\d+\.\d+\.\d+)`
			version := extractVersion(output, regex)
			Expect(version).To(Equal("1.31.1"))
		})

		It("should return an empty string if no match is found", func() {
			output := "some random string"
			regex := `(\d+\.\d+\.\d+)`
			version := extractVersion(output, regex)
			Expect(version).To(BeEmpty())
		})
	})

	Describe("compareVersions", func() {
		It("should return true when version1 is greater than version2", func() {
			Expect(compareVersions("1.2.3", "1.2.2")).To(BeTrue())
			Expect(compareVersions("1.3.0", "1.2.9")).To(BeTrue())
			Expect(compareVersions("2.0.0", "1.9.9")).To(BeTrue())
		})

		It("should return true when versions are equal", func() {
			Expect(compareVersions("1.2.3", "1.2.3")).To(BeTrue())
		})

		It("should return false when version1 is less than version2", func() {
			Expect(compareVersions("1.2.2", "1.2.3")).To(BeFalse())
			Expect(compareVersions("1.1.9", "1.2.0")).To(BeFalse())
			Expect(compareVersions("0.9.9", "1.0.0")).To(BeFalse())
		})

		It("should handle 'v' prefix correctly", func() {
			Expect(compareVersions("v1.2.3", "1.2.3")).To(BeTrue())
			Expect(compareVersions("1.2.3", "v1.2.3")).To(BeTrue())
			Expect(compareVersions("v1.2.4", "1.2.3")).To(BeTrue())
			Expect(compareVersions("1.2.2", "v1.2.3")).To(BeFalse())
		})

		It("should handle different numbers of components", func() {
			Expect(compareVersions("1.2", "1.1.9")).To(BeTrue())
			Expect(compareVersions("1.2", "1.2.0")).To(BeTrue())
			Expect(compareVersions("1.2", "1.2.1")).To(BeFalse())
		})
	})

	Describe("Checker", func() {
		var (
			checker *Checker
			ctx     context.Context
			cfg     *config.Config
		)

		BeforeEach(func() {
			ctx = context.Background()
			// Create a mock config
			cfg = &config.Config{}
			checker = NewChecker(cfg)
		})

		Describe("Tool Checks with Mock Executables", func() {
			var (
				originalPath string
				tempBinDir   string
			)

			BeforeEach(func() {
				originalPath = os.Getenv("PATH")
				var err error
				tempBinDir, err = os.MkdirTemp("", "fake-bin-*")
				Expect(err).NotTo(HaveOccurred())
				// Set PATH to ONLY the temp directory to ensure we're testing mock tools only
				_ = os.Setenv("PATH", tempBinDir)
			})

			AfterEach(func() {
				_ = os.Setenv("PATH", originalPath)
				_ = os.RemoveAll(tempBinDir)
			})

			// Helper to create a mock executable
			createMockTool := func(name, versionOutput string) {
				script := fmt.Sprintf("#!/bin/sh\necho '%s'", versionOutput)
				path := filepath.Join(tempBinDir, name)
				Expect(os.WriteFile(path, []byte(script), 0755)).To(Succeed())
			}

			Describe("checkTool", func() {
				It("should return 'ok' for a valid tool version", func() {
					createMockTool("go", "go version go1.25.0 linux/amd64")
					result := checker.checkTool(ctx, "go", "go", []string{"version"}, "1.24.0", `go(\d+\.\d+(?:\.\d+)?)`)
					Expect(result.Status).To(Equal("ok"))
					Expect(result.Version).To(Equal("1.25.0"))
				})

				It("should return 'outdated' for an old tool version", func() {
					createMockTool("go", "go version go1.23.0 linux/amd64")
					result := checker.checkTool(ctx, "go", "go", []string{"version"}, "1.24.0", `go(\d+\.\d+(?:\.\d+)?)`)
					Expect(result.Status).To(Equal("outdated"))
					Expect(result.Version).To(Equal("1.23.0"))
				})

				It("should return 'missing' for a tool that is not installed", func() {
					result := checker.checkTool(ctx, "nonexistent", "nonexistent", []string{"version"}, "1.0.0", "")
					Expect(result.Status).To(Equal("missing"))
				})
			})

			Describe("CheckAll", func() {
				It("should pass when all tools are present and up-to-date", func() {
					createMockTool("go", "go version go1.24.0")
					createMockTool("kind", "kind v0.26.0")
					createMockTool("kubectl", "Client Version: v1.31.1")
					createMockTool("docker", "Docker version 27.0.1")
					createMockTool("git", "git version 2.46.0")
					createMockTool("helm", "v3.0.0")

					result, err := checker.CheckAll(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.AllMet).To(BeTrue())
				})

				It("should fail when a tool is missing", func() {
					createMockTool("go", "go version go1.24.0")
					// Missing kind
					createMockTool("kubectl", "Client Version: v1.31.1")
					createMockTool("docker", "Docker version 27.0.1")
					createMockTool("git", "git version 2.46.0")
					createMockTool("helm", "v3.0.0")

					result, err := checker.CheckAll(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.AllMet).To(BeFalse())
					Expect(result.Errors).To(ContainElement("kind is not installed"))
				})

				It("should use podman as a fallback for docker", func() {
					createMockTool("go", "go version go1.24.0")
					createMockTool("kind", "kind v0.26.0")
					createMockTool("kubectl", "Client Version: v1.31.1")
					// Missing docker
					createMockTool("podman", "podman version 5.3.1")
					createMockTool("git", "git version 2.46.0")
					createMockTool("helm", "v3.0.0")

					result, err := checker.CheckAll(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.AllMet).To(BeTrue())
					// Verify podman was detected as fallback
					podmanResult, exists := result.Prerequisites["podman"]
					Expect(exists).To(BeTrue(), "podman should be checked when docker is missing")
					Expect(podmanResult.Status).To(Equal("ok"))
				})
			})
		})
	})
})
