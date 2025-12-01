package com.redhat.mpcdev.ui

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.service
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBPanel
import com.intellij.ui.components.JBScrollPane
import com.redhat.mpcdev.exceptions.BackendConnectionException
import com.redhat.mpcdev.exceptions.BackendDataException
import com.redhat.mpcdev.services.BackendClient
import com.redhat.mpcdev.ui.panels.*
import kotlinx.coroutines.*
import java.awt.BorderLayout
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import javax.swing.BoxLayout
import javax.swing.JButton
import javax.swing.JPanel

/**
 * Main content for the MPC Dev Studio tool window.
 *
 * Manages different UI states:
 * 1. Backend not running
 * 2. Backend running, no environment
 * 3. Environment paused
 * 4. Environment running (full dashboard)
 *
 * NOTE: This implementation fetches data on-demand rather than maintaining a cache.
 * All UI refreshes call BackendClient.getStatus() directly to get the latest state.
 * There is no periodic polling - the UI only updates when explicitly triggered by
 * user actions or when a view becomes visible.
 */
class MPCToolWindowContent(
    private val project: Project,
    private val toolWindow: ToolWindow
) {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    private val backendClient = service<BackendClient>()

    private val mainPanel = JBPanel<JBPanel<*>>(BorderLayout())
    private var currentStatePanel: javax.swing.JComponent? = null

    // Dashboard panels (created when environment is running)
    private var statusPanel: StatusPanel? = null
    private var repositoryPanel: RepositoryPanel? = null
    private var featuresPanel: FeaturesPanel? = null

    // Polling for operation status
    private var pollingJob: kotlinx.coroutines.Job? = null

    init {
        refreshUI()
    }

    fun getContent(): JPanel = mainPanel

    /**
     * Refresh the entire UI by fetching fresh data from the backend.
     * This is the main entry point for all UI updates.
     */
    fun refreshUI() {
        scope.launch {
            try {
                val status = backendClient.getStatus()

                withContext(Dispatchers.Main) {
                    // Backend is running, determine environment state
                    val sessionId = status.sessionId
                    if (sessionId == "not_initialized") {
                        showNoEnvironmentState()
                    } else {
                        // Check if cluster is running
                        val clusterRunning = status.cluster.status == "running"
                        if (clusterRunning) {
                            showRunningState()
                        } else {
                            showPausedState()
                        }
                    }
                }
            } catch (e: BackendConnectionException) {
                // Backend is not running or not reachable
                withContext(Dispatchers.Main) {
                    showBackendNotRunningState()
                }
            } catch (e: BackendDataException) {
                // Backend sent invalid data
                withContext(Dispatchers.Main) {
                    showInvalidDataState()
                }
            } catch (e: Exception) {
                // Unexpected error - treat as backend not running
                withContext(Dispatchers.Main) {
                    showBackendNotRunningState()
                }
            }
        }
    }

    /**
     * State 1: Backend daemon is not running.
     */
    private fun showBackendNotRunningState() {
        val panel = JBPanel<JBPanel<*>>(GridBagLayout())
        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = java.awt.Insets(10, 10, 10, 10)

        panel.add(JBLabel("⚠️ Backend Daemon Not Running"), gbc)
        panel.add(JBLabel("<html>The MPC Dev Studio backend<br>daemon is not running.</html>"), gbc)

        val startButton = JButton("Start Daemon")
        startButton.addActionListener {
            startDaemon()
        }
        panel.add(startButton, gbc)

        panel.add(JBLabel("<html><br>Or start manually:<br>$ mpc-daemon</html>"), gbc)

        setContentPanel(panel)
    }

    /**
     * State: Backend sent invalid/unparseable data.
     */
    private fun showInvalidDataState() {
        val panel = JBPanel<JBPanel<*>>(GridBagLayout())
        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = java.awt.Insets(10, 10, 10, 10)

        panel.add(JBLabel("⚠️ Invalid Backend Data"), gbc)
        panel.add(JBLabel("<html>Received invalid data from backend.<br>The backend may be misconfigured.</html>"), gbc)

        val refreshButton = JButton("Retry")
        refreshButton.addActionListener {
            refreshUI()
        }
        panel.add(refreshButton, gbc)

        setContentPanel(panel)
    }

    /**
     * State 2: Backend running, but no environment exists.
     */
    private fun showNoEnvironmentState() {
        val panel = JBPanel<JBPanel<*>>(GridBagLayout())
        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = java.awt.Insets(10, 10, 10, 10)

        panel.add(JBLabel("No Environment Running"), gbc)
        panel.add(JBLabel("<html>Create a new MPC development<br>environment to get started.</html>"), gbc)

        val startButton = JButton("Start New Environment")
        startButton.addActionListener {
            startNewEnvironment()
        }
        panel.add(startButton, gbc)

        setContentPanel(panel)
    }

    /**
     * State 3: Environment exists but is paused.
     */
    private fun showPausedState() {
        val panel = JBPanel<JBPanel<*>>(GridBagLayout())
        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = java.awt.Insets(10, 10, 10, 10)

        panel.add(JBLabel("Environment Paused"), gbc)
        panel.add(JBLabel("<html>Cluster is not running.<br>Resume to continue working.</html>"), gbc)

        val resumeButton = JButton("Resume Environment")
        resumeButton.addActionListener {
            resumeEnvironment()
        }
        panel.add(resumeButton, gbc)

        setContentPanel(panel)
    }

    /**
     * State 4: Environment is running - show full dashboard.
     */
    private fun showRunningState() {
        val dashboardPanel = JBPanel<JBPanel<*>>()
        dashboardPanel.layout = BoxLayout(dashboardPanel, BoxLayout.Y_AXIS)

        // Create panels - they will fetch their own data when needed
        statusPanel = StatusPanel(backendClient)
        repositoryPanel = RepositoryPanel(backendClient)
        featuresPanel = FeaturesPanel(backendClient, project)

        // Add panels to dashboard
        dashboardPanel.add(statusPanel)
        dashboardPanel.add(createSeparator())
        dashboardPanel.add(repositoryPanel)
        dashboardPanel.add(createSeparator())
        dashboardPanel.add(featuresPanel)

        // Wrap in scroll pane
        val scrollPane = JBScrollPane(dashboardPanel)
        setContentPanel(scrollPane)

        // Trigger initial data load for all panels
        scope.launch {
            try {
                val status = backendClient.getStatus()
                withContext(Dispatchers.Main) {
                    statusPanel?.updateStatus(status)
                    repositoryPanel?.updateRepositories(status)
                    featuresPanel?.updateFeatures(status)
                }

                // Start polling if there's an active operation
                if (status.operationStatus != null && status.operationStatus != "idle") {
                    startPollingForOperationCompletion()
                }
            } catch (e: BackendConnectionException) {
                // Backend connection lost - switch to error state
                withContext(Dispatchers.Main) {
                    showBackendNotRunningState()
                }
            } catch (e: BackendDataException) {
                // Invalid data - switch to error state
                withContext(Dispatchers.Main) {
                    showInvalidDataState()
                }
            } catch (e: Exception) {
                // Unexpected error - silently fail, panels will handle empty state
            }
        }
    }

    /**
     * Start polling the API for operation status updates.
     * Continues polling while operation_status is active (not "idle").
     */
    private fun startPollingForOperationCompletion() {
        // Cancel existing polling job if any
        pollingJob?.cancel()

        pollingJob = scope.launch {
            while (true) {
                try {
                    kotlinx.coroutines.delay(2000) // Poll every 2 seconds

                    val status = backendClient.getStatus()

                    // Update UI with latest status
                    withContext(Dispatchers.Main) {
                        statusPanel?.updateStatus(status)
                        repositoryPanel?.updateRepositories(status)
                        featuresPanel?.updateFeatures(status)
                    }

                    // Stop polling if operation is idle or null
                    if (status.operationStatus == null || status.operationStatus == "idle") {
                        // Show notification if there was an error
                        if (status.lastOperationError != null) {
                            withContext(Dispatchers.Main) {
                                NotificationGroupManager.getInstance()
                                    .getNotificationGroup("MPC Dev Studio")
                                    .createNotification(
                                        "Operation failed: ${status.lastOperationError}",
                                        NotificationType.ERROR
                                    )
                                    .notify(project)
                            }
                        }
                        break
                    }
                } catch (e: Exception) {
                    // Connection error - stop polling
                    break
                }
            }
        }
    }

    private fun createSeparator(): JPanel {
        val separator = JBPanel<JBPanel<*>>()
        separator.preferredSize = java.awt.Dimension(0, 1)
        separator.background = java.awt.Color.GRAY
        return separator
    }

    private fun setContentPanel(panel: javax.swing.JComponent) {
        ApplicationManager.getApplication().invokeLater {
            currentStatePanel?.let { mainPanel.remove(it) }
            currentStatePanel = panel
            mainPanel.add(panel, BorderLayout.CENTER)
            mainPanel.revalidate()
            mainPanel.repaint()
        }
    }

    /**
     * Poll the backend until it becomes ready or timeout is reached.
     *
     * @param timeoutMillis Maximum time to wait for backend to become ready
     * @return true if backend became ready, false if timeout was reached
     */
    private suspend fun pollForBackendReady(timeoutMillis: Long): Boolean {
        val startTime = System.currentTimeMillis()
        while (System.currentTimeMillis() - startTime < timeoutMillis) {
            try {
                // Try to get status from backend
                backendClient.getStatus()
                // If we get here without exception, backend is ready
                return true
            } catch (e: BackendConnectionException) {
                // Backend not ready yet, continue polling
                delay(500)
            } catch (e: BackendDataException) {
                // Backend is responding but with invalid data - consider it ready
                // (the error will be handled by refreshUI)
                return true
            } catch (e: Exception) {
                // Other unexpected errors - continue polling
                delay(500)
            }
        }
        // Timeout reached
        return false
    }

    private fun startDaemon() {
        scope.launch {
            try {
                // Execute mpc-daemon directly (Go binary) instead of Python wrapper
                val process = ProcessBuilder("mpc-daemon")
                    .redirectErrorStream(true)
                    .start()

                // Poll for backend readiness instead of fixed delay
                val ready = pollForBackendReady(timeoutMillis = 15000)

                withContext(Dispatchers.Main) {
                    if (ready) {
                        // Backend is ready, refresh UI to show current state
                        refreshUI()
                    } else {
                        // Timeout reached - show error notification
                        NotificationGroupManager.getInstance()
                            .getNotificationGroup("MPC Dev Studio")
                            .createNotification(
                                "Backend daemon failed to start within 15 seconds",
                                NotificationType.ERROR
                            )
                            .notify(project)
                    }
                }
            } catch (e: Exception) {
                // Show error notification
                withContext(Dispatchers.Main) {
                    NotificationGroupManager.getInstance()
                        .getNotificationGroup("MPC Dev Studio")
                        .createNotification("Failed to start daemon: ${e.message}", NotificationType.ERROR)
                        .notify(project)
                }
            }
        }
    }

    private fun startNewEnvironment() {
        scope.launch {
            try {
                // TODO: Re-implement when Go API has startEnvironment endpoint
                // For now, show notification that this is not yet implemented
                withContext(Dispatchers.Main) {
                    NotificationGroupManager.getInstance()
                        .getNotificationGroup("MPC Dev Studio")
                        .createNotification(
                            "Start environment functionality will be available when Go API is expanded",
                            NotificationType.WARNING
                        )
                        .notify(project)
                }
            } catch (e: Exception) {
                // Show error notification
                withContext(Dispatchers.Main) {
                    NotificationGroupManager.getInstance()
                        .getNotificationGroup("MPC Dev Studio")
                        .createNotification("Failed to start environment: ${e.message}", NotificationType.ERROR)
                        .notify(project)
                }
            }
        }
    }

    private fun resumeEnvironment() {
        scope.launch {
            try {
                // TODO: Re-implement when Go API has resumeEnvironment endpoint
                // For now, show notification that this is not yet implemented
                withContext(Dispatchers.Main) {
                    NotificationGroupManager.getInstance()
                        .getNotificationGroup("MPC Dev Studio")
                        .createNotification(
                            "Resume environment functionality will be available when Go API is expanded",
                            NotificationType.WARNING
                        )
                        .notify(project)
                }
            } catch (e: Exception) {
                // Show error notification
                withContext(Dispatchers.Main) {
                    NotificationGroupManager.getInstance()
                        .getNotificationGroup("MPC Dev Studio")
                        .createNotification("Failed to resume environment: ${e.message}", NotificationType.ERROR)
                        .notify(project)
                }
            }
        }
    }

    fun dispose() {
        pollingJob?.cancel()
        scope.cancel()
    }
}
