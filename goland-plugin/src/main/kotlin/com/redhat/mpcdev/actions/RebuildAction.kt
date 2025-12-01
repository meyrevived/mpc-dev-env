package com.redhat.mpcdev.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.components.service
import com.redhat.mpcdev.services.BackendClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/**
 * Action to rebuild and redeploy MPC.
 *
 * Triggered via Tools menu or keyboard shortcut (Ctrl+Shift+M).
 */
class RebuildAction : AnAction() {

    override fun actionPerformed(e: AnActionEvent) {
        val backendClient = service<BackendClient>()
        val project = e.project ?: return

        CoroutineScope(Dispatchers.Default).launch {
            try {
                backendClient.rebuildMpc()
                NotificationGroupManager.getInstance()
                    .getNotificationGroup("MPC Dev Studio")
                    .createNotification("MPC rebuild completed successfully", NotificationType.INFORMATION)
                    .notify(project)
            } catch (ex: Exception) {
                NotificationGroupManager.getInstance()
                    .getNotificationGroup("MPC Dev Studio")
                    .createNotification("MPC rebuild failed: ${ex.message}", NotificationType.ERROR)
                    .notify(project)
            }
        }
    }
}
