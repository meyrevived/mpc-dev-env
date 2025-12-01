package com.redhat.mpcdev.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Represents the complete development environment state.
 * Returned by GET /api/status
 */
@Serializable
data class DevEnvironment(
    @SerialName("session_id")
    val sessionId: String,

    @SerialName("created_at")
    val createdAt: String,

    @SerialName("last_active")
    val lastActive: String,

    @SerialName("cluster")
    val cluster: ClusterState,

    @SerialName("repositories")
    val repositories: Map<String, RepositoryState>,

    @SerialName("mpc_deployment")
    val mpcDeployment: MpcDeployment? = null,

    @SerialName("features")
    val features: FeatureState,

    @SerialName("operation_status")
    val operationStatus: String? = null,  // "idle", "rebuilding", "smoke_testing", "deploying_metrics", etc.

    @SerialName("last_operation_error")
    val lastOperationError: String? = null  // Error message from last failed operation
)
