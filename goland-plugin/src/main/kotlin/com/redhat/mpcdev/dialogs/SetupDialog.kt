package com.redhat.mpcdev.dialogs

import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBCheckBox
import com.intellij.ui.components.JBLabel
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import java.awt.Insets
import javax.swing.JComponent
import javax.swing.JPanel

/**
 * Dialog for environment setup configuration.
 *
 * Allows the user to:
 * - Skip AWS/IBM features
 * - Enable metrics dashboard
 */
class SetupDialog : DialogWrapper(true) {

    private val skipAwsCheckbox = JBCheckBox("Skip AWS configuration", false)
    private val skipIbmCheckbox = JBCheckBox("Skip IBM configuration", false)
    private val enableMetricsCheckbox = JBCheckBox("Enable metrics dashboard (Prometheus/Grafana)", false)

    init {
        title = "Start New Environment"
        init()
    }

    override fun createCenterPanel(): JComponent {
        val panel = JPanel(GridBagLayout())
        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = GridBagConstraints.RELATIVE
        gbc.anchor = GridBagConstraints.WEST
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.insets = Insets(5, 5, 5, 5)

        panel.add(JBLabel("Feature Configuration:"), gbc)
        panel.add(skipAwsCheckbox, gbc)
        panel.add(skipIbmCheckbox, gbc)

        gbc.insets = Insets(15, 5, 5, 5) // Add spacing
        panel.add(JBLabel("Optional Features:"), gbc)

        gbc.insets = Insets(5, 5, 5, 5)
        panel.add(enableMetricsCheckbox, gbc)

        gbc.insets = Insets(15, 5, 5, 5)
        panel.add(
            JBLabel("<html><i>Note: Setup may take 5-10 minutes.<br>" +
                    "You can monitor progress in the Activity feed.</i></html>"),
            gbc
        )

        return panel
    }

    /**
     * Get the list of features to skip.
     */
    fun getSkipFeatures(): List<String> {
        val skipList = mutableListOf<String>()
        if (skipAwsCheckbox.isSelected) skipList.add("aws")
        if (skipIbmCheckbox.isSelected) skipList.add("ibm")
        return skipList
    }

    /**
     * Check if metrics should be enabled.
     */
    fun shouldEnableMetrics(): Boolean {
        return enableMetricsCheckbox.isSelected
    }
}
