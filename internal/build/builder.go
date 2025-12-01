// Package build provides functionality for building Multi-Platform Controller (MPC)
// container images using either Docker or Podman.
//
// The package automatically detects which container runtime is available and uses it
// to build the MPC image from the local source repository. After building, the image
// is loaded into the Kind cluster for deployment.
//
// The build process streams output to logs and supports context-based cancellation
// for long-running builds.
package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/meyrevived/mpc-dev-env/internal/config"
)

// Builder handles building MPC container images using Docker or Podman.
// It encapsulates the build configuration and runtime detection logic.
type Builder struct {
	config *config.Config
}

// NewBuilder creates a new Builder instance with the provided configuration.
// The configuration must contain a valid MPC repository path where the Dockerfile
// and source code are located.
func NewBuilder(cfg *config.Config) *Builder {
	return &Builder{
		config: cfg,
	}
}

// BuildMPCImage builds the multi-platform-controller container image
// It automatically detects whether to use docker or podman, builds the image,
// and streams build output to the daemon logs.
//
// Args:
//
//	ctx: Context for cancellation and timeout
//	config: Configuration containing MPC repository path
//
// Returns:
//
//	error: An error if the build fails, nil otherwise
func BuildMPCImage(ctx context.Context, cfg *config.Config) error {
	builder := NewBuilder(cfg)
	return builder.build(ctx)
}

// build performs the actual build operation for the MPC container image.
// It executes the following steps:
//  1. Detects container runtime (Docker or Podman)
//  2. Verifies Dockerfile exists in MPC repository
//  3. Builds the image with tag "multi-platform-controller:latest"
//  4. Streams build output to daemon logs
//  5. Loads the built image into the Kind cluster
//
// The build runs in the MPC repository directory and respects context cancellation.
func (b *Builder) build(ctx context.Context) error {
	log.Println("Starting MPC image build...")

	// Step 1: Determine container runtime (docker or podman)
	runtime, err := b.detectContainerRuntime()
	if err != nil {
		return fmt.Errorf("failed to detect container runtime: %w", err)
	}
	log.Printf("Using container runtime: %s", runtime)

	// Step 2: Set up build parameters
	buildContext := b.config.GetMpcRepoPath()
	dockerfile := filepath.Join(buildContext, "Dockerfile")

	// Verify Dockerfile exists
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("dockerfile not found at %s: %w", dockerfile, err)
	}

	// Step 3: Define image tag
	// Using a simple tag for now - can be made configurable later
	imageTag := "multi-platform-controller:latest"
	log.Printf("Building image: %s", imageTag)
	log.Printf("Build context: %s", buildContext)
	log.Printf("Dockerfile: %s", dockerfile)

	// Step 4: Construct build command
	// Format: <runtime> build -t <tag> -f <dockerfile> <context>
	buildArgs := []string{
		"build",
		"-t", imageTag,
		"-f", dockerfile,
		buildContext,
	}

	cmd := exec.CommandContext(ctx, runtime, buildArgs...)
	cmd.Dir = buildContext

	// Step 5: Set up streaming output
	// Capture both stdout and stderr and stream to logs
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start build command: %w", err)
	}

	// Stream stdout
	go b.streamOutput(stdout, "BUILD")

	// Stream stderr
	go b.streamOutput(stderr, "BUILD-ERR")

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("build command failed: %w", err)
	}

	log.Println("MPC image build completed successfully")

	// Step 6: Load image into Kind cluster
	if err := b.loadImageIntoKind(ctx, imageTag); err != nil {
		return fmt.Errorf("failed to load image into Kind cluster: %w", err)
	}

	return nil
}

// detectContainerRuntime determines whether to use docker or podman.
// It checks in the following order:
//  1. Check DOCKER_CLI environment variable (allows manual override)
//  2. Check if podman is available (preferred on RHEL/Fedora)
//  3. Check if docker is available (fallback)
//  4. Return error if neither is found
//
// Returns the runtime command name ("docker" or "podman") or an error if none are available.
func (b *Builder) detectContainerRuntime() (string, error) {
	// Check environment variable first
	if dockerCli := os.Getenv("DOCKER_CLI"); dockerCli != "" {
		// Verify the specified CLI exists
		if _, err := exec.LookPath(dockerCli); err == nil {
			return dockerCli, nil
		}
		log.Printf("WARNING: DOCKER_CLI is set to %s but command not found, trying alternatives...", dockerCli)
	}

	// Try podman first (it's often preferred in RHEL/Fedora environments)
	if _, err := exec.LookPath("podman"); err == nil {
		// Verify podman is working
		cmd := exec.Command("podman", "--version")
		if err := cmd.Run(); err == nil {
			return "podman", nil
		}
	}

	// Try docker
	if _, err := exec.LookPath("docker"); err == nil {
		// Verify docker is working
		cmd := exec.Command("docker", "--version")
		if err := cmd.Run(); err == nil {
			return "docker", nil
		}
	}

	return "", errors.New("neither docker nor podman found in PATH")
}

// streamOutput reads from an io.Reader and logs each line with a prefix.
// This function is designed to run in a goroutine and stream build output
// (stdout or stderr) to the daemon logs in real-time.
//
// Lines are buffered until a newline is encountered, then logged with the
// specified prefix (e.g., "BUILD" or "BUILD-ERR").
func (b *Builder) streamOutput(reader io.Reader, prefix string) {
	buf := make([]byte, 1024)
	var lineBuffer strings.Builder

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			// Process the bytes, looking for newlines
			chunk := string(buf[:n])
			for _, char := range chunk {
				if char == '\n' {
					// Log the complete line
					line := lineBuffer.String()
					if line != "" {
						log.Printf("[%s] %s", prefix, line)
					}
					lineBuffer.Reset()
				} else {
					lineBuffer.WriteRune(char)
				}
			}
		}

		if err == io.EOF {
			// Log any remaining content in the buffer
			if lineBuffer.Len() > 0 {
				log.Printf("[%s] %s", prefix, lineBuffer.String())
			}
			break
		}

		if err != nil {
			log.Printf("[%s-ERROR] Failed to read output: %v", prefix, err)
			break
		}
	}
}

// loadImageIntoKind loads the built image into the Kind cluster named "konflux".
// It uses a pipe between the container runtime's "save" command and kind's
// "load image-archive" command to efficiently transfer the image without creating
// a temporary tar file.
//
// For Podman, sets KIND_EXPERIMENTAL_PROVIDER=podman environment variable.
// The operation respects context cancellation.
func (b *Builder) loadImageIntoKind(ctx context.Context, imageTag string) error {
	log.Println("Loading image into Kind cluster...")

	// Determine container runtime
	runtime, err := b.detectContainerRuntime()
	if err != nil {
		return fmt.Errorf("failed to detect container runtime: %w", err)
	}

	// Use podman save to export image and pipe to kind load
	// Format: podman save <image> | KIND_EXPERIMENTAL_PROVIDER=podman kind load image-archive /dev/stdin --name konflux
	saveCmd := exec.CommandContext(ctx, runtime, "save", imageTag)
	loadCmd := exec.CommandContext(ctx, "kind", "load", "image-archive", "/dev/stdin", "--name", "konflux")

	// Set environment for kind if using podman
	if runtime == "podman" {
		loadCmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
	}

	// Create pipe between commands
	pipe, err := saveCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}

	loadCmd.Stdin = pipe
	loadCmd.Stdout = os.Stdout
	loadCmd.Stderr = os.Stderr

	// Start both commands
	if err := loadCmd.Start(); err != nil {
		return fmt.Errorf("failed to start kind load command: %w", err)
	}

	if err := saveCmd.Start(); err != nil {
		return fmt.Errorf("failed to start save command: %w", err)
	}

	// Wait for save to complete (writes to pipe)
	if err := saveCmd.Wait(); err != nil {
		return fmt.Errorf("save command failed: %w", err)
	}

	// Close the pipe to signal EOF to kind load
	_ = pipe.Close()

	// Wait for kind load to complete
	if err := loadCmd.Wait(); err != nil {
		return fmt.Errorf("kind load command failed: %w", err)
	}

	log.Println("Image loaded into Kind cluster successfully")
	return nil
}
