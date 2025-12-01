package com.redhat.mpcdev.dialogs

import com.intellij.openapi.ui.ComboBox
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBLabel
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import java.awt.Insets
import javax.swing.JComponent
import javax.swing.JPanel

/**
 * Dialog for switching Git branches.
 *
 * Shows a dropdown of available branches and allows the user to select one.
 */
class BranchSwitchDialog(
    private val repositoryName: String,
    private val branches: List<String>,
    private val currentBranch: String
) : DialogWrapper(true) {

    private val branchComboBox = ComboBox(branches.toTypedArray())

    init {
        title = "Switch Branch - $repositoryName"
        branchComboBox.selectedItem = currentBranch
        init()
    }

    override fun createCenterPanel(): JComponent {
        val panel = JPanel(GridBagLayout())
        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = 0
        gbc.anchor = GridBagConstraints.WEST
        gbc.insets = Insets(5, 5, 5, 5)

        panel.add(JBLabel("Select branch:"), gbc)

        gbc.gridx = 1
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.weightx = 1.0
        panel.add(branchComboBox, gbc)

        return panel
    }

    /**
     * Get the selected branch.
     */
    fun getSelectedBranch(): String? {
        return branchComboBox.selectedItem as? String
    }
}
