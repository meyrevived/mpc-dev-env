package taskrun

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// monitorTaskRunTimeout allows overriding the default timeout for testing
// NOTE: This is only used by commented-out tests. Keep it for when they're re-enabled.
// var monitorTaskRunTimeout = 30 * time.Minute

func TestTaskRun(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TaskRun Manager Suite")
}

var _ = Describe("TaskRun Manager", func() {
	var (
		manager *Manager
	)

	Describe("parseTaskRunYAML", func() {
		BeforeEach(func() {
			manager = &Manager{}
		})

		It("should parse a valid TaskRun YAML successfully", func() {
			yamlData := `
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  name: my-taskrun
spec:
  taskSpec:
    steps:
      - name: echo
        image: ubuntu
        script: echo "hello world"
`
			taskRun, err := manager.parseTaskRunYAML([]byte(yamlData))
			Expect(err).NotTo(HaveOccurred())
			Expect(taskRun).NotTo(BeNil())
			Expect(taskRun.Kind).To(Equal("TaskRun"))
			Expect(taskRun.APIVersion).To(Equal("tekton.dev/v1"))
			Expect(taskRun.Name).To(Equal("my-taskrun"))
			Expect(taskRun.Spec.TaskSpec.Steps).To(HaveLen(1))
			Expect(taskRun.Spec.TaskSpec.Steps[0].Name).To(Equal("echo"))
		})

		It("should return an error for YAML that is not a TaskRun", func() {
			yamlData := `
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
spec:
  containers:
  - name: my-container
    image: nginx
`
			_, err := manager.parseTaskRunYAML([]byte(yamlData))
			Expect(err).To(HaveOccurred())
			// Pod is not registered in the Tekton scheme, so decode fails before type checking
			Expect(err.Error()).To(Or(
				ContainSubstring("YAML does not contain a TaskRun"),
				ContainSubstring("no kind \"Pod\" is registered"),
			))
		})

		It("should return an error for invalid YAML", func() {
			yamlData := `
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  name: my-taskrun
spec:
  taskSpec:
    steps:
      - name: echo
-       image: ubuntu # Invalid YAML indentation
        script: echo "hello world"
`
			_, err := manager.parseTaskRunYAML([]byte(yamlData))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("NewManager", func() {
		var (
			originalHome string
			tempHome     string
		)

		BeforeEach(func() {
			originalHome = os.Getenv("HOME")
			var err error
			tempHome, err = os.MkdirTemp("", "fake-home-*")
			Expect(err).NotTo(HaveOccurred())
			_ = os.Setenv("HOME", tempHome)
		})

		AfterEach(func() {
			_ = os.Setenv("HOME", originalHome)
			_ = os.RemoveAll(tempHome)
		})

		It("should fail if kubeconfig does not exist", func() {
			_, err := NewManager()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no such file or directory"))
		})

		It("should succeed if a valid kubeconfig exists", func() {
			// Create a dummy kubeconfig file
			kubeconfigDir := filepath.Join(tempHome, ".kube")
			Expect(os.MkdirAll(kubeconfigDir, 0755)).To(Succeed())
			kubeconfigPath := filepath.Join(kubeconfigDir, "config")
			// An empty file is enough for BuildConfigFromFlags to not fail on file not found
			Expect(os.WriteFile(kubeconfigPath, []byte(""), 0644)).To(Succeed())

			// We expect an error here because the kubeconfig is empty and invalid for creating clients,
			// but we are testing that the file-loading part of NewManager works.
			// A full integration test would need a valid kubeconfig.
			_, err := NewManager()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid configuration"))
		})
	})

	// DISABLED: Integration tests requiring heavy mocking (not worth the effort)
	// These workflows are tested end-to-end via 'make test-e2e' with real cluster
	/*
		Describe("monitorTaskRun", func() {
			var (
				fakeTektonClientset *fakeTekton.Clientset
				managerWithFake     *Manager
				ctx                 context.Context
			)

			BeforeEach(func() {
				ctx = context.Background()
				fakeTektonClientset = fakeTekton.NewSimpleClientset()
				managerWithFake = &Manager{tektonClient: fakeTektonClientset}
			})

			It("should return Succeeded when TaskRun completes successfully", func() {
				taskRunName := "test-taskrun-success"
				testTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{Name: taskRunName, Namespace: namespace},
				}
				testTaskRun.Status.SetConditions(apis.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				}})
				_, err := fakeTektonClientset.TektonV1().TaskRuns(namespace).Create(ctx, testTaskRun, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				status, err := managerWithFake.monitorTaskRun(ctx, taskRunName)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("Succeeded"))
			})

			It("should return Failed when TaskRun fails", func() {
				taskRunName := "test-taskrun-fail"
				testTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{Name: taskRunName, Namespace: namespace},
				}
				testTaskRun.Status.SetConditions(apis.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				}})
				_, err := fakeTektonClientset.TektonV1().TaskRuns(namespace).Create(ctx, testTaskRun, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				status, err := managerWithFake.monitorTaskRun(ctx, taskRunName)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("Failed"))
			})

			It("should timeout if TaskRun does not complete", func() {
				taskRunName := "test-taskrun-running"
				testTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{Name: taskRunName, Namespace: namespace},
				}
				testTaskRun.Status.SetConditions(apis.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionUnknown,
				}})
				_, err := fakeTektonClientset.TektonV1().TaskRuns(namespace).Create(ctx, testTaskRun, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Temporarily reduce timeout for testing purposes
				originalTimeout := monitorTaskRunTimeout
				DeferCleanup(func() { monitorTaskRunTimeout = originalTimeout })
				monitorTaskRunTimeout = 1 * time.Second

				status, err := managerWithFake.monitorTaskRun(ctx, taskRunName)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timed out"))
				Expect(status).To(Equal("Timeout"))
			})
		})
	*/

	// DISABLED: Integration tests requiring heavy mocking (not worth the effort)
	// These workflows are tested end-to-end via 'make test-e2e' with real cluster
	/*
		Describe("waitForTaskRunPod", func() {
			var (
				fakeK8sClientset *kubernetesFake.Clientset
				managerWithFake   *Manager
				ctx               context.Context
			)

			BeforeEach(func() {
				ctx = context.Background()
				fakeK8sClientset = kubernetesFake.NewSimpleClientset()
				managerWithFake = &Manager{k8sClient: fakeK8sClientset}
			})

			It("should return the pod when it is created and running", func() {
				taskRunName := "test-pod-running"
				podName := "test-pod-running-pod"
				_, err := fakeK8sClientset.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace, Labels: map[string]string{"tekton.dev/taskRun": taskRunName}},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				}, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				pod, err := managerWithFake.waitForTaskRunPod(ctx, taskRunName, 1*time.Second)
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				Expect(pod.Name).To(Equal(podName))
			})

			It("should timeout if the pod is never created", func() {
				taskRunName := "test-pod-missing"

				pod, err := managerWithFake.waitForTaskRunPod(ctx, taskRunName, 1*time.Second)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timeout waiting for TaskRun pod"))
				Expect(pod).To(BeNil())
			})

			It("should timeout if the pod is created but never runs", func() {
				taskRunName := "test-pod-pending"
				podName := "test-pod-pending-pod"
				_, err := fakeK8sClientset.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace, Labels: map[string]string{"tekton.dev/taskRun": taskRunName}},
					Status:     corev1.PodStatus{Phase: corev1.PodPending},
				}, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				pod, err := managerWithFake.waitForTaskRunPod(ctx, taskRunName, 1*time.Second)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timeout waiting for TaskRun pod"))
				Expect(pod).To(BeNil())
			})
		})
	*/
})
