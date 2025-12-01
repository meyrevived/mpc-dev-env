package com.redhat.mpcdev.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Represents the state of cloud provider features.
 */
@Serializable
data class FeatureState(
    @SerialName("aws_enabled")
    val awsEnabled: Boolean,

    @SerialName("ibm_enabled")
    val ibmEnabled: Boolean
)
