package com.redhat.mpcdev.ui.panels

import com.intellij.openapi.application.ApplicationManager
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBPanel
import com.redhat.mpcdev.model.DevEnvironment
import com.redhat.mpcdev.services.BackendClient
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import javax.swing.BorderFactory

/**
 * Panel displaying cluster and MPC deployment status.
 *
 * Shows:
 * - Cluster name and status (running/paused/stopped)
 * - MPC deployment status
 * - Session information
 */
class StatusPanel(private val backendClient: BackendClient) : JBPanel<StatusPanel>(GridBagLayout()) {

    private val clusterLabel = JBLabel()
    private val mpcLabel = JBLabel()
    private val sessionLabel = JBLabel()

    init {
        border = BorderFactory.createTitledBorder("Status")

        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.anchor = GridBagConstraints.WEST
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = java.awt.Insets(5, 5, 5, 5)

        add(clusterLabel, gbc)
        add(mpcLabel, gbc)
        add(sessionLabel, gbc)

        // Initial state
        clusterLabel.text = "Cluster: Loading..."
        mpcLabel.text = "MPC: Loading..."
        sessionLabel.text = ""
    }

    /**
     * Update status display with data from backend.
     * Uses strongly-typed DevEnvironment instead of Map<String, Any>.
     */
    fun updateStatus(env: DevEnvironment) {
        ApplicationManager.getApplication().invokeLater {
            val clusterRunning = env.cluster.status == "running"
            val mpcDeployed = env.mpcDeployment != null

            // Update cluster status
            val clusterIcon = if (clusterRunning) "●" else "○"
            val clusterColor = if (clusterRunning) "green" else "gray"
            clusterLabel.text = "<html><font color='$clusterColor'>$clusterIcon</font> Cluster: " +
                    "${env.cluster.name} (${env.cluster.status})</html>"

            // Update MPC status
            val mpcIcon = if (mpcDeployed) "●" else "○"
            val mpcColor = if (mpcDeployed) "green" else "gray"
            mpcLabel.text = "<html><font color='$mpcColor'>$mpcIcon</font> MPC: " +
                    if (mpcDeployed) "Deployed" else "Not deployed" + "</html>"

            // Update session info
            sessionLabel.text = "Session: ${env.sessionId.take(8)}"
        }
    }
}
