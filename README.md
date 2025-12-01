# MPC Dev Environment

A complete local development environment for the Multi-Platform Controller (MPC)

## Overview

The MPC Dev Environment tool automates the setup of a complete local Kubernetes development environment for the Multi-Platform Controller. In a single command, it:

- Creates a local Kind cluster optimized for MPC development
- Builds and deploys the MPC controller from your local source code
- Configures AWS secrets for multi-platform builds
- Deploys minimal MPC stack (Tekton Pipelines, MPC Operator, OTP Server)
- Enables TaskRun testing with automated log streaming
- Provides intelligent cleanup options at every step

This tool is designed for MPC developers who need to quickly iterate on code changes, test TaskRuns, and debug issues in a realistic but isolated environment.

## Prerequisites

### Required Software

You must have these tools installed and available in your PATH:

| Tool | Minimum Version | Purpose |
|------|----------------|---------|
| Go | 1.24+ | Building MPC daemon and controller |
| kubectl | v1.31.1+ | Kubernetes cluster management |
| Kind | v0.26.0+ | Local Kubernetes cluster |
| **Podman** or Docker | v5.3.1+ (Podman) | Container runtime for building images |
| Helm | v3.0.0+ | Kubernetes package management |
| Git | Any recent | Repository operations |
| jq | Any recent | JSON parsing in scripts |

**Important**: This tool supports both **Podman** and **Docker** as container runtimes. The tool automatically detects which runtime is available and uses it for building MPC images and creating Kind clusters.

- **Podman** is recommended for Fedora/RHEL systems due to better SELinux compatibility
- **Docker** works on most systems but may have SELinux issues on Fedora/RHEL

If using Podman on Fedora/RHEL:
```bash
# Install Podman (if not already installed)
sudo dnf install podman

# Enable and start Podman socket for Kind
systemctl --user enable --now podman.socket
```

### Hardware Requirements

- **CPU**: At least 4 cores
- **RAM**: At least 8GB available
- **Disk**: ~20GB free space (for cluster, images, and logs)

### Repository Structure

The tool expects a specific directory layout. All repositories should be cloned as siblings:

```
~/Work/                             # Your workspace directory
├── mpc_dev_env/                    # This repository (the tool)
└── multi-platform-controller/      # MPC source code (required)
```

**Note**: The `multi-platform-controller` repository is where your MPC code lives. The tool builds and deploys from this location.

## Quick Start

### One-Command Setup

```bash
# Just run - no configuration needed!
make dev-env
```

**No setup required!** The tool automatically detects paths based on your directory structure:
- `MPC_DEV_ENV_PATH` → Current directory
- `MPC_REPO_PATH` → Looks for `multi-platform-controller` as sibling directory

**Standard directory layout:**
```
~/Work/
├── mpc_dev_env/                    # This tool (where you run make dev-env)
└── multi-platform-controller/      # MPC source code
```

**Manual override** (only if your structure differs):
```bash
export MPC_REPO_PATH="/custom/path/to/multi-platform-controller"
make dev-env
```

### What Happens During Setup

The `make dev-env` command runs through 8 phases:

1. **Prerequisites Validation** (30 seconds)
   - Verifies all required tools are installed
   - Checks environment variables are set
   - Builds the Go daemon if needed

2. **Daemon Startup** (10 seconds)
   - Starts background API server on port 8765
   - Validates system prerequisites via API
   - Auto-restarts if already running

3. **Cluster Creation** (2-3 minutes)
   - Creates Kind cluster named "konflux"
   - Waits 10 seconds for initialization
   - Polls cluster status for up to 5 minutes
   - Configures kubectl context
   - Verifies cluster accessibility
   - Shows detailed error messages with log locations if creation fails

4. **MPC Stack Deployment** (2-3 minutes)
   - Deploys Tekton Pipelines (TaskRun engine)
   - Deploys MPC Operator (controller)
   - Deploys OTP Server (one-time passwords)
   - **Minimal deployment - only what MPC needs**

5. **MPC Build & Deployment** (5-10 minutes)
   - Builds MPC container image from your local code
   - Auto-generates minimal host-config.yaml if not found
   - Waits for MPC controller deployment
   - Patches deployment with custom-built image
   - Waits for deployment to be ready

6. **TaskRun Workflow** (interactive)
   - **AWS Credential Prompt**: Only asks if your TaskRun uses AWS platforms
     - Platforms needing AWS: linux/arm64, linux/amd64, linux-mlarge/arm64, linux-mlarge/amd64
     - Platforms NOT needing AWS: local, linux/s390x, linux/ppc64le, linux/x86_64
   - Deploys AWS secrets if needed (30 seconds)
   - Prompts to select a TaskRun YAML file
   - Applies TaskRun to cluster
   - Streams logs to terminal and file

7. **TaskRun Monitoring** (varies)
   - Monitors TaskRun status
   - Streams logs in real-time
   - Saves logs to `logs/` directory

8. **Summary** (instant)
   - Displays environment status
   - Shows useful commands
   - Provides next steps

**Total time**: ~10-20 minutes for complete setup

## Interactive Prompts

The tool will prompt you at several points:

### Continue Prompts (Phases 5)
```
MPC deployed successfully. Continue to TaskRun? [y/n]:
```

- Answer **y** to proceed to the next phase
- Answer **n** to stop here and choose cleanup options

### AWS Credential Prompt (Phase 6)
```
Does your TaskRun use AWS platforms and need AWS secrets deployed? [y/n]:
```

- Answer **y** if your TaskRun needs AWS platforms → prompts for AWS credentials and SSH key
- Answer **n** if using local or IBM Cloud platforms only → skips credential collection and secrets deployment

### TaskRun Selection (Phase 6)
```
Do you want to apply a TaskRun?
[1] Use TaskRun from taskruns/ directory
[2] Provide path to TaskRun file
[3] Skip TaskRun
```

- Option **1**: Select from YAML files in `taskruns/` directory
- Option **2**: Provide absolute path to any TaskRun file
- Option **3**: Skip TaskRun testing and go straight to summary

## Working with TaskRuns

### Adding Your TaskRun

Place your TaskRun YAML files in the `taskruns/` directory so that it can be discovered by Phase 6's prompt

### Running TaskRuns

During Phase 8, the tool will:

1. List all `.yaml` files in `taskruns/` directory
2. Let you select one by number
3. Apply it to the cluster automatically
4. Stream logs to both terminal and a file

### Viewing Logs

Logs are saved in the `logs/` directory with timestamps:
`logs/my-test_20251127_143052.log`

**Log filename format**: `<taskrun-name>_YYYYMMDD_HHMMSS.log`

## Cleanup Options

The tool offers intelligent cleanup at every failure point and after TaskRun completion.

### During Failures

If a phase fails, you'll see cleanup options appropriate to that stage:

#### Level 1: Cluster Failed to Start
```
Cleanup options:
[1] Stop daemon and exit
[2] Keep daemon running for debugging
[3] Retry cluster creation
```

#### Level 2: MPC Stack Deployment Failed
```
Cleanup options:
[1] Delete cluster, stop daemon
[2] Keep everything, stop daemon
[3] Keep everything, keep daemon (you can manually fix the stack)
[4] Retry MPC stack deployment
```

#### Level 3: Secrets Deployment Failed
```
Cleanup options:
[1] Delete cluster, stop daemon
[2] Keep everything, stop daemon
[3] Keep everything, keep daemon (you can manually fix secrets)
[4] Retry secrets deployment
```

#### Level 4: MPC Build/Deployment Failed
```
Cleanup options:
[1] Delete cluster, stop daemon
[2] Keep cluster, stop daemon (allows debugging)
[3] Keep cluster, keep daemon (manual retry possible)
[4] Retry MPC build/deployment
```

### After TaskRun Completion

After a TaskRun completes (success or failure), you get the most options:

```
What would you like to do next?
[1] Apply another TaskRun (keeps everything running)
[2] Rebuild MPC only (fix code, redeploy, test again)
[3] Rebuild MPC + apply new TaskRun
[4] Full teardown (delete cluster, stop daemon, exit)
[5] Partial teardown (delete cluster, keep daemon, exit)
[6] Exit only (keep cluster + daemon running for manual work)
```

**Most common workflows:**

- **Iterating on code**: Choose [2], make code changes, MPC rebuilds automatically, then apply TaskRun again
- **Testing multiple TaskRuns**: Choose [1] repeatedly
- **Clean exit**: Choose [6] to keep everything running for manual kubectl work

### Interruption (Ctrl+C)

Press Ctrl+C at any time to interrupt:

```
Interrupted by user

Current state:
- Cluster: running
- Daemon: running
- MPC: deployed

Cleanup options:
[1] Delete cluster, stop daemon, exit
[2] Keep cluster, stop daemon, exit
[3] Keep everything running, exit
```

### Manual Cleanup

Tear down everything manually:

```bash
make teardown
```

This will:
- Delete the Kind cluster (if exists)
- Stop the daemon (if running)
- Clean up resources

## Common Workflows

### Workflow 1: Test Code Changes

Perfect for iterating on MPC code:

```bash
# Initial setup
make dev-env
# Complete all phases (MPC stack, then MPC), apply a TaskRun

# TaskRun fails (expected - you're debugging)
# Choose option [2] - Rebuild MPC only

# In another terminal: make code changes
cd $MPC_REPO_PATH
# Edit files...
git add .
git commit -m "Fix bug"

# Back in dev-env terminal: MPC rebuilds automatically
# Choose option [1] - Apply another TaskRun
# Test your fix

# Repeat until TaskRun succeeds
```

### Workflow 2: Test Multiple TaskRuns

Test different TaskRuns without rebuilding:

```bash
make dev-env
# Complete all phases

# Apply first TaskRun
# Choose option [1] - Apply another TaskRun

# Apply second TaskRun
# Choose option [1] - Apply another TaskRun

# Continue as needed
```

### Workflow 3: Development Without AWS

If you don't need AWS secrets:

```bash
make dev-env

# When prompted for credentials, answer 'n'
# Secrets deployment will be skipped automatically
# Continue with MPC stack and TaskRuns

# Use for TaskRuns that don't require AWS
```

### Workflow 4: Manual kubectl Work

Keep environment running for manual debugging:

```bash
make dev-env
# Complete setup and apply TaskRun

# Choose option [6] - Exit only
# Everything keeps running

# Now use kubectl manually
kubectl get pods -n multi-platform-controller
kubectl logs -f <pod-name> -n multi-platform-controller
kubectl describe taskrun <name> -n multi-platform-controller

# When done, clean up
make teardown
```

### Workflow 5: Quick E2E Testing

Test any TaskRun from your `taskruns/` directory with interactive prompts:

```bash
make test-e2e

# The workflow will:
# 1. Auto-detect paths (MPC_DEV_ENV_PATH and MPC_REPO_PATH)
# 2. List all TaskRun YAML files in taskruns/ directory
# 3. Prompt you to select which TaskRun to test
# 4. Ask if your TaskRun uses AWS platforms (y/n)
# 5. Run the complete dev-env workflow automatically
# 6. Prompt for cleanup options after TaskRun completes
```

**Interactive Prompts:**

```
Available TaskRuns in taskruns/ directory:

  [1] aws_test.yaml
  [2] localhost_test.yaml
  [3] taskrun_test2.yaml

Select TaskRun to test [1-3]: 2
✓ Selected TaskRun: localhost_test.yaml

Does your selected TaskRun use AWS platforms? (y/n): n
ℹ Will skip AWS credential setup

Running make dev-env...
You will be prompted for cleanup options after TaskRun completes.
[dev-env workflow runs automatically...]

# After TaskRun completes, choose cleanup option:
What would you like to do next?
[1] Apply another TaskRun (keeps everything running)
[2] Rebuild MPC only (fix code, redeploy, test again)
[3] Rebuild MPC + apply new TaskRun
[4] Full teardown (delete cluster, stop daemon, exit)
[5] Partial teardown (delete cluster, keep daemon, exit)
[6] Exit only (keep cluster + daemon running for manual work)
```

**Benefits:**
- No need to manually run through the full `make dev-env` workflow
- Quickly test different TaskRuns without rebuilding everything
- Flexibility to choose cleanup level based on your needs
- Logs saved automatically for verification

**Use Cases:**
- Testing a new TaskRun YAML before committing
- Verifying TaskRun behavior on localhost vs AWS platforms
- Quick validation of MPC code changes with a specific TaskRun
- Iterative testing with different cleanup strategies

## Architecture Note

**Why Minimal Stack:**

The Multi-Platform Controller requires only three components to function:

1. **Tekton Pipelines** - TaskRun execution engine (required for running builds)
2. **MPC Operator** - Controller deployment (the core MPC functionality)
3. **OTP Server** - One-time password service (for secure access to build hosts)

The workflow order reflects these minimal dependencies:

1. **Phase 3**: Create Kind cluster (the foundation)
2. **Phase 4**: Deploy MPC stack (Tekton + MPC Operator + OTP)
3. **Phase 5**: Build & deploy MPC (with your custom-built image)
4. **Phase 6**: Deploy AWS secrets (only if TaskRun needs them) + Apply TaskRun

In production environments, ArgoCD is used as a GitOps deployment tool, but for local development we apply the same manifests directly. This minimal approach:
- Reduces setup time from 30-45 min to 10-20 min
- Eliminates unnecessary dependencies (Kyverno, Dex, etc.)
- Matches production MPC dependencies exactly

## Troubleshooting

### Daemon Not Starting

**Symptoms**: Error about port 8765 already in use, or daemon fails to start

**Solutions**:
```bash
# Check if daemon is already running
lsof -ti :8765

# Kill existing daemon
lsof -ti :8765 | xargs kill -9 2>/dev/null

# Rebuild daemon
make build

# Try again
make dev-env
```

### Cluster Creation Failed

**Symptoms**: Phase 4 fails immediately or after timeout with error message showing log location

**Common Error Messages**:
- "Cluster status check failed" - Usually means Podman isn't running or kind binary has issues
- "Timeout waiting for cluster to become ready" - Cluster creation is taking longer than 5 minutes
- "kubectl cannot access cluster" - Cluster created but kubectl can't connect
- "exit status 137" or "OOM killed" - Container runtime/SELinux conflict (try switching to Podman if using Docker on Fedora/RHEL)

**Solutions**:
```bash
# 1. Verify Podman is running and socket is enabled
podman info
systemctl --user status podman.socket

# If socket is not running:
systemctl --user enable --now podman.socket

# 2. Check daemon logs for detailed error
cat $MPC_DEV_ENV_PATH/logs/daemon.log

# 3. Verify kind is working with Podman
kind version
KIND_EXPERIMENTAL_PROVIDER=podman kind get clusters

# 4. Check system resources
free -h  # At least 8GB RAM
df -h    # At least 20GB disk

# 5. Delete any existing cluster
KIND_EXPERIMENTAL_PROVIDER=podman kind delete cluster --name konflux

# 6. Retry cluster creation (choose option [3] in cleanup menu)
```

### MPC Build Failed

**Symptoms**: Phase 5 times out or shows build errors

**Solutions**:
```bash
# Check MPC_REPO_PATH points to correct location
echo $MPC_REPO_PATH
ls -la $MPC_REPO_PATH

# Try building manually to see errors
cd $MPC_REPO_PATH
make build

# Check daemon logs for details
cat logs/daemon.log

# Retry build (choose option [4] in cleanup menu)
```

### TaskRun Not Starting

**Symptoms**: TaskRun stays in Pending state

**Solutions**:
```bash
# Check Tekton Pipelines is running
kubectl get pods -n tekton-pipelines

# Check TaskRun status
kubectl get taskrun -n multi-platform-controller
kubectl describe taskrun <name> -n multi-platform-controller

# Check pod status
kubectl get pods -n multi-platform-controller

# View daemon logs
cat logs/daemon.log
```

### Out of Disk Space

**Symptoms**: Build or deployment fails with disk space errors

**Solutions**:
```bash
# Quick automated cleanup (recommended)
make clean          # Removes build artifacts, prunes Podman containers and volumes
make clean-all      # Also removes Go caches and orphaned images

# Manual cleanup (if needed)
podman system prune -a  # or: docker system prune -a

# Remove old Kind clusters
kind get clusters
kind delete cluster --name <old-cluster>

# Remove old logs
rm logs/*.log

# Clear Go build cache (included in make clean-all)
go clean -cache
```

## Advanced Usage

### Using the Daemon API Directly

The daemon exposes an HTTP API on port 8765:

```bash
# Get environment status
curl http://localhost:8765/api/status | jq

# Rebuild MPC manually
curl -X POST http://localhost:8765/api/mpc/rebuild-and-redeploy

# Check cluster status
curl http://localhost:8765/api/cluster/status | jq

# View prerequisites
curl http://localhost:8765/api/prerequisites | jq
```

### Customizing the Workflow

You can skip phases by modifying the environment:

```bash
# Skip credential collection (use existing env vars)
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"
export SSH_KEY_PATH="$HOME/.ssh/id_rsa"
export CREDENTIALS_PROVIDED=true

# Run dev-env
make dev-env
# Answer 'n' to credential prompt - it will use env vars
```

## Host Configuration

### Built-In host-config.yaml

During Phase 5 (MPC Deployment), the tool automatically generates a **minimal configuration** optimized for local development if the file doesn't exist.
But there is also a default built-in host-config.yaml file.

**Location**: `mpc_dev_env/temp/host-config.yaml` 

**Included Platforms**:
- **4 AWS Dynamic Platforms**:
  - `linux/arm64` (m6g.large)
  - `linux/amd64` (m6a.large)
  - `linux-mlarge/arm64` (m6g.large)
  - `linux-mlarge/amd64` (m6a.large)
- **2 Static Hosts** (for testing, point to localhost):
  - `linux/s390x` (s390x-dev)
  - `linux/ppc64le` (ppc64le-dev)

### Platform Configuration Details

Each platform in host-config requires these settings:

**Dynamic (AWS) Platforms**:
- `type`: Cloud provider (aws, ibm, etc.)
- `region`: AWS region
- `ami`: AMI ID for the platform
- `instance-type`: EC2 instance type
- `key-name`: SSH key name in AWS
- `aws-secret`: Kubernetes secret name for AWS credentials
- `ssh-secret`: Kubernetes secret name for SSH key
- `security-group-id`: AWS security group
- `subnet-id`: AWS subnet
- `max-instances`: Maximum concurrent instances

**Static Hosts**:
- `address`: Host IP or hostname
- `platform`: Platform architecture
- `user`: SSH user
- `secret`: Kubernetes secret name for SSH key
- `concurrency`: Number of parallel builds

## Project Structure

```
mpc_dev_env/
├── cmd/                                    # Go application entry points
│   └── mpc-daemon/
│       ├── main.go                         # Daemon main entry point with file watching
│       └── main_test.go                    # Daemon main tests
│
├── internal/                               # Internal Go packages
│   ├── api/
│   │   └── models.go                       # Shared API request/response models
│   ├── build/
│   │   ├── builder.go                      # MPC image building (Podman/Docker)
│   │   └── builder_test.go                 # Builder tests
│   ├── cluster/
│   │   ├── manager.go                      # Kind cluster lifecycle management
│   │   └── manager_test.go                 # Cluster manager tests
│   ├── config/
│   │   ├── config.go                       # Configuration management (auto-detection)
│   │   └── config_test.go                  # Configuration tests
│   ├── daemon/
│   │   ├── api/
│   │   │   ├── handlers.go                 # HTTP API handlers
│   │   │   ├── handlers_test.go            # Handler tests
│   │   │   ├── router.go                   # HTTP router setup
│   │   │   └── router_test.go              # Router tests
│   │   ├── git/
│   │   │   ├── manager.go                  # Git operations (fork-aware)
│   │   │   └── manager_test.go             # Git manager tests
│   │   └── state/
│   │       ├── manager.go                  # State management (thread-safe)
│   │       ├── manager_test.go             # State manager tests
│   │       ├── models.go                   # State data models
│   │       └── models_test.go              # Model tests
│   ├── deploy/
│   │   ├── manager.go                      # Deployment orchestration
│   │   ├── manager_test.go                 # Deployment manager tests
│   │   ├── minimal.go                      # Minimal MPC stack (Tekton + MPC Operator + OTP)
│   │   └── minimal_test.go                 # Minimal deployment tests
│   ├── git/
│   │   ├── syncer.go                       # Git sync operations
│   │   └── syncer_test.go                  # Git syncer tests
│   ├── prereq/
│   │   ├── checker.go                      # Prerequisites validation
│   │   └── checker_test.go                 # Prerequisites checker tests
│   └── taskrun/
│       ├── manager.go                      # TaskRun execution and monitoring
│       └── manager_test.go                 # TaskRun manager tests
│
├── scripts/                                # Bash scripts for workflow orchestration
│   ├── dev-env.sh                          # Main dev environment setup script
│   ├── cleanup.sh                          # 6-level cleanup system
│   ├── api-client.sh                       # HTTP API client helpers
│   ├── utils.sh                            # Utility functions (logging, prompts)
│   ├── test-dev-env.sh                     # Comprehensive testing script
│   └── test-e2e.sh                         # Interactive E2E TaskRun testing
│
├── taskruns/                               # TaskRun YAML files for testing
│   ├── .gitkeep                            # Ensures directory exists in git
│   ├── aws_test.yaml                       # Example TaskRun using AWS platforms
│   ├── localhost_test.yaml                 # Example TaskRun using local platform
│   └── taskrun_test2.yaml                  # Additional test TaskRun
│
├── logs/                                   # Runtime logs (gitignored)
│   ├── .gitkeep                            # Ensures directory exists in git
│   ├── daemon.log                          # Daemon output and errors
│   └── <taskrun>_<timestamp>.log           # TaskRun execution logs
│
├── temp/                                   # Auto-generated temporary files
│   └── host-config.yaml                    # Auto-generated MPC platform config
│
├── bin/                                    # Compiled binaries (gitignored)
│   └── mpc-daemon                          # Daemon binary (created by make build)
│
├── goland-plugin/                          # GoLand IDE plugin (separate project)
│   └── (plugin source files)
│
├── .gitignore                              # Git ignore patterns
├── .golangci.yaml                          # Linter configuration
├── go.mod                                  # Go module definition
├── go.sum                                  # Go dependency checksums
├── host-config.yaml                        # Production host-config example
├── Makefile                                # Build and workflow targets
├── README.md                               # This file
```

## Environment Variables

### Auto-Detected (Optional Override)

These variables are automatically detected based on your directory structure. Manual override is only needed if your setup differs from the standard layout.

- `MPC_REPO_PATH`: Path to multi-platform-controller repository
  - **Auto-detected**: Looks for `multi-platform-controller` as sibling directory
  - **Manual override**: `export MPC_REPO_PATH=/custom/path/to/multi-platform-controller`
  - Example: `$HOME/Work/multi-platform-controller`

- `MPC_DEV_ENV_PATH`: Path to this repository
  - **Auto-detected**: Uses current working directory when running `make` commands
  - **Manual override**: `export MPC_DEV_ENV_PATH=/custom/path/to/mpc_dev_env`
  - Example: `$HOME/Work/mpc_dev_env`

### Optional

- `AWS_ACCESS_KEY_ID`: AWS access key (for secrets deployment)
- `AWS_SECRET_ACCESS_KEY`: AWS secret key (for secrets deployment)
- `SSH_KEY_PATH`: SSH key path (default: `~/.ssh/id_rsa`)
- `CREDENTIALS_PROVIDED`: Set to `true` if credentials are pre-configured

## Makefile Targets

```bash
make help           # Show all available commands
make build          # Build the Go daemon
make run            # Run the daemon (auto-detects paths)
make dev-env        # Start complete development environment
make test-e2e       # Interactive E2E test (select TaskRun, choose cleanup)
make teardown       # Tear down everything (cluster + daemon)
make test           # Run Go tests
make test-api       # Run API tests only
make lint           # Run linter - golangci-lint
make clean          # Remove build artifacts and prune Podman containers/volumes
make clean-all      # Remove all generated files, caches, and orphaned images
make fmt            # Format Go code
make vet            # Run go vet
make deps           # Download Go dependencies
make setup          # Setup development environment (installs golangci-lint)
make env            # Show environment variables
```

## FAQ

**Q: How long does the initial setup take?**
A: ~10-20 minutes for the complete workflow (cluster + MPC stack + MPC + TaskRun). Subsequent runs are faster since the daemon binary is already built.

**Q: Can I run multiple TaskRuns in one session?**
A: Yes! After a TaskRun completes, choose option [1] to apply another TaskRun without rebuilding.

**Q: Do I need AWS credentials?**
A: Only if your TaskRuns require AWS multi-platform builds. You can skip credential collection for local-only testing.

**Q: Does the tool support both Docker and Podman?**
A: Yes! The tool automatically detects and works with both Docker and Podman (v5.3.1+). Podman is recommended for Fedora/RHEL systems due to better SELinux compatibility.

**Q: What if I press Ctrl+C during setup?**
A: You'll see a cleanup menu with options to keep or delete the cluster/daemon. Choose based on whether you want to debug the current state.

**Q: How do I update the MPC code and re-test?**
A: After a TaskRun, choose option [2] to rebuild MPC only. Make your code changes, and the rebuild happens automatically.

**Q: Can I manually apply TaskRuns with kubectl?**
A: Yes! Use option [6] to exit while keeping everything running, then use `kubectl apply -f your-taskrun.yaml`.

**Q: Where are the logs stored?**
A: In `logs/` directory:
- Daemon logs: `logs/daemon.log`
- TaskRun logs: `logs/<taskrun-name>_<timestamp>.log`

**Q: Can I customize the build platforms (host-config)?**
A: Yes! The tool auto-generates a minimal config with 4 AWS platforms and 2 static hosts. To customize:
1. Create your own `temp/host-config.yaml` before running `make dev-env`
2. Or modify the auto-generated file after the first run
3. See the production example at `host-config.yaml` in the root directory

**Q: How do I clean up everything?**
A: Run `make teardown` to delete the cluster and stop the daemon. For more thorough cleanup including containers and images, use `make clean` or `make clean-all`.

**Q: How do I set up the development environment for the first time?**
A: Run `make setup` to install all development dependencies including golangci-lint, download Go modules, and build the daemon binary. This is a one-time setup step.

## Known Limitations

1**Static Hosts in Auto-Generated Config**: The auto-generated `host-config.yaml` includes S390X and PPC64LE static hosts pointing to 127.0.0.1 for testing purposes only. These won't work for actual builds unless you have these architectures available locally or update the addresses.

This is a non-blocking limitations that don't affect functionality. Just change the damn file and you'll be able to use it

## Contributing

### Getting Started with Development

1. **Initial Setup**: Run `make setup` to install all development tools (including golangci-lint)
2. **Code Quality**: All code follows Go documentation standards - see existing code for examples
3. **Testing**: Run `make test` for unit tests, `make test-api` for API tests
4. **Linting**: Run `make lint` to check code quality (all existing code passes linting)
5. **Pre-commit**: Run `make check` for quick validation before committing

### Documentation

All code files are fully documented following [Go's documentation standards](https://go.dev/doc/comment):
- Package-level comments explain purpose and architecture
- All exported functions and types are documented
- Complex logic includes inline explanations

## Support

If you encounter issues:

1. Check the troubleshooting section above
2. Review daemon logs: `cat logs/daemon.log`
3. Check TaskRun logs: `cat logs/<taskrun>_<timestamp>.log`
4. Yell at me on Slack at @MRATH (use capslock only, for that extra-yelling feel) or email me at `mrath@redhat.com` and 
wait for me to have the time to check my emails (time-blocked for ~1 a day)

---

I wrote and vibed most of this thing at 22:00PM - 02:00AM, don't be judgy lest ye be judged 
