package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/meyrevived/mpc-dev-env/internal/config"
)

// TestManagerCreation tests that we can create a Manager instance
func TestManagerCreation(t *testing.T) {
	// Create a minimal config
	cfg := &config.Config{
		MpcDevEnvPath: "/tmp/test",
	}

	manager := NewManager(cfg)
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.config != cfg {
		t.Error("Manager config does not match provided config")
	}
}

// TestStatusWithTimeout tests that Status respects context timeout
func TestStatusWithTimeout(t *testing.T) {
	cfg := &config.Config{
		MpcDevEnvPath: "/tmp/test",
	}

	manager := NewManager(cfg)

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Status should complete quickly (just runs "kind get clusters")
	status, err := manager.Status(ctx)

	// We don't care about the exact status or error here
	// We just want to verify it doesn't hang indefinitely
	t.Logf("Status: %s, Error: %v", status, err)
}
