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
 * Panel for repository management.
 *
 * Shows current branch, upstream status, and local changes for each repository.
 */
class RepositoryPanel(private val backendClient: BackendClient) : JBPanel<RepositoryPanel>(GridBagLayout()) {

    private val mpcRepoLabel = JBLabel()
    private val konfluxRepoLabel = JBLabel()
    private val infraRepoLabel = JBLabel()

    init {
        border = BorderFactory.createTitledBorder("Repositories")

        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.anchor = GridBagConstraints.WEST
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = java.awt.Insets(5, 5, 5, 5)

        add(mpcRepoLabel, gbc)
        add(konfluxRepoLabel, gbc)
        add(infraRepoLabel, gbc)

        // Initial state
        mpcRepoLabel.text = "📁 multi-platform-controller"
        konfluxRepoLabel.text = "📁 konflux-ci"
        infraRepoLabel.text = "📁 infra-deployments"
    }

    /**
     * Update repository information from backend status.
     * Uses strongly-typed DevEnvironment instead of Map<String, Any>.
     */
    fun updateRepositories(env: DevEnvironment) {
        ApplicationManager.getApplication().invokeLater {
            // Update MPC repo
            env.repositories["multi-platform-controller"]?.let { repo ->
                val statusText = buildStatusText(repo.commitsBehindUpstream, repo.hasLocalChanges)
                mpcRepoLabel.text = "📁 multi-platform-controller: ${repo.currentBranch}$statusText"
            }

            // Update Konflux repo
            env.repositories["konflux-ci"]?.let { repo ->
                val statusText = buildStatusText(repo.commitsBehindUpstream, repo.hasLocalChanges)
                konfluxRepoLabel.text = "📁 konflux-ci: ${repo.currentBranch}$statusText"
            }

            // Update infra repo
            env.repositories["infra-deployments"]?.let { repo ->
                val statusText = buildStatusText(repo.commitsBehindUpstream, repo.hasLocalChanges)
                infraRepoLabel.text = "📁 infra-deployments: ${repo.currentBranch}$statusText"
            }
        }
    }

    /**
     * Build status text showing upstream commits and local changes.
     */
    private fun buildStatusText(commitsBehindUpstream: Int, hasLocalChanges: Boolean): String {
        val parts = mutableListOf<String>()

        if (commitsBehindUpstream > 0) {
            parts.add("⚠️ $commitsBehindUpstream behind")
        }

        if (hasLocalChanges) {
            parts.add("● local changes")
        }

        return if (parts.isEmpty()) {
            " ✓"
        } else {
            " ${parts.joinToString(", ")}"
        }
    }
}

