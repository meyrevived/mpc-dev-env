package api_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meyrevived/mpc-dev-env/internal/config"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/api"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/state"
)

var _ = Describe("Router", func() {
	var (
		mockState *mockStateManager
		mockCfg   *config.Config
		handlers  *api.Handlers
		router    *http.ServeMux
	)

	BeforeEach(func() {
		// Create mock dependencies
		mockState = &mockStateManager{
			stateToReturn: state.DevEnvironment{
				SessionID:  "test-session",
				CreatedAt:  time.Now(),
				LastActive: time.Now(),
			},
		}

		// Create minimal mock config
		mockCfg = &config.Config{}

		// Create handlers and router (no scriptRunner needed)
		handlers = api.NewHandlers(mockState, mockCfg)
		router = api.NewRouter(handlers)
	})

	Describe("NewRouter", func() {
		It("should create a new ServeMux", func() {
			Expect(router).NotTo(BeNil())
		})

		It("should register /api/status route", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// Should not return 404
			Expect(rr.Code).NotTo(Equal(http.StatusNotFound))
		})

		It("should register /api/rebuild route", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// Should not return 404
			Expect(rr.Code).NotTo(Equal(http.StatusNotFound))
		})
	})

	Describe("Route Mapping", func() {
		It("should map GET /api/status to StatusHandler", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
		})

		It("should map POST /api/rebuild to RebuildHandler", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusAccepted))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
		})

		It("should return 404 for unknown routes", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})

		It("should return 404 for root path", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})

		It("should return 404 for /api path without endpoint", func() {
			req := httptest.NewRequest(http.MethodGet, "/api", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("HTTP Method Handling", func() {
		It("should accept GET for /api/status", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("should reject POST for /api/status", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should accept POST for /api/rebuild", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusAccepted))
		})

		It("should reject GET for /api/rebuild", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})
	})

	Describe("Handler Integration", func() {
		It("should correctly call StatusHandler through router", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(ContainSubstring("test-session"))
		})

		It("should correctly call RebuildHandler through router", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/rebuild", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusAccepted))
			Expect(rr.Body.String()).To(ContainSubstring("rebuild initiated"))

			// Note: Script execution verification removed as RebuildHandler
			// now uses native Go build.BuildMPCImage() instead of shell scripts
		})
	})
})
