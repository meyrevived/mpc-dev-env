package config

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config", func() {
	var (
		originalWd string
		tempDir    string
	)

	BeforeEach(func() {
		var err error
		originalWd, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "config-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.Chdir(originalWd)).To(Succeed())
		_ = os.RemoveAll(tempDir)
		_ = os.Unsetenv("MPC_DEV_ENV_PATH")
		_ = os.Unsetenv("MPC_REPO_PATH")
	})

	Describe("LoadConfig", func() {
		var mpcDevEnvPath, mpcRepoPath string

		BeforeEach(func() {
			// Create a realistic directory structure
			mpcDevEnvPath = filepath.Join(tempDir, "mpc_dev_env")
			mpcRepoPath = filepath.Join(tempDir, "multi-platform-controller")

			Expect(os.MkdirAll(mpcDevEnvPath, 0755)).To(Succeed())
			Expect(os.MkdirAll(mpcRepoPath, 0755)).To(Succeed())
		})

		Context("with environment variables set", func() {
			It("should load configuration from environment variables", func() {
				_ = os.Setenv("MPC_DEV_ENV_PATH", mpcDevEnvPath)
				_ = os.Setenv("MPC_REPO_PATH", mpcRepoPath)

				cfg, err := LoadConfig()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())

				Expect(cfg.GetMpcDevEnvPath()).To(Equal(mpcDevEnvPath))
				Expect(cfg.GetMpcRepoPath()).To(Equal(mpcRepoPath))
				Expect(cfg.GetTempDir()).To(Equal(filepath.Join(mpcDevEnvPath, "temp")))
			})
		})

		Context("without environment variables (auto-detection)", func() {
			It("should auto-detect paths based on working directory", func() {
				// Change working directory to the simulated mpc_dev_env path
				Expect(os.Chdir(mpcDevEnvPath)).To(Succeed())

				cfg, err := LoadConfig()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())

				Expect(cfg.GetMpcDevEnvPath()).To(Equal(mpcDevEnvPath))
				Expect(cfg.GetMpcRepoPath()).To(Equal(mpcRepoPath))
			})

			It("should fail if multi-platform-controller sibling directory is not found", func() {
				// Remove the mpc repo path to cause failure
				Expect(os.RemoveAll(mpcRepoPath)).To(Succeed())
				Expect(os.Chdir(mpcDevEnvPath)).To(Succeed())

				_, err := LoadConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("multi-platform-controller not found"))
			})
		})

		Context("with invalid paths", func() {
			It("should fail validation if a path does not exist", func() {
				nonExistentPath := filepath.Join(tempDir, "non-existent")
				_ = os.Setenv("MPC_DEV_ENV_PATH", mpcDevEnvPath)
				_ = os.Setenv("MPC_REPO_PATH", nonExistentPath)

				_, err := LoadConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("configuration validation failed"))
				Expect(err.Error()).To(ContainSubstring("MPC_REPO_PATH does not exist"))
			})
		})
	})

	Describe("Validate", func() {
		var cfg *Config

		BeforeEach(func() {
			mpcDevEnvPath := filepath.Join(tempDir, "mpc_dev_env")
			mpcRepoPath := filepath.Join(tempDir, "multi-platform-controller")

			cfg = &Config{
				MpcDevEnvPath: mpcDevEnvPath,
				MpcRepoPath:   mpcRepoPath,
			}
		})

		It("should return no error if all paths exist", func() {
			Expect(os.MkdirAll(cfg.MpcDevEnvPath, 0755)).To(Succeed())
			Expect(os.MkdirAll(cfg.MpcRepoPath, 0755)).To(Succeed())

			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error if MpcRepoPath does not exist", func() {
			Expect(os.MkdirAll(cfg.MpcDevEnvPath, 0755)).To(Succeed())

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MPC_REPO_PATH does not exist"))
		})

		It("should return an error if MpcDevEnvPath does not exist", func() {
			Expect(os.MkdirAll(cfg.MpcRepoPath, 0755)).To(Succeed())

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MPC_DEV_ENV_PATH does not exist"))
		})
	})
})
