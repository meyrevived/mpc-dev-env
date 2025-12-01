package com.redhat.mpcdev.ui.panels

import com.intellij.openapi.application.ApplicationManager
import com.redhat.mpcdev.model.*
import com.redhat.mpcdev.services.BackendClient
import io.mockk.every
import io.mockk.mockk
import io.mockk.mockkStatic
import io.mockk.unmockkAll
import org.junit.After
import org.junit.Before
import org.junit.Test
import javax.swing.JLabel
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Comprehensive test suite for StatusPanel.
 *
 * Tests validate:
 * - Panel correctly populates UI components from DevEnvironment data class
 * - Panel correctly reflects different cluster states (running, paused, stopped)
 * - Panel correctly shows MPC deployment status
 * - All UI updates happen on EDT
 */
class StatusPanelTest {

    private lateinit var mockBackendClient: BackendClient
    private lateinit var panel: StatusPanel

    @Before
    fun setup() {
        mockBackendClient = mockk(relaxed = true)

        // Mock ApplicationManager to avoid EDT requirements during testing
        mockkStatic(ApplicationManager::class)
        val mockApp = mockk<com.intellij.openapi.application.Application>(relaxed = true)
        every { ApplicationManager.getApplication() } returns mockApp
        every { mockApp.invokeLater(any()) } answers {
            // Execute immediately in tests instead of scheduling on EDT
            firstArg<Runnable>().run()
        }

        panel = StatusPanel(mockBackendClient)
    }

    @After
    fun tearDown() {
        unmockkAll()
    }

    // ======================
    // Initial State Tests
    // ======================

    @Test
    fun `panel should show loading state initially`() {
        // Verify initial "Loading..." text is set
        val components = panel.components
        assertTrue(components.size >= 3, "Panel should have at least 3 labels")

        // Find the labels
        val labels = components.filterIsInstance<JLabel>()
        assertTrue(labels.any { it.text.contains("Loading") }, "Should show Loading state")
    }

    // ======================
    // Cluster Running State Tests
    // ======================

    @Test
    fun `updateStatus should show running cluster correctly`() {
        val env = DevEnvironment(
            sessionId = "test-session-123",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "konflux-mpc-debug",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/home/user/.kube/config",
                konfluxDeployed = true
            ),
            repositories = emptyMap(),
            mpcDeployment = MpcDeployment(
                controllerImage = "localhost:5001/controller:debug",
                otpImage = "localhost:5001/otp:debug",
                deployedAt = "2024-01-15T11:00:00Z",
                sourceGitHash = "abc123"
            ),
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        // Get the labels
        val labels = panel.components.filterIsInstance<JLabel>()

        // Verify cluster label shows running state with green color
        val clusterLabel = labels[0]
        assertTrue(clusterLabel.text.contains("konflux-mpc-debug"), "Should show cluster name")
        assertTrue(clusterLabel.text.contains("running"), "Should show running status")
        assertTrue(clusterLabel.text.contains("green"), "Should use green color for running")
        assertTrue(clusterLabel.text.contains("●"), "Should show filled circle for running")

        // Verify MPC label shows deployed state
        val mpcLabel = labels[1]
        assertTrue(mpcLabel.text.contains("Deployed"), "Should show deployed status")
        assertTrue(mpcLabel.text.contains("green"), "Should use green color for deployed")
        assertTrue(mpcLabel.text.contains("●"), "Should show filled circle for deployed")

        // Verify session label
        val sessionLabel = labels[2]
        assertTrue(sessionLabel.text.contains("test-ses"), "Should show first 8 chars of session ID")
    }

    @Test
    fun `updateStatus should show paused cluster correctly`() {
        val env = DevEnvironment(
            sessionId = "session-456",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "paused",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = emptyMap(),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        val labels = panel.components.filterIsInstance<JLabel>()

        // Verify cluster label shows paused state
        val clusterLabel = labels[0]
        assertTrue(clusterLabel.text.contains("test-cluster"), "Should show cluster name")
        assertTrue(clusterLabel.text.contains("paused"), "Should show paused status")
        assertTrue(clusterLabel.text.contains("gray"), "Should use gray color for not running")
        assertTrue(clusterLabel.text.contains("○"), "Should show empty circle for not running")
    }

    @Test
    fun `updateStatus should show stopped cluster correctly`() {
        val env = DevEnvironment(
            sessionId = "session-789",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "stopped-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "stopped",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = false
            ),
            repositories = emptyMap(),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        val labels = panel.components.filterIsInstance<JLabel>()

        // Verify cluster label shows stopped state
        val clusterLabel = labels[0]
        assertTrue(clusterLabel.text.contains("stopped-cluster"), "Should show cluster name")
        assertTrue(clusterLabel.text.contains("stopped"), "Should show stopped status")
        assertTrue(clusterLabel.text.contains("gray"), "Should use gray color for stopped")
        assertTrue(clusterLabel.text.contains("○"), "Should show empty circle for stopped")
    }

    // ======================
    // MPC Deployment Tests
    // ======================

    @Test
    fun `updateStatus should show MPC deployed when mpcDeployment is not null`() {
        val env = DevEnvironment(
            sessionId = "session-mpc",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = emptyMap(),
            mpcDeployment = MpcDeployment(
                controllerImage = "localhost:5001/controller:v1.0",
                otpImage = "localhost:5001/otp:v1.0",
                deployedAt = "2024-01-15T11:00:00Z",
                sourceGitHash = "def456"
            ),
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val mpcLabel = labels[1]

        assertTrue(mpcLabel.text.contains("Deployed"), "Should show deployed")
        assertTrue(mpcLabel.text.contains("green"), "Should use green color")
        assertTrue(mpcLabel.text.contains("●"), "Should show filled circle")
    }

    @Test
    fun `updateStatus should show MPC not deployed when mpcDeployment is null`() {
        val env = DevEnvironment(
            sessionId = "session-no-mpc",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = emptyMap(),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val mpcLabel = labels[1]

        assertTrue(mpcLabel.text.contains("Not deployed"), "Should show not deployed")
        assertTrue(mpcLabel.text.contains("gray"), "Should use gray color")
        assertTrue(mpcLabel.text.contains("○"), "Should show empty circle")
    }

    // ======================
    // Session ID Tests
    // ======================

    @Test
    fun `updateStatus should display truncated session ID`() {
        val env = DevEnvironment(
            sessionId = "very-long-session-id-123456789",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = emptyMap(),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val sessionLabel = labels[2]

        // Should show only first 8 characters
        assertTrue(sessionLabel.text.contains("very-lon"), "Should show first 8 chars")
        assertTrue(sessionLabel.text.contains("Session:"), "Should have Session: prefix")
    }

    @Test
    fun `updateStatus should handle short session IDs`() {
        val env = DevEnvironment(
            sessionId = "short",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = emptyMap(),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val sessionLabel = labels[2]

        // Should show the entire short session ID
        assertTrue(sessionLabel.text.contains("short"), "Should show full short ID")
    }

    // ======================
    // EDT Tests
    // ======================

    @Test
    fun `updateStatus should use invokeLater for UI updates`() {
        var invokeWasCalled = false

        // Override the mock to track invokeLater calls
        every { ApplicationManager.getApplication().invokeLater(any()) } answers {
            invokeWasCalled = true
            firstArg<Runnable>().run()
        }

        val env = DevEnvironment(
            sessionId = "test",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test",
                konfluxDeployed = true
            ),
            repositories = emptyMap(),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateStatus(env)

        assertTrue(invokeWasCalled, "Should call invokeLater to update UI on EDT")
    }
}
