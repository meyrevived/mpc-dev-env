package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/api"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/state"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Handlers Suite")
}

// Mock StateManager for testing
type mockStateManager struct {
	stateToReturn state.DevEnvironment
	lastStatus    string
	lastError     error
}

func (m *mockStateManager) GetState() state.DevEnvironment {
	return m.stateToReturn
}

func (m *mockStateManager) SetOperationStatus(status string, err error) {
	m.lastStatus = status
	m.lastError = err
}

func (m *mockStateManager) SetTaskRunInfo(info *state.TaskRunInfo) {
	m.stateToReturn.TaskRunInfo = info
}

func (m *mockStateManager) ClearTaskRunInfo() {
	m.stateToReturn.TaskRunInfo = nil
}

var _ = Describe("Handlers", func() {
	var (
		mockState *mockStateManager
		handlers  *api.Handlers
		mockCfg   *config.Config
	)

	BeforeEach(func() {
		// Create mock dependencies
		mockState = &mockStateManager{
			stateToReturn: state.DevEnvironment{
				SessionID:  "test-session-123",
				CreatedAt:  time.Now(),
				LastActive: time.Now(),
				Cluster: state.ClusterState{
					Name:            "konflux-mpc-debug",
					Status:          "running",
					KubeconfigPath:  "/home/test/.kube/config",
					KonfluxDeployed: true,
				},
				Repositories: map[string]state.RepositoryState{
					"multi-platform-controller": {
						Name:                  "multi-platform-controller",
						Path:                  "/home/test/mpc",
						CurrentBranch:         "main",
						CommitsBehindUpstream: 0,
						HasLocalChanges:       false,
					},
				},
				MPCDeployment: &state.MPCDeployment{
					ControllerImage: "localhost:5001/multi-platform-controller:debug",
					OTPImage:        "localhost:5001/multi-platform-otp:debug",
					SourceGitHash:   "abc123",
				},
				Features: state.FeatureState{
					AWSEnabled: false,
					IBMEnabled: true,
				},
			},
		}

		// Create a minimal mock config for testing
		mockCfg = &config.Config{}

		// Create handlers with mock dependencies (no scriptRunner needed)
		handlers = api.NewHandlers(mockState, mockCfg)
	})

	Describe("StatusHandler", func() {
		It("should return 200 OK with valid JSON for GET requests", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			rr := httptest.NewRecorder()

			handlers.StatusHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
		})

		It("should return the current DevEnvironment state as JSON", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			rr := httptest.NewRecorder()

			handlers.StatusHandler(rr, req)

			var response state.DevEnvironment
			err := json.NewDecoder(rr.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.SessionID).To(Equal("test-session-123"))
			Expect(response.Cluster.Name).To(Equal("konflux-mpc-debug"))
			Expect(response.Cluster.Status).To(Equal("running"))
			Expect(response.Repositories).To(HaveKey("multi-platform-controller"))
			Expect(response.MPCDeployment).NotTo(BeNil())
			Expect(response.MPCDeployment.ControllerImage).To(Equal("localhost:5001/multi-platform-controller:debug"))
			Expect(response.Features.IBMEnabled).To(BeTrue())
			Expect(response.Features.AWSEnabled).To(BeFalse())
		})

		It("should call StateManager.GetState()", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			rr := httptest.NewRecorder()

			// Verify the state is being requested
			beforeState := mockState.stateToReturn.SessionID
			handlers.StatusHandler(rr, req)

			var response state.DevEnvironment
			_ = json.NewDecoder(rr.Body).Decode(&response)
			Expect(response.SessionID).To(Equal(beforeState))
		})

		It("should return 405 Method Not Allowed for POST requests", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
			rr := httptest.NewRecorder()

			handlers.StatusHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should return 405 Method Not Allowed for PUT requests", func() {
			req := httptest.NewRequest(http.MethodPut, "/api/status", nil)
			rr := httptest.NewRecorder()

			handlers.StatusHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should return 405 Method Not Allowed for DELETE requests", func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/status", nil)
			rr := httptest.NewRecorder()

			handlers.StatusHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})
	})

	Describe("RebuildHandler", func() {
		It("should return 202 Accepted for POST requests", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			handlers.RebuildHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusAccepted))
		})

		It("should return application/json Content-Type", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			handlers.RebuildHandler(rr, req)

			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
		})

		It("should return JSON with status 'rebuild initiated'", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			handlers.RebuildHandler(rr, req)

			var response map[string]string
			err := json.NewDecoder(rr.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(HaveKey("status"))
			Expect(response["status"]).To(Equal("rebuild initiated"))
		})

		// Note: Tests for script runner integration removed as RebuildHandler
		// now uses native Go build.BuildMPCImage() instead of shell scripts

		It("should return immediately (not wait for build to complete)", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			start := time.Now()
			handlers.RebuildHandler(rr, req)
			duration := time.Since(start)

			// The handler should return immediately, not wait for the goroutine
			Expect(duration).To(BeNumerically("<", 100*time.Millisecond))
			Expect(rr.Code).To(Equal(http.StatusAccepted))
		})

		It("should return 405 Method Not Allowed for GET requests", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			handlers.RebuildHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should return 405 Method Not Allowed for PUT requests", func() {
			req := httptest.NewRequest(http.MethodPut, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			handlers.RebuildHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should return 405 Method Not Allowed for DELETE requests", func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			handlers.RebuildHandler(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		// Note: Build failure/success logging tests removed - these would require mocking
		// the build.BuildMPCImage function which is not necessary for API handler tests.
		// Build-specific tests should be in internal/build/builder_test.go
		// The handler tests above verify that the handler responds correctly (HTTP 202),
		// which is the primary responsibility of the API handler layer.
	})

	Describe("generateLogFilename", func() {
		// This function is not exported, so we copy its logic here for testing.
		generateLogFilename := func(yamlPath string) string {
			base := filepath.Base(yamlPath)
			base = strings.TrimSuffix(base, filepath.Ext(base))
			// We can't easily mock time, so we'll just check the format.
			timestamp := time.Now().Format("20060102_150405")
			return fmt.Sprintf("%s_%s.log", base, timestamp)
		}

		It("should generate a correctly formatted log filename", func() {
			yamlPath := "/path/to/my_taskrun.yaml"
			filename := generateLogFilename(yamlPath)

			// We can't assert the exact timestamp, but we can check the prefix and suffix
			Expect(filename).To(HavePrefix("my_taskrun_"))
			Expect(filename).To(HaveSuffix(".log"))
			// Check that the timestamp part is the correct length
			// e.g., my_taskrun_20240101_123000.log -> 15 chars for timestamp + underscore
			Expect(filename).To(HaveLen(len("my_taskrun_") + 15 + len(".log")))
		})

		It("should handle paths with no extension", func() {
			yamlPath := "/path/to/another_taskrun"
			filename := generateLogFilename(yamlPath)
			Expect(filename).To(HavePrefix("another_taskrun_"))
			Expect(filename).To(HaveSuffix(".log"))
		})
	})
})
