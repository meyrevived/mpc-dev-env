package com.redhat.mpcdev.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Response from POST /api/rebuild endpoint.
 */
@Serializable
data class RebuildResponse(
    @SerialName("status")
    val status: String  // "rebuild initiated"
)
