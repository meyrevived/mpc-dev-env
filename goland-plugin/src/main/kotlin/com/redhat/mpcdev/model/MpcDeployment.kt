package com.redhat.mpcdev.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Represents the deployed MPC instance state.
 */
@Serializable
data class MpcDeployment(
    @SerialName("controller_image")
    val controllerImage: String,

    @SerialName("otp_image")
    val otpImage: String,

    @SerialName("deployed_at")
    val deployedAt: String,

    @SerialName("source_git_hash")
    val sourceGitHash: String
)
