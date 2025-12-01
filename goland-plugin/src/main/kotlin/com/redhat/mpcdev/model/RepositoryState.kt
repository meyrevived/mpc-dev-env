package com.redhat.mpcdev.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Represents the state of a Git repository.
 */
@Serializable
data class RepositoryState(
    @SerialName("name")
    val name: String,

    @SerialName("path")
    val path: String,

    @SerialName("current_branch")
    val currentBranch: String,

    @SerialName("last_synced")
    val lastSynced: String,

    @SerialName("commits_behind_upstream")
    val commitsBehindUpstream: Int,

    @SerialName("has_local_changes")
    val hasLocalChanges: Boolean
)
