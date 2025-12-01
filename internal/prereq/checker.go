// Package prereq provides prerequisite checking for the MPC Dev Environment.
//
// It verifies that all required tools (Go, kubectl, kind, Docker/Podman, git, helm)
// are installed and meet minimum version requirements.
//
// The checker supports Docker/Podman flexibility - if Docker is not available, it
// will check for Podman as an alternative and accept it if version requirements are met.
package prereq

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/meyrevived/mpc-dev-env/internal/config"
)

// PrerequisiteResult represents the result of checking a single prerequisite.
// It contains the tool name, installation status, version information, and
// whether it meets the minimum requirement.
type PrerequisiteResult struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Required  string `json:"required"`
	Status    string `json:"status"` // "ok", "missing", "outdated", "unknown"
}

// CheckResult represents the overall result of all prerequisite checks.
// It aggregates results from individual tool checks and provides a summary
// of whether all prerequisites are met and any errors encountered.
type CheckResult struct {
	Prerequisites map[string]PrerequisiteResult `json:"prerequisites"`
	AllMet        bool                          `json:"all_met"`
	Errors        []string                      `json:"errors,omitempty"`
}

// Checker performs prerequisite checks for required tools and system configuration.
// It validates that the development environment has everything needed to run the
// MPC Dev Environment daemon and create Kind clusters.
type Checker struct {
	config *config.Config
}

// NewChecker creates a new prerequisite checker with the provided configuration.
func NewChecker(cfg *config.Config) *Checker {
	return &Checker{
		config: cfg,
	}
}

// CheckAll runs all prerequisite checks and returns the aggregated results.
//
// It checks for:
//   - Go (minimum 1.24.0)
//   - kind (minimum 0.26.0)
//   - kubectl (minimum 1.31.1)
//   - Docker or Podman (minimum Docker 27.0.1 or Podman 5.3.1)
//   - git (minimum 2.46.0)
//   - helm (minimum 3.0.0)
//
// If Docker is not available but Podman is, Podman will be accepted as an alternative.
// The function returns a CheckResult with all individual check results and an overall
// status indicating whether all prerequisites are met.
func (c *Checker) CheckAll(ctx context.Context) (*CheckResult, error) {
	result := &CheckResult{
		Prerequisites: make(map[string]PrerequisiteResult),
		AllMet:        true,
		Errors:        []string{},
	}

	// Define prerequisite checks
	checks := []struct {
		name         string
		command      string
		args         []string
		required     string
		versionRegex string
	}{
		{
			name:         "go",
			command:      "go",
			args:         []string{"version"},
			required:     "1.24.0",
			versionRegex: `go(\d+\.\d+(?:\.\d+)?)`,
		},
		{
			name:         "kind",
			command:      "kind",
			args:         []string{"--version"},
			required:     "0.26.0",
			versionRegex: `(\d+\.\d+\.\d+)`,
		},
		{
			name:         "kubectl",
			command:      "kubectl",
			args:         []string{"version", "--client"},
			required:     "1.31.1",
			versionRegex: `v?(\d+\.\d+\.\d+)`,
		},
		{
			name:         "docker",
			command:      "docker",
			args:         []string{"--version"},
			required:     "27.0.1",
			versionRegex: `(\d+\.\d+\.\d+)`,
		},
		{
			name:         "git",
			command:      "git",
			args:         []string{"--version"},
			required:     "2.46.0",
			versionRegex: `(\d+\.\d+\.\d+)`,
		},
		{
			name:         "helm",
			command:      "helm",
			args:         []string{"version", "--short"},
			required:     "3.0.0",
			versionRegex: `v?(\d+\.\d+\.\d+)`,
		},
	}

	// Run each check
	for _, check := range checks {
		prereqResult := c.checkTool(ctx, check.name, check.command, check.args, check.required, check.versionRegex)
		result.Prerequisites[check.name] = prereqResult

		// Update overall status
		if prereqResult.Status != "ok" {
			result.AllMet = false
			switch prereqResult.Status {
			case "missing":
				result.Errors = append(result.Errors, check.name+" is not installed")
			case "outdated":
				result.Errors = append(result.Errors, fmt.Sprintf("%s version %s is below minimum requirement %s",
					check.name, prereqResult.Version, prereqResult.Required))
			}
		}
	}

	// Special handling for Docker/Podman - check for either one
	dockerResult := result.Prerequisites["docker"]
	if dockerResult.Status != "ok" {
		// Docker is not available, try Podman
		podmanResult := c.checkTool(ctx, "podman", "podman", []string{"--version"}, "5.3.1", `(\d+\.\d+\.\d+)`)

		if podmanResult.Status == "ok" {
			// Podman is available, use it instead
			result.Prerequisites["podman"] = podmanResult

			// Remove Docker error since we have Podman
			filteredErrors := []string{}
			for _, err := range result.Errors {
				if !strings.Contains(err, "docker") {
					filteredErrors = append(filteredErrors, err)
				}
			}
			result.Errors = filteredErrors

			// If all other prerequisites are met, we're good
			allOthersOk := true
			for name, prereq := range result.Prerequisites {
				if name != "docker" && prereq.Status != "ok" {
					allOthersOk = false
					break
				}
			}
			result.AllMet = allOthersOk
		} else {
			// Neither Docker nor Podman is available
			result.Prerequisites["podman"] = podmanResult
			result.Errors = append(result.Errors, "Neither Docker nor Podman is available")
		}
	}

	return result, nil
}

// checkTool checks if a specific tool is installed and meets version requirements.
// It executes the tool's version command, extracts the version using regex, and
// compares it against the minimum required version.
//
// Returns a PrerequisiteResult with status:
//   - "ok" if tool is installed and version meets requirement
//   - "missing" if tool is not found in PATH
//   - "outdated" if tool version is below requirement
//   - "unknown" if version cannot be determined
func (c *Checker) checkTool(ctx context.Context, name, command string, args []string, requiredVersion, versionRegex string) PrerequisiteResult {
	result := PrerequisiteResult{
		Name:      name,
		Installed: false,
		Version:   "Not Found",
		Required:  requiredVersion,
		Status:    "missing",
	}

	// Check if command exists
	_, err := exec.LookPath(command)
	if err != nil {
		return result
	}

	result.Installed = true

	// Execute version command
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = "unknown"
		result.Version = "Unknown"
		return result
	}

	// Extract version from output
	version := extractVersion(string(output), versionRegex)
	if version == "" {
		result.Status = "unknown"
		result.Version = "Unknown"
		return result
	}

	result.Version = version

	// Compare versions
	if compareVersions(version, requiredVersion) {
		result.Status = "ok"
	} else {
		result.Status = "outdated"
	}

	return result
}

// extractVersion extracts a version string from command output using a regex pattern.
// It looks for the first capture group in the regex match.
// Returns an empty string if no match is found.
func extractVersion(output, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// compareVersions compares two semantic version strings.
// It parses each version as major.minor.patch and compares numerically.
// Missing components are treated as 0.
//
// Returns true if version1 >= version2, false otherwise.
func compareVersions(version1, version2 string) bool {
	// Remove 'v' prefix if present
	v1 := strings.TrimPrefix(version1, "v")
	v2 := strings.TrimPrefix(version2, "v")

	// Split into components
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Ensure we have at least 3 parts
	for len(parts1) < 3 {
		parts1 = append(parts1, "0")
	}
	for len(parts2) < 3 {
		parts2 = append(parts2, "0")
	}

	// Compare each component
	for i := 0; i < 3; i++ {
		var num1, num2 int
		_, _ = fmt.Sscanf(parts1[i], "%d", &num1)
		_, _ = fmt.Sscanf(parts2[i], "%d", &num2)

		if num1 > num2 {
			return true
		} else if num1 < num2 {
			return false
		}
	}

	// Versions are equal
	return true
}
