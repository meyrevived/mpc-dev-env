package com.redhat.mpcdev.exceptions

/**
 * Exception thrown when the backend connection fails.
 *
 * This indicates network-level failures such as:
 * - Backend daemon not running
 * - Connection refused
 * - Network timeout
 * - DNS resolution failure
 */
class BackendConnectionException(cause: Throwable) : Exception("Failed to connect to backend", cause)

/**
 * Exception thrown when backend response cannot be parsed.
 *
 * This indicates data-level failures such as:
 * - Malformed JSON response
 * - JSON structure doesn't match expected schema
 * - Deserialization errors
 */
class BackendDataException(cause: Throwable) : Exception("Failed to parse backend response", cause)
