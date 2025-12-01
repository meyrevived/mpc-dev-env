// Package taskrun provides complete TaskRun workflow management using Tekton and Kubernetes client-go.
//
// This package encapsulates all Tekton TaskRun operations for the MPC development environment:
//   - Parsing TaskRun YAML files
//   - Creating TaskRuns in the Kubernetes cluster
//   - Monitoring TaskRun execution status
//   - Streaming pod logs to files
//
// The Manager type is the primary interface for TaskRun operations and is used by the
// API handlers to execute the complete TaskRun workflow asynchronously.
//
// # Testing Strategy
//
// This package uses a hybrid testing approach:
//
//   - Unit tests: Pure functions like parseTaskRunYAML() are tested with standard unit tests
//   - Integration tests: The complete workflow (Manager.RunTaskRunWorkflow) is tested via
//     'make test-e2e' which runs against a real Kind cluster
//
// The Manager struct intentionally uses concrete Kubernetes client types (*Clientset)
// rather than interfaces because it's a thin orchestration layer with no business logic.
// Mock-based unit tests would require heavy mocking infrastructure for minimal value,
// as the code simply chains Kubernetes API calls. The end-to-end test provides better
// coverage by verifying the actual API interactions work correctly.
//
// This design follows the pattern used in the actual multi-platform-controller project,
// which uses controller-runtime's fake client only for testing reconciliation business
// logic, not for API wrapper utilities like this Manager.
package taskrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	tektonscheme "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/scheme"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	namespace = "multi-platform-controller"
)

var scheme = runtime.NewScheme()

func init() {
	_ = tektonscheme.AddToScheme(scheme)
}

// Manager handles all TaskRun operations using Tekton and Kubernetes clients.
//
// It maintains both a Tekton clientset (for TaskRun API operations) and a Kubernetes
// clientset (for pod log streaming). Both clients are configured from the default
// kubeconfig location (~/.kube/config).
type Manager struct {
	tektonClient *tektonclient.Clientset
	k8sClient    *kubernetes.Clientset
}

// NewManager creates a new TaskRun manager configured with Tekton and Kubernetes clients.
//
// The kubeconfig is loaded from ~/.kube/config. Returns an error if the kubeconfig
// cannot be loaded or if client creation fails.
func NewManager() (*Manager, error) {
	kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	tektonClient, err := tektonclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Tekton client: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &Manager{
		tektonClient: tektonClient,
		k8sClient:    k8sClient,
	}, nil
}

// RunTaskRunWorkflow runs the complete TaskRun workflow from start to finish.
//
// This method orchestrates the entire TaskRun lifecycle:
//  1. Parse and validate TaskRun YAML file
//  2. Create TaskRun in the Kubernetes cluster
//  3. Launch async goroutine to stream logs to file
//  4. Monitor TaskRun status until completion (success/failure)
//  5. Return final status
//
// The log streaming happens asynchronously to avoid blocking status monitoring.
// This ensures we can track TaskRun progress while simultaneously capturing all logs.
//
// Returns the TaskRun name, final status ("Succeeded", "Failed", "Timeout"), and any error.
func (m *Manager) RunTaskRunWorkflow(ctx context.Context, yamlPath, logFilePath string) (name, status string, err error) {
	// Step 1: Parse YAML
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read TaskRun file: %w", err)
	}

	taskRun, err := m.parseTaskRunYAML(data)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse TaskRun YAML: %w", err)
	}

	// Step 2: Apply TaskRun
	result, err := m.tektonClient.TektonV1().TaskRuns(namespace).Create(ctx, taskRun, metav1.CreateOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to create TaskRun: %w", err)
	}

	name = result.Name

	// Step 3: Wait for pod to be created and stream logs
	go m.streamLogsAsync(ctx, name, logFilePath)

	// Step 4: Monitor TaskRun until completion
	status, err = m.monitorTaskRun(ctx, name)
	if err != nil {
		return name, "", fmt.Errorf("failed to monitor TaskRun: %w", err)
	}

	// Give log streaming a moment to finish
	time.Sleep(2 * time.Second)

	return name, status, nil
}

// monitorTaskRun monitors a TaskRun until it completes.
//
// This method polls the TaskRun status every 5 seconds, checking the Tekton condition
// to determine if the TaskRun has succeeded, failed, or is still running. It has a
// 30-minute timeout to prevent indefinite waiting.
//
// Returns "Succeeded", "Failed", or "Timeout" along with any error encountered.
func (m *Manager) monitorTaskRun(ctx context.Context, name string) (string, error) {
	timeout := time.After(30 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return "Timeout", errors.New("TaskRun monitoring timed out after 30 minutes")
		case <-ticker.C:
			taskRun, err := m.tektonClient.TektonV1().TaskRuns(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue
			}

			if len(taskRun.Status.Conditions) == 0 {
				continue
			}

			condition := taskRun.Status.Conditions[0]

			// Check if completed
			if condition.Status == corev1.ConditionTrue && condition.Type == "Succeeded" {
				return "Succeeded", nil
			} else if condition.Status == corev1.ConditionFalse {
				return "Failed", nil
			}

			// Still running
		}
	}
}

// streamLogsAsync streams logs to a file asynchronously in a goroutine.
//
// This method:
//  1. Waits for the TaskRun's pod to be created and enter Running state
//  2. Creates the log file at the specified path
//  3. Streams logs from all containers in the pod to the file
//
// Any errors are printed to stdout but don't stop the workflow, since log streaming
// is supplementary to TaskRun monitoring.
func (m *Manager) streamLogsAsync(ctx context.Context, taskRunName, logFilePath string) {
	// Wait for pod to be created
	pod, err := m.waitForTaskRunPod(ctx, taskRunName, 5*time.Minute)
	if err != nil {
		fmt.Printf("Failed to wait for TaskRun pod: %v\n", err)
		return
	}

	// Create log file
	logFile, err := os.Create(logFilePath)
	if err != nil {
		fmt.Printf("Failed to create log file: %v\n", err)
		return
	}
	defer func() {
		_ = logFile.Close()
	}()

	// Stream logs from all containers
	for _, container := range pod.Spec.Containers {
		if err := m.streamContainerLogs(ctx, pod.Name, container.Name, logFile); err != nil {
			fmt.Printf("Warning: failed to stream logs from container %s: %v\n", container.Name, err)
		}
	}
}

// waitForTaskRunPod waits for the TaskRun's pod to be created AND running.
//
// This method polls the Kubernetes API every 2 seconds looking for a pod with the label
// "tekton.dev/taskRun=<taskRunName>". It only returns when the pod reaches Running phase,
// not just when it's created, to avoid log streaming errors from pods in Initializing state.
//
// Has a configurable timeout (typically 5 minutes).
func (m *Manager) waitForTaskRunPod(ctx context.Context, taskRunName string, timeout time.Duration) (*corev1.Pod, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return nil, errors.New("timeout waiting for TaskRun pod")
		case <-ticker.C:
			pods, err := m.k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "tekton.dev/taskRun=" + taskRunName,
			})
			if err != nil {
				continue
			}

			if len(pods.Items) > 0 {
				pod := &pods.Items[0]
				// Wait for pod to be running before streaming logs
				if pod.Status.Phase == corev1.PodRunning {
					return pod, nil
				}
				// Continue waiting if pod is still initializing
			}
		}
	}
}

// streamContainerLogs streams logs from a specific container to a file.
//
// This method uses the Kubernetes Pods().GetLogs() API with Follow=true to stream
// logs in real-time. It blocks until the container finishes or the context is cancelled.
//
// The logs are written directly to the provided io.Writer (typically a file).
func (m *Manager) streamContainerLogs(ctx context.Context, podName, containerName string, logFile io.Writer) error {
	req := m.k8sClient.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
		Follow:    true,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to get log stream: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	_, err = io.Copy(logFile, stream)
	return err
}

// parseTaskRunYAML parses YAML data into a Tekton TaskRun object.
//
// This method uses the Tekton scheme's universal deserializer to parse the YAML
// and validate that it contains a valid TaskRun resource. Returns an error if
// the YAML is invalid or doesn't contain a TaskRun.
func (m *Manager) parseTaskRunYAML(data []byte) (*tektonv1.TaskRun, error) {
	decode := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode
	obj, _, err := decode(data, nil, nil)
	if err != nil {
		return nil, err
	}

	taskRun, ok := obj.(*tektonv1.TaskRun)
	if !ok {
		return nil, errors.New("YAML does not contain a TaskRun")
	}

	return taskRun, nil
}
