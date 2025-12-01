package state_test

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meyrevived/mpc-dev-env/internal/daemon/state"
)

func TestState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "State Models Suite")
}

var _ = Describe("ClusterState", func() {
	var clusterState state.ClusterState

	BeforeEach(func() {
		clusterState = state.ClusterState{
			Name:            "konflux-mpc-debug",
			CreatedAt:       time.Date(2024, 1, 15, 10, 35, 0, 0, time.UTC),
			Status:          "running",
			KubeconfigPath:  "/home/user/.kube/config",
			KonfluxDeployed: true,
		}
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			jsonBytes, err := json.Marshal(clusterState)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["name"]).To(Equal("konflux-mpc-debug"))
			Expect(result["status"]).To(Equal("running"))
			Expect(result["kubeconfig_path"]).To(Equal("/home/user/.kube/config"))
			Expect(result["konflux_deployed"]).To(BeTrue())
		})

		It("should deserialize from JSON correctly", func() {
			jsonStr := `{
				"name": "test-cluster",
				"created_at": "2024-01-15T10:35:00Z",
				"status": "paused",
				"kubeconfig_path": "/path/to/config",
				"konflux_deployed": false
			}`

			var result state.ClusterState
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Name).To(Equal("test-cluster"))
			Expect(result.Status).To(Equal("paused"))
			Expect(result.KubeconfigPath).To(Equal("/path/to/config"))
			Expect(result.KonfluxDeployed).To(BeFalse())
		})
	})

	Describe("Edge cases", func() {
		It("should handle empty fields", func() {
			emptyState := state.ClusterState{}
			jsonBytes, err := json.Marshal(emptyState)
			Expect(err).ToNot(HaveOccurred())

			var result state.ClusterState
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Name).To(BeEmpty())
			Expect(result.Status).To(BeEmpty())
			Expect(result.KonfluxDeployed).To(BeFalse())
		})
	})
})

var _ = Describe("RepositoryState", func() {
	var repoState state.RepositoryState

	BeforeEach(func() {
		repoState = state.RepositoryState{
			Name:                  "multi-platform-controller",
			Path:                  "/home/user/work/multi-platform-controller",
			CurrentBranch:         "fix-aws-timeout",
			LastSynced:            time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			CommitsBehindUpstream: 0,
			HasLocalChanges:       false,
		}
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			jsonBytes, err := json.Marshal(repoState)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["name"]).To(Equal("multi-platform-controller"))
			Expect(result["current_branch"]).To(Equal("fix-aws-timeout"))
			Expect(result["commits_behind_upstream"]).To(BeNumerically("==", 0))
			Expect(result["has_local_changes"]).To(BeFalse())
		})

		It("should deserialize from JSON correctly", func() {
			jsonStr := `{
				"name": "konflux-ci",
				"path": "/home/user/work/konflux-ci",
				"current_branch": "main",
				"last_synced": "2024-01-15T10:00:00Z",
				"commits_behind_upstream": 3,
				"has_local_changes": true
			}`

			var result state.RepositoryState
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Name).To(Equal("konflux-ci"))
			Expect(result.CurrentBranch).To(Equal("main"))
			Expect(result.CommitsBehindUpstream).To(Equal(3))
			Expect(result.HasLocalChanges).To(BeTrue())
		})
	})

	Describe("Edge cases", func() {
		It("should handle zero commits ahead", func() {
			repoState.CommitsBehindUpstream = 0
			jsonBytes, err := json.Marshal(repoState)
			Expect(err).ToNot(HaveOccurred())

			var result state.RepositoryState
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.CommitsBehindUpstream).To(Equal(0))
		})
	})
})

var _ = Describe("MPCDeployment", func() {
	var mpcDeployment state.MPCDeployment

	BeforeEach(func() {
		mpcDeployment = state.MPCDeployment{
			ControllerImage: "localhost:5001/multi-platform-controller:debug",
			OTPImage:        "localhost:5001/multi-platform-otp:debug",
			DeployedAt:      time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
			SourceGitHash:   "abc123def456",
		}
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			jsonBytes, err := json.Marshal(mpcDeployment)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["controller_image"]).To(Equal("localhost:5001/multi-platform-controller:debug"))
			Expect(result["otp_image"]).To(Equal("localhost:5001/multi-platform-otp:debug"))
			Expect(result["source_git_hash"]).To(Equal("abc123def456"))
		})

		It("should deserialize from JSON correctly", func() {
			jsonStr := `{
				"controller_image": "localhost:5001/controller:v1",
				"otp_image": "localhost:5001/otp:v1",
				"deployed_at": "2024-01-15T11:00:00Z",
				"source_git_hash": "xyz789"
			}`

			var result state.MPCDeployment
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.ControllerImage).To(Equal("localhost:5001/controller:v1"))
			Expect(result.OTPImage).To(Equal("localhost:5001/otp:v1"))
			Expect(result.SourceGitHash).To(Equal("xyz789"))
		})
	})
})

var _ = Describe("FeatureState", func() {
	var featureState state.FeatureState

	BeforeEach(func() {
		featureState = state.FeatureState{
			AWSEnabled: false,
			IBMEnabled: true,
		}
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			jsonBytes, err := json.Marshal(featureState)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["aws_enabled"]).To(BeFalse())
			Expect(result["ibm_enabled"]).To(BeTrue())
		})

		It("should deserialize from JSON correctly", func() {
			jsonStr := `{
				"aws_enabled": true,
				"ibm_enabled": false
			}`

			var result state.FeatureState
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.AWSEnabled).To(BeTrue())
			Expect(result.IBMEnabled).To(BeFalse())
		})
	})

	Describe("Edge cases", func() {
		It("should handle both features disabled", func() {
			featureState.AWSEnabled = false
			featureState.IBMEnabled = false

			jsonBytes, err := json.Marshal(featureState)
			Expect(err).ToNot(HaveOccurred())

			var result state.FeatureState
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.AWSEnabled).To(BeFalse())
			Expect(result.IBMEnabled).To(BeFalse())
		})

		It("should handle both features enabled", func() {
			featureState.AWSEnabled = true
			featureState.IBMEnabled = true

			jsonBytes, err := json.Marshal(featureState)
			Expect(err).ToNot(HaveOccurred())

			var result state.FeatureState
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.AWSEnabled).To(BeTrue())
			Expect(result.IBMEnabled).To(BeTrue())
		})
	})
})

var _ = Describe("DevEnvironment", func() {
	var devEnv state.DevEnvironment

	BeforeEach(func() {
		devEnv = state.DevEnvironment{
			SessionID:  "abc-123",
			CreatedAt:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			LastActive: time.Date(2024, 1, 15, 14, 45, 0, 0, time.UTC),
			Cluster: state.ClusterState{
				Name:            "konflux-mpc-debug",
				CreatedAt:       time.Date(2024, 1, 15, 10, 35, 0, 0, time.UTC),
				Status:          "running",
				KubeconfigPath:  "/home/user/.kube/config",
				KonfluxDeployed: true,
			},
			Repositories: map[string]state.RepositoryState{
				"multi-platform-controller": {
					Name:                  "multi-platform-controller",
					Path:                  "/home/user/work/multi-platform-controller",
					CurrentBranch:         "fix-aws-timeout",
					LastSynced:            time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
					CommitsBehindUpstream: 0,
					HasLocalChanges:       false,
				},
			},
			MPCDeployment: &state.MPCDeployment{
				ControllerImage: "localhost:5001/multi-platform-controller:debug",
				OTPImage:        "localhost:5001/multi-platform-otp:debug",
				DeployedAt:      time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
				SourceGitHash:   "abc123def456",
			},
			Features: state.FeatureState{
				AWSEnabled: false,
				IBMEnabled: true,
			},
		}
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			jsonBytes, err := json.Marshal(devEnv)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["session_id"]).To(Equal("abc-123"))
			Expect(result["cluster"]).ToNot(BeNil())
			Expect(result["repositories"]).ToNot(BeNil())
			Expect(result["mpc_deployment"]).ToNot(BeNil())
			Expect(result["features"]).ToNot(BeNil())
		})

		It("should deserialize from JSON correctly", func() {
			jsonStr := `{
				"session_id": "test-session",
				"created_at": "2024-01-15T10:30:00Z",
				"last_active": "2024-01-15T14:45:00Z",
				"cluster": {
					"name": "test-cluster",
					"created_at": "2024-01-15T10:35:00Z",
					"status": "running",
					"kubeconfig_path": "/path/to/config",
					"konflux_deployed": true
				},
				"repositories": {
					"mpc": {
						"name": "mpc",
						"path": "/path/to/mpc",
						"current_branch": "main",
						"last_synced": "2024-01-15T10:00:00Z",
						"commits_behind_upstream": 0,
						"has_local_changes": false
					}
				},
				"mpc_deployment": {
					"controller_image": "localhost:5001/controller:v1",
					"otp_image": "localhost:5001/otp:v1",
					"deployed_at": "2024-01-15T11:00:00Z",
					"source_git_hash": "xyz789"
				},
				"features": {
					"aws_enabled": true,
					"ibm_enabled": false
				}
			}`

			var result state.DevEnvironment
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.SessionID).To(Equal("test-session"))
			Expect(result.Cluster.Name).To(Equal("test-cluster"))
			Expect(result.Repositories).To(HaveKey("mpc"))
			Expect(result.MPCDeployment).ToNot(BeNil())
			Expect(result.Features.AWSEnabled).To(BeTrue())
		})
	})

	Describe("Edge cases", func() {
		It("should handle nil MPCDeployment", func() {
			devEnv.MPCDeployment = nil
			jsonBytes, err := json.Marshal(devEnv)
			Expect(err).ToNot(HaveOccurred())

			var result state.DevEnvironment
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.MPCDeployment).To(BeNil())
		})

		It("should handle empty repositories map", func() {
			devEnv.Repositories = map[string]state.RepositoryState{}
			jsonBytes, err := json.Marshal(devEnv)
			Expect(err).ToNot(HaveOccurred())

			var result state.DevEnvironment
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Repositories).To(BeEmpty())
		})

		It("should handle nil repositories map", func() {
			jsonStr := `{
				"session_id": "test",
				"created_at": "2024-01-15T10:30:00Z",
				"last_active": "2024-01-15T14:45:00Z",
				"cluster": {
					"name": "test",
					"created_at": "2024-01-15T10:35:00Z",
					"status": "running",
					"kubeconfig_path": "/path",
					"konflux_deployed": false
				},
				"repositories": null,
				"mpc_deployment": null,
				"features": {
					"aws_enabled": false,
					"ibm_enabled": false
				}
			}`

			var result state.DevEnvironment
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Repositories).To(BeNil())
		})
	})
})

var _ = Describe("ChangeSet", func() {
	var changeSet state.ChangeSet

	BeforeEach(func() {
		changeSet = state.ChangeSet{
			RepoName:              "multi-platform-controller",
			Commits:               []string{"abc123", "def456", "789ghi"},
			FilesChanged:          []string{"pkg/controller/ibm.go", "pkg/host/allocator.go"},
			PotentiallyAffectsMPC: true,
			ImpactSummary:         "3 commits: Fixed IBM timeout bug, improved host allocation",
			HasUpdates:            true,
		}
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			jsonBytes, err := json.Marshal(changeSet)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["repo_name"]).To(Equal("multi-platform-controller"))
			Expect(result["has_updates"]).To(BeTrue())
			Expect(result["potentially_affects_mpc"]).To(BeTrue())
		})

		It("should deserialize from JSON correctly", func() {
			jsonStr := `{
				"repo_name": "konflux-ci",
				"commits": ["commit1", "commit2"],
				"files_changed": ["file1.go", "file2.go"],
				"potentially_affects_mpc": false,
				"impact_summary": "Minor documentation updates",
				"has_updates": true
			}`

			var result state.ChangeSet
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.RepoName).To(Equal("konflux-ci"))
			Expect(result.Commits).To(HaveLen(2))
			Expect(result.PotentiallyAffectsMPC).To(BeFalse())
		})
	})

	Describe("Edge cases", func() {
		It("should handle empty commits slice", func() {
			changeSet.Commits = []string{}
			jsonBytes, err := json.Marshal(changeSet)
			Expect(err).ToNot(HaveOccurred())

			var result state.ChangeSet
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Commits).To(BeEmpty())
		})

		It("should handle nil slices", func() {
			jsonStr := `{
				"repo_name": "test",
				"commits": null,
				"files_changed": null,
				"potentially_affects_mpc": false,
				"impact_summary": "",
				"has_updates": false
			}`

			var result state.ChangeSet
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Commits).To(BeNil())
			Expect(result.FilesChanged).To(BeNil())
		})
	})
})

var _ = Describe("TestResult", func() {
	Describe("JSON serialization", func() {
		It("should serialize with no error", func() {
			testResult := state.TestResult{
				Passed:          true,
				DurationSeconds: 45,
				Output:          "ok  \tgithub.com/konflux-ci/multi-platform-controller/pkg/controller\t12.456s",
				Error:           nil,
			}

			jsonBytes, err := json.Marshal(testResult)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["passed"]).To(BeTrue())
			Expect(result["duration_seconds"]).To(BeNumerically("==", 45))
			Expect(result["error"]).To(BeNil())
		})

		It("should serialize with error", func() {
			errorMsg := "Test failed: timeout"
			testResult := state.TestResult{
				Passed:          false,
				DurationSeconds: 30,
				Output:          "FAIL",
				Error:           &errorMsg,
			}

			jsonBytes, err := json.Marshal(testResult)
			Expect(err).ToNot(HaveOccurred())

			var result state.TestResult
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Passed).To(BeFalse())
			Expect(result.Error).ToNot(BeNil())
			Expect(*result.Error).To(Equal("Test failed: timeout"))
		})
	})
})

var _ = Describe("TaskRunResult", func() {
	Describe("JSON serialization", func() {
		It("should serialize with no error", func() {
			taskRun := state.TaskRunResult{
				Name:            "smoke-test-abc123",
				Succeeded:       true,
				DurationSeconds: 42,
				Logs:            "Step 1/5: FROM golang:1.21\n...",
				Error:           nil,
			}

			jsonBytes, err := json.Marshal(taskRun)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["name"]).To(Equal("smoke-test-abc123"))
			Expect(result["succeeded"]).To(BeTrue())
			Expect(result["error"]).To(BeNil())
		})

		It("should serialize with error", func() {
			errorMsg := "Build failed: image not found"
			taskRun := state.TaskRunResult{
				Name:            "smoke-test-failed",
				Succeeded:       false,
				DurationSeconds: 10,
				Logs:            "Error output...",
				Error:           &errorMsg,
			}

			jsonBytes, err := json.Marshal(taskRun)
			Expect(err).ToNot(HaveOccurred())

			var result state.TaskRunResult
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Succeeded).To(BeFalse())
			Expect(result.Error).ToNot(BeNil())
			Expect(*result.Error).To(Equal("Build failed: image not found"))
		})
	})
})

var _ = Describe("MetricsConfig", func() {
	var metricsConfig state.MetricsConfig

	BeforeEach(func() {
		metricsConfig = state.MetricsConfig{
			PrometheusEnabled: true,
			GrafanaEnabled:    true,
			PrometheusURL:     "http://localhost:9090",
			GrafanaURL:        "http://localhost:3000",
			RetentionDays:     7,
		}
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			jsonBytes, err := json.Marshal(metricsConfig)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["prometheus_enabled"]).To(BeTrue())
			Expect(result["grafana_enabled"]).To(BeTrue())
			Expect(result["prometheus_url"]).To(Equal("http://localhost:9090"))
			Expect(result["grafana_url"]).To(Equal("http://localhost:3000"))
			Expect(result["retention_days"]).To(BeNumerically("==", 7))
		})

		It("should deserialize from JSON correctly", func() {
			jsonStr := `{
				"prometheus_enabled": false,
				"grafana_enabled": false,
				"prometheus_url": "http://prometheus:9090",
				"grafana_url": "http://grafana:3000",
				"retention_days": 14
			}`

			var result state.MetricsConfig
			err := json.Unmarshal([]byte(jsonStr), &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.PrometheusEnabled).To(BeFalse())
			Expect(result.GrafanaEnabled).To(BeFalse())
			Expect(result.RetentionDays).To(Equal(14))
		})
	})

	Describe("Edge cases", func() {
		It("should handle disabled metrics", func() {
			metricsConfig.PrometheusEnabled = false
			metricsConfig.GrafanaEnabled = false

			jsonBytes, err := json.Marshal(metricsConfig)
			Expect(err).ToNot(HaveOccurred())

			var result state.MetricsConfig
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.PrometheusEnabled).To(BeFalse())
			Expect(result.GrafanaEnabled).To(BeFalse())
		})
	})
})
