package com.redhat.mpcdev.dialogs

import com.intellij.openapi.fileChooser.FileChooserDescriptorFactory
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.openapi.ui.TextFieldWithBrowseButton
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBPasswordField
import com.intellij.ui.components.JBTextField
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import java.awt.Insets
import javax.swing.JComponent
import javax.swing.JPanel

/**
 * Dialog for entering cloud provider credentials.
 *
 * Supports AWS and IBM Cloud credential input.
 */
class CredentialDialog(
    private val provider: String // "aws" or "ibm"
) : DialogWrapper(true) {

    private val accessKeyField = JBTextField()
    private val secretKeyField = JBPasswordField()
    private val apiKeyField = JBTextField()
    private val sshKeyField = TextFieldWithBrowseButton()

    init {
        title = "Configure $provider Credentials"
        init()
    }

    override fun createCenterPanel(): JComponent {
        val panel = JPanel(GridBagLayout())
        val gbc = GridBagConstraints()
        gbc.gridx = 0
        gbc.gridy = 0
        gbc.anchor = GridBagConstraints.WEST
        gbc.insets = Insets(5, 5, 5, 5)

        if (provider.equals("aws", ignoreCase = true)) {
            // AWS fields
            gbc.gridx = 0
            panel.add(JBLabel("Access Key ID:"), gbc)
            gbc.gridx = 1
            gbc.fill = GridBagConstraints.HORIZONTAL
            gbc.weightx = 1.0
            panel.add(accessKeyField, gbc)

            gbc.gridy++
            gbc.gridx = 0
            gbc.weightx = 0.0
            gbc.fill = GridBagConstraints.NONE
            panel.add(JBLabel("Secret Access Key:"), gbc)
            gbc.gridx = 1
            gbc.fill = GridBagConstraints.HORIZONTAL
            gbc.weightx = 1.0
            panel.add(secretKeyField, gbc)

        } else if (provider.equals("ibm", ignoreCase = true)) {
            // IBM fields
            gbc.gridx = 0
            panel.add(JBLabel("API Key:"), gbc)
            gbc.gridx = 1
            gbc.fill = GridBagConstraints.HORIZONTAL
            gbc.weightx = 1.0
            panel.add(apiKeyField, gbc)
        }

        // SSH key field (common)
        gbc.gridy++
        gbc.gridx = 0
        gbc.weightx = 0.0
        gbc.fill = GridBagConstraints.NONE
        panel.add(JBLabel("SSH Private Key:"), gbc)
        gbc.gridx = 1
        gbc.fill = GridBagConstraints.HORIZONTAL
        gbc.weightx = 1.0

        // Configure file chooser
        sshKeyField.addBrowseFolderListener(
            "Select SSH Private Key",
            "Choose the SSH private key file",
            null,
            FileChooserDescriptorFactory.createSingleFileDescriptor()
        )
        panel.add(sshKeyField, gbc)

        return panel
    }

    /**
     * Get the entered credentials.
     */
    fun getCredentials(): Map<String, String> {
        val credentials = mutableMapOf<String, String>()

        if (provider.equals("aws", ignoreCase = true)) {
            credentials["access_key"] = accessKeyField.text
            credentials["secret_key"] = String(secretKeyField.password)
        } else if (provider.equals("ibm", ignoreCase = true)) {
            credentials["api_key"] = apiKeyField.text
        }

        // Read SSH key file
        val sshKeyPath = sshKeyField.text
        if (sshKeyPath.isNotEmpty()) {
            try {
                val sshKeyContent = java.io.File(sshKeyPath).readText()
                credentials["ssh_key"] = sshKeyContent
            } catch (e: Exception) {
                // Handle error
            }
        }

        return credentials
    }
}
