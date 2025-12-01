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
import kotlin.test.assertTrue

/**
 * Comprehensive test suite for RepositoryPanel.
 *
 * Tests validate:
 * - Panel correctly populates repository information from DevEnvironment data class
 * - Panel correctly displays branch names
 * - Panel correctly shows upstream commits ahead status
 * - Panel correctly shows local changes indicator
 * - All UI updates happen on EDT
 */
class RepositoryPanelTest {

    private lateinit var mockBackendClient: BackendClient
    private lateinit var panel: RepositoryPanel

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

        panel = RepositoryPanel(mockBackendClient)
    }

    @After
    fun tearDown() {
        unmockkAll()
    }

    // ======================
    // Initial State Tests
    // ======================

    @Test
    fun `panel should show repository names initially`() {
        val labels = panel.components.filterIsInstance<JLabel>()

        assertTrue(labels.any { it.text.contains("multi-platform-controller") }, "Should show MPC repo")
        assertTrue(labels.any { it.text.contains("konflux-ci") }, "Should show Konflux repo")
        assertTrue(labels.any { it.text.contains("infra-deployments") }, "Should show infra repo")
    }

    // ======================
    // Repository Update Tests - Clean State
    // ======================

    @Test
    fun `updateRepositories should show clean repository state`() {
        val env = DevEnvironment(
            sessionId = "test-session",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = mapOf(
                "multi-platform-controller" to RepositoryState(
                    name = "multi-platform-controller",
                    path = "/home/user/mpc",
                    currentBranch = "main",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 0,
                    hasLocalChanges = false
                )
            ),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateRepositories(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val mpcLabel = labels[0]

        assertTrue(mpcLabel.text.contains("main"), "Should show branch name")
        assertTrue(mpcLabel.text.contains("✓"), "Should show checkmark for clean state")
    }

    @Test
    fun `updateRepositories should show repository behind upstream`() {
        val env = DevEnvironment(
            sessionId = "test-session",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = mapOf(
                "multi-platform-controller" to RepositoryState(
                    name = "multi-platform-controller",
                    path = "/home/user/mpc",
                    currentBranch = "feature-branch",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 5,
                    hasLocalChanges = false
                )
            ),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateRepositories(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val mpcLabel = labels[0]

        assertTrue(mpcLabel.text.contains("feature-branch"), "Should show branch name")
        assertTrue(mpcLabel.text.contains("5 behind"), "Should show commits behind")
        assertTrue(mpcLabel.text.contains("⚠️"), "Should show warning icon")
    }

    @Test
    fun `updateRepositories should show repository with local changes`() {
        val env = DevEnvironment(
            sessionId = "test-session",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = mapOf(
                "multi-platform-controller" to RepositoryState(
                    name = "multi-platform-controller",
                    path = "/home/user/mpc",
                    currentBranch = "dev",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 0,
                    hasLocalChanges = true
                )
            ),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateRepositories(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val mpcLabel = labels[0]

        assertTrue(mpcLabel.text.contains("dev"), "Should show branch name")
        assertTrue(mpcLabel.text.contains("local changes"), "Should show local changes indicator")
        assertTrue(mpcLabel.text.contains("●"), "Should show dot for local changes")
    }

    @Test
    fun `updateRepositories should show repository behind upstream with local changes`() {
        val env = DevEnvironment(
            sessionId = "test-session",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = mapOf(
                "multi-platform-controller" to RepositoryState(
                    name = "multi-platform-controller",
                    path = "/home/user/mpc",
                    currentBranch = "experimental",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 3,
                    hasLocalChanges = true
                )
            ),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateRepositories(env)

        val labels = panel.components.filterIsInstance<JLabel>()
        val mpcLabel = labels[0]

        assertTrue(mpcLabel.text.contains("experimental"), "Should show branch name")
        assertTrue(mpcLabel.text.contains("3 behind"), "Should show commits behind")
        assertTrue(mpcLabel.text.contains("local changes"), "Should show local changes")
        assertTrue(mpcLabel.text.contains("⚠️"), "Should show warning icon")
        assertTrue(mpcLabel.text.contains("●"), "Should show dot for local changes")
    }

    // ======================
    // Multiple Repository Tests
    // ======================

    @Test
    fun `updateRepositories should handle multiple repositories`() {
        val env = DevEnvironment(
            sessionId = "test-session",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = mapOf(
                "multi-platform-controller" to RepositoryState(
                    name = "multi-platform-controller",
                    path = "/home/user/mpc",
                    currentBranch = "main",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 0,
                    hasLocalChanges = false
                ),
                "konflux-ci" to RepositoryState(
                    name = "konflux-ci",
                    path = "/home/user/konflux",
                    currentBranch = "staging",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 2,
                    hasLocalChanges = false
                ),
                "infra-deployments" to RepositoryState(
                    name = "infra-deployments",
                    path = "/home/user/infra",
                    currentBranch = "production",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 0,
                    hasLocalChanges = true
                )
            ),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateRepositories(env)

        val labels = panel.components.filterIsInstance<JLabel>()

        // MPC repo - clean
        val mpcLabel = labels[0]
        assertTrue(mpcLabel.text.contains("main"), "MPC should show main branch")
        assertTrue(mpcLabel.text.contains("✓"), "MPC should show clean state")

        // Konflux repo - behind upstream
        val konfluxLabel = labels[1]
        assertTrue(konfluxLabel.text.contains("staging"), "Konflux should show staging branch")
        assertTrue(konfluxLabel.text.contains("2 behind"), "Konflux should show behind count")

        // Infra repo - local changes
        val infraLabel = labels[2]
        assertTrue(infraLabel.text.contains("production"), "Infra should show production branch")
        assertTrue(infraLabel.text.contains("local changes"), "Infra should show local changes")
    }

    // ======================
    // Missing Repository Tests
    // ======================

    @Test
    fun `updateRepositories should handle missing repositories gracefully`() {
        val env = DevEnvironment(
            sessionId = "test-session",
            createdAt = "2024-01-15T10:00:00Z",
            lastActive = "2024-01-15T14:00:00Z",
            cluster = ClusterState(
                name = "test-cluster",
                createdAt = "2024-01-15T10:00:00Z",
                status = "running",
                kubeconfigPath = "/test/kubeconfig",
                konfluxDeployed = true
            ),
            repositories = mapOf(
                "multi-platform-controller" to RepositoryState(
                    name = "multi-platform-controller",
                    path = "/home/user/mpc",
                    currentBranch = "main",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 0,
                    hasLocalChanges = false
                )
                // konflux-ci and infra-deployments are missing
            ),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        // Should not throw exception
        panel.updateRepositories(env)

        val labels = panel.components.filterIsInstance<JLabel>()

        // MPC repo should be updated
        val mpcLabel = labels[0]
        assertTrue(mpcLabel.text.contains("main"), "MPC should show main branch")

        // Other repos should keep their default text
        val konfluxLabel = labels[1]
        assertTrue(konfluxLabel.text.contains("konflux-ci"), "Konflux should show default text")

        val infraLabel = labels[2]
        assertTrue(infraLabel.text.contains("infra-deployments"), "Infra should show default text")
    }

    @Test
    fun `updateRepositories should handle empty repository map`() {
        val env = DevEnvironment(
            sessionId = "test-session",
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

        // Should not throw exception
        panel.updateRepositories(env)

        val labels = panel.components.filterIsInstance<JLabel>()

        // All repos should keep their default text
        assertTrue(labels[0].text.contains("multi-platform-controller"), "Should keep default MPC text")
        assertTrue(labels[1].text.contains("konflux-ci"), "Should keep default Konflux text")
        assertTrue(labels[2].text.contains("infra-deployments"), "Should keep default infra text")
    }

    // ======================
    // EDT Tests
    // ======================

    @Test
    fun `updateRepositories should use invokeLater for UI updates`() {
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
            repositories = mapOf(
                "multi-platform-controller" to RepositoryState(
                    name = "multi-platform-controller",
                    path = "/test",
                    currentBranch = "main",
                    lastSynced = "2024-01-15T10:00:00Z",
                    upstreamCommitsAhead = 0,
                    hasLocalChanges = false
                )
            ),
            mpcDeployment = null,
            features = FeatureState(awsEnabled = false, ibmEnabled = false)
        )

        panel.updateRepositories(env)

        assertTrue(invokeWasCalled, "Should call invokeLater to update UI on EDT")
    }
}
