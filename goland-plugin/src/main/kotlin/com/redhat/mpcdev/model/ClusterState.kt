package com.redhat.mpcdev.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Represents the state of the Kubernetes cluster.
 */
@Serializable
data class ClusterState(
    @SerialName("name")
    val name: String,

    @SerialName("created_at")
    val createdAt: String,

    @SerialName("status")
    val status: String,  // "running", "paused", "stopped"

    @SerialName("kubeconfig_path")
    val kubeconfigPath: String,

    @SerialName("konflux_deployed")
    val konfluxDeployed: Boolean
)
