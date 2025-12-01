// Package config provides configuration management for the MPC Dev Environment daemon.
//
// It handles loading and validating environment paths required for the daemon to operate,
// with automatic path detection as a fallback when environment variables are not set.
// The package supports a zero-configuration setup for standard directory layouts where
// mpc_dev_env and multi-platform-controller are sibling directories.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all environment-dependent paths and settings required
// by the MPC Dev Studio daemon.
type Config struct {
	// MpcRepoPath is the absolute path to the multi-platform-controller repository
	MpcRepoPath string

	// MpcDevEnvPath is the absolute path to the mpc_dev_env repository (this project)
	MpcDevEnvPath string

	// InfraDeploymentsPath is the absolute path to the infra-deployments repository
	InfraDeploymentsPath string

	// TempDir is the directory for temporary daemon operations (derived from MpcDevEnvPath)
	TempDir string
}

// LoadConfig reads environment variables and constructs the Config struct.
// It validates that critical paths exist and creates necessary directories.
//
// Environment variables (with auto-detection fallback):
//   - MPC_REPO_PATH: Path to the multi-platform-controller repository
//     Auto-detected: Looks for "multi-platform-controller" as sibling to working directory
//   - MPC_DEV_ENV_PATH: Path to the mpc_dev_env repository
//     Auto-detected: Uses current working directory
//
// Returns:
//   - *Config: The populated configuration struct
//   - error: An error if critical paths cannot be determined or validation fails
func LoadConfig() (*Config, error) {
	// Auto-detect or read MPC_DEV_ENV_PATH
	mpcDevEnvPath := os.Getenv("MPC_DEV_ENV_PATH")
	if mpcDevEnvPath == "" {
		// Auto-detect: use current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
		mpcDevEnvPath = cwd
	}

	// Auto-detect or read MPC_REPO_PATH
	mpcRepoPath := os.Getenv("MPC_REPO_PATH")
	if mpcRepoPath == "" {
		// Auto-detect: look for multi-platform-controller as sibling
		parentDir := filepath.Dir(mpcDevEnvPath)
		candidatePath := filepath.Join(parentDir, "multi-platform-controller")

		if _, err := os.Stat(candidatePath); err == nil {
			mpcRepoPath = candidatePath
		} else {
			return nil, fmt.Errorf("MPC_REPO_PATH not set and auto-detection failed: multi-platform-controller not found at %s", candidatePath)
		}
	}

	// Derive infra-deployments path as sibling to mpc_dev_env
	// Structure: ~/Work/mpc_dev_env and ~/Work/infra-deployments
	infraDeploymentsPath := filepath.Join(filepath.Dir(mpcDevEnvPath), "infra-deployments")

	// Construct derived paths
	tempDir := filepath.Join(mpcDevEnvPath, "temp")

	// Create the Config struct
	cfg := &Config{
		MpcRepoPath:          mpcRepoPath,
		MpcDevEnvPath:        mpcDevEnvPath,
		InfraDeploymentsPath: infraDeploymentsPath,
		TempDir:              tempDir,
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Ensure the temp directory exists
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required paths exist and are accessible.
func (c *Config) Validate() error {
	// Check that MPC_REPO_PATH exists
	if _, err := os.Stat(c.MpcRepoPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("MPC_REPO_PATH does not exist: %s", c.MpcRepoPath)
		}
		return fmt.Errorf("cannot access MPC_REPO_PATH: %w", err)
	}

	// Check that MPC_DEV_ENV_PATH exists
	if _, err := os.Stat(c.MpcDevEnvPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("MPC_DEV_ENV_PATH does not exist: %s", c.MpcDevEnvPath)
		}
		return fmt.Errorf("cannot access MPC_DEV_ENV_PATH: %w", err)
	}

	// Check that InfraDeploymentsPath exists
	if _, err := os.Stat(c.InfraDeploymentsPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("InfraDeploymentsPath does not exist: %s", c.InfraDeploymentsPath)
		}
		return fmt.Errorf("cannot access InfraDeploymentsPath: %w", err)
	}

	return nil
}

// GetTempDir returns the path to the temp directory for daemon operations.
func (c *Config) GetTempDir() string {
	return c.TempDir
}

// GetMpcRepoPath returns the path to the multi-platform-controller repository.
func (c *Config) GetMpcRepoPath() string {
	return c.MpcRepoPath
}

// GetMpcDevEnvPath returns the path to the mpc_dev_env repository.
func (c *Config) GetMpcDevEnvPath() string {
	return c.MpcDevEnvPath
}

// GetInfraDeploymentsPath returns the path to the infra-deployments repository.
func (c *Config) GetInfraDeploymentsPath() string {
	return c.InfraDeploymentsPath
}
