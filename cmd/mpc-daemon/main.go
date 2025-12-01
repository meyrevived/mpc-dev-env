// Package main implements the MPC Dev Environment daemon.
//
// The daemon provides an HTTP API (port 8765) for managing a local Kubernetes
// development environment for the Multi-Platform Controller (MPC). It handles:
//   - Kind cluster creation and management
//   - MPC image building and deployment
//   - Tekton TaskRun execution and monitoring
//   - Git repository synchronization
//   - File watching for hot reload during development
//
// The daemon is designed to be called by bash scripts that handle user interaction,
// while the daemon handles all Kubernetes, Tekton, and MPC operations.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/meyrevived/mpc-dev-env/internal/build"
	"github.com/meyrevived/mpc-dev-env/internal/cluster"
	"github.com/meyrevived/mpc-dev-env/internal/config"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/api"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/git"
	"github.com/meyrevived/mpc-dev-env/internal/daemon/state"
)

func main() {
	log.Println("Starting MPC Dev Studio daemon...")

	// Load configuration early in startup
	log.Println("Loading configuration...")
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	log.Printf("Configuration loaded successfully:")
	log.Printf("  MPC_REPO_PATH: %s", cfg.GetMpcRepoPath())
	log.Printf("  MPC_DEV_ENV_PATH: %s", cfg.GetMpcDevEnvPath())
	log.Printf("  TempDir: %s", cfg.GetTempDir())

	kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")

	// Step 1: Instantiate GitManager
	log.Println("Initializing GitManager...")
	gitManager := git.NewGitManager()

	// Step 2: Instantiate ClusterManager
	log.Println("Initializing ClusterManager...")
	clusterManager := cluster.NewManager(cfg)

	// Step 3: Instantiate StateManager
	log.Println("Initializing StateManager...")

	// Configure repository paths using Config
	repoPaths := map[string]string{
		"multi-platform-controller": cfg.GetMpcRepoPath(),
	}

	stateManagerConfig := &state.StateManagerConfig{
		GitManager:     gitManager,
		ClusterManager: clusterManager,
		RepoPaths:      repoPaths,
		KubeconfigPath: kubeconfigPath,
	}

	stateManager, err := state.NewStateManager(stateManagerConfig)
	if err != nil {
		log.Fatalf("Failed to create StateManager: %v", err)
	}
	log.Println("StateManager initialized successfully")

	// Step 4: Instantiate API handlers and router
	log.Println("Setting up API handlers and router...")
	handlers := api.NewHandlers(stateManager, cfg)
	router := api.NewRouter(handlers)

	// Step 5: Create and configure HTTP server
	server := &http.Server{
		Addr:    "localhost:8765",
		Handler: router,
	}

	// Step 6: Start the server in a goroutine
	go func() {
		log.Printf("HTTP server listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Step 7: Start background Git sync ticker
	// This replaces the Python UpstreamChangeDetector
	syncTicker := time.NewTicker(60 * time.Minute)
	defer syncTicker.Stop()

	go func() {
		log.Println("Starting background Git sync worker (every 60 minutes)...")

		// Perform initial sync on startup
		for repoName, repoPath := range repoPaths {
			log.Printf("Performing initial sync for repository: %s", repoName)
			if err := gitManager.Sync(repoPath); err != nil {
				log.Printf("WARNING: Failed to sync repository %s: %v", repoName, err)
			} else {
				log.Printf("Successfully synced repository: %s", repoName)
			}
		}

		// Periodic sync
		for range syncTicker.C {
			log.Println("Running periodic Git sync...")
			for repoName, repoPath := range repoPaths {
				if err := gitManager.Sync(repoPath); err != nil {
					log.Printf("WARNING: Failed to sync repository %s: %v", repoName, err)
				} else {
					log.Printf("Successfully synced repository: %s", repoName)
				}
			}
		}
	}()

	// Step 8: Start file watcher for hot reload (replaces detector.py)
	// Watch the multi-platform-controller directory for changes
	log.Println("Initializing file watcher for hot reload...")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("WARNING: Failed to create file watcher: %v", err)
	} else {
		defer func() {
			_ = watcher.Close()
		}()

		// Watch the MPC repository directory
		if err := addRecursiveWatch(watcher, cfg.GetMpcRepoPath()); err != nil {
			log.Printf("WARNING: Failed to add watch on %s: %v", cfg.GetMpcRepoPath(), err)
		} else {
			log.Printf("File watcher active on: %s", cfg.GetMpcRepoPath())

			// Start file watcher goroutine with debouncing
			go fileWatcherLoop(watcher, handlers, 2*time.Second)
		}
	}

	// Step 9: Implement graceful shutdown
	// Set up channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal
	sig := <-quit
	log.Printf("Received signal: %v. Shutting down gracefully...", sig)

	// Create a context with timeout for the shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Attempt to gracefully shutdown the server
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server shutdown complete")
	}

	log.Println("MPC Dev Studio daemon stopped")
}

// addRecursiveWatch adds a file system watcher recursively to all subdirectories
// under the given root path. It skips common ignore patterns like .git, node_modules,
// and IDE directories to reduce overhead.
//
// The watcher is used for hot reload functionality - when source files change in the
// MPC repository, the daemon can automatically rebuild and redeploy.
func addRecursiveWatch(watcher *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and common ignore patterns
		if info.IsDir() {
			name := filepath.Base(path)
			if name == ".git" || name == "__pycache__" || name == ".pytest_cache" ||
				name == "node_modules" || name == ".vscode" || name == ".idea" {
				return filepath.SkipDir
			}

			// Add watch on this directory
			if err := watcher.Add(path); err != nil {
				log.Printf("WARNING: Failed to watch directory %s: %v", path, err)
			}
		}

		return nil
	})
}

// fileWatcherLoop processes file system events with debouncing to implement hot reload.
// It listens for Write and Create events on source files and triggers a rebuild after
// a debounce period (default 2 seconds) to avoid multiple rebuilds for rapid file changes.
//
// The loop ignores temporary files (.swp, .log), hidden files, and non-code files to
// prevent unnecessary rebuild triggers.
func fileWatcherLoop(watcher *fsnotify.Watcher, handlers *api.Handlers, debounceDuration time.Duration) {
	var debounceTimer *time.Timer
	var lastChangeTime time.Time

	log.Println("File watcher loop started (Hot Reload active)")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				log.Println("File watcher events channel closed")
				return
			}

			// Ignore certain file types and operations
			if shouldIgnoreEvent(event) {
				continue
			}

			// Update last change time
			lastChangeTime = time.Now()

			// Cancel existing timer if any
			if debounceTimer != nil {
				debounceTimer.Stop()
			}

			// Create new debounce timer
			debounceTimer = time.AfterFunc(debounceDuration, func() {
				// Check if enough time has passed since last change
				if time.Since(lastChangeTime) >= debounceDuration {
					log.Printf("File changes detected, triggering rebuild...")
					triggerRebuild(handlers)
				}
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				log.Println("File watcher errors channel closed")
				return
			}
			log.Printf("File watcher error: %v", err)
		}
	}
}

// shouldIgnoreEvent returns true if the file system event should be ignored for hot reload.
// It filters out events that don't indicate meaningful source code changes:
//   - Non-Write/Create operations (Rename, Remove, Chmod)
//   - Temporary files (.swp, .log, .tmp, etc.)
//   - Hidden files (starting with .)
func shouldIgnoreEvent(event fsnotify.Event) bool {
	// Only watch Write and Create events
	if event.Op != fsnotify.Write && event.Op != fsnotify.Create {
		return true
	}

	// Ignore certain file extensions
	ext := filepath.Ext(event.Name)
	ignoreExtensions := []string{".swp", ".swo", ".pyc", ".pyo", ".log", ".tmp"}
	for _, ignoreExt := range ignoreExtensions {
		if ext == ignoreExt {
			return true
		}
	}

	// Ignore hidden files
	base := filepath.Base(event.Name)
	if len(base) > 0 && base[0] == '.' {
		return true
	}

	return false
}

// triggerRebuild triggers an MPC rebuild and redeploy operation.
// This function is used by both the file watcher (hot reload) and the HTTP API handler
// to ensure consistent rebuild behavior across both code paths.
//
// The rebuild process:
//  1. Sets operation status to "rebuilding"
//  2. Builds the MPC container image from local source
//  3. Updates operation status to "idle" on completion or error
//
// The operation runs with a 15-minute timeout to handle long build times.
func triggerRebuild(handlers *api.Handlers) {
	// Create a context with timeout for the rebuild (builds can take several minutes)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	log.Println("Starting rebuild (triggered by file watcher)...")

	// Update state to rebuilding
	handlers.StateManager.SetOperationStatus("rebuilding", nil)

	// Execute native Go build
	if err := build.BuildMPCImage(ctx, handlers.Config); err != nil {
		log.Printf("ERROR: Rebuild failed: %v", err)
		handlers.StateManager.SetOperationStatus("idle", err)
		return
	}

	log.Println("Rebuild completed successfully")
	handlers.StateManager.SetOperationStatus("idle", nil)
}
