package com.redhat.mpcdev.ui.panels

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.ui.components.JBCheckBox
import com.intellij.ui.components.JBPanel
import com.redhat.mpcdev.dialogs.CredentialDialog
import com.redhat.mpcdev.services.BackendClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import javax.swing.BorderFactory
import javax.swing.JButton

/**
 * Panel for enabling/disabling cloud provider features and running operations.
 *
 * Shows checkboxes for AWS and IBM Cloud integration, plus buttons for:
 * - Running smoke tests
 * - Deploying metrics stack
 */
class FeaturesPanel(
    private val backendClient: BackendClient,
    private val project: Project
) : JBPanel<FeaturesPanel>(GridBagLayout()) {

    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    private val awsCheckbox = JBCheckBox("AWS")
    private val ibmCheckbox = JBCheckBox("IBM Cloud")
    private val smokeTestButton = JButton("Run Smoke Test")
    private val metricsButton = JButton("Deploy Metrics")
    private var isUpdatingFromBackend = false

    private fun showNotification(message: String, type: NotificationType) {
        ApplicationManager.getApplication().invokeLater {
            NotificationGroupManager.getInstance()
                .getNotificationGroup("MPC Dev Studio")
                .createNotification(message, type)
                .notify(project)
        }
    }

    init {
        border = BorderFactory.createTitledBorder("Features & Operations")

        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.anchor = GridBagConstraints.WEST
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = java.awt.Insets(5, 5, 5, 5)

        add(awsCheckbox, gbc)
        add(ibmCheckbox, gbc)

        // Add separator
        gbc.insets = java.awt.Insets(15, 5, 5, 5)
        add(smokeTestButton, gbc)

        gbc.insets = java.awt.Insets(5, 5, 5, 5)
        add(metricsButton, gbc)

        // Add listeners for checkbox changes to enable/disable features
        awsCheckbox.addActionListener {
            if (!isUpdatingFromBackend) {
                toggleFeature("aws", awsCheckbox.isSelected)
            }
        }

        ibmCheckbox.addActionListener {
            if (!isUpdatingFromBackend) {
                toggleFeature("ibm", ibmCheckbox.isSelected)
            }
        }

        // Add listeners for operation buttons
        smokeTestButton.addActionListener {
            runSmokeTest()
        }

        metricsButton.addActionListener {
            deployMetrics()
        }
    }

    /**
     * Toggle feature on/off.
     */
    private fun toggleFeature(feature: String, enable: Boolean) {
        val checkbox = if (feature == "aws") awsCheckbox else ibmCheckbox

        scope.launch {
            try {
                if (enable) {
                    // Enable the feature via the new API
                    backendClient.enableFeature(feature)
                    showNotification("${feature.uppercase()} feature enabled", NotificationType.INFORMATION)
                } else {
                    // Disabling is not supported yet - revert checkbox
                    ApplicationManager.getApplication().invokeLater {
                        isUpdatingFromBackend = true
                        checkbox.isSelected = true
                        isUpdatingFromBackend = false
                        showNotification(
                            "Feature disabling is not supported yet",
                            NotificationType.WARNING
                        )
                    }
                }
            } catch (e: Exception) {
                // Revert checkbox on error
                ApplicationManager.getApplication().invokeLater {
                    isUpdatingFromBackend = true
                    checkbox.isSelected = !enable
                    isUpdatingFromBackend = false
                    showNotification("Failed to toggle ${feature.uppercase()}: ${e.message}", NotificationType.ERROR)
                }
            }
        }
    }

    /**
     * Run smoke test on the deployed MPC.
     */
    private fun runSmokeTest() {
        scope.launch {
            try {
                smokeTestButton.isEnabled = false
                backendClient.runSmokeTest()
                showNotification("Smoke test started - check logs for results", NotificationType.INFORMATION)
            } catch (e: Exception) {
                showNotification("Failed to start smoke test: ${e.message}", NotificationType.ERROR)
            } finally {
                ApplicationManager.getApplication().invokeLater {
                    smokeTestButton.isEnabled = true
                }
            }
        }
    }

    /**
     * Deploy metrics stack (Prometheus and Grafana).
     */
    private fun deployMetrics() {
        scope.launch {
            try {
                metricsButton.isEnabled = false
                backendClient.deployMetrics()
                showNotification("Metrics deployment started", NotificationType.INFORMATION)
            } catch (e: Exception) {
                showNotification("Failed to deploy metrics: ${e.message}", NotificationType.ERROR)
            } finally {
                ApplicationManager.getApplication().invokeLater {
                    metricsButton.isEnabled = true
                }
            }
        }
    }

    /**
     * Update feature states from backend status.
     * Uses strongly-typed DevEnvironment instead of Map<String, Any>.
     */
    fun updateFeatures(env: com.redhat.mpcdev.model.DevEnvironment) {
        ApplicationManager.getApplication().invokeLater {
            isUpdatingFromBackend = true
            awsCheckbox.isSelected = env.features.awsEnabled
            ibmCheckbox.isSelected = env.features.ibmEnabled
            isUpdatingFromBackend = false
        }
    }

    fun dispose() {
        scope.cancel()
    }
}
