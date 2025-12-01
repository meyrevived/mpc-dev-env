package com.redhat.mpcdev.services

import com.intellij.openapi.diagnostic.Logger
import com.redhat.mpcdev.exceptions.BackendConnectionException
import com.redhat.mpcdev.exceptions.BackendDataException
import com.redhat.mpcdev.model.DevEnvironment
import com.redhat.mpcdev.model.RebuildResponse
import io.ktor.client.*
import io.ktor.client.call.*
import io.ktor.client.engine.cio.*
import io.ktor.client.plugins.contentnegotiation.*
import io.ktor.client.request.*
import io.ktor.http.*
import io.ktor.serialization.kotlinx.json.*
import kotlinx.serialization.json.Json

/**
 * HTTP client for communicating with the Go daemon backend API.
 *
 * This client provides strongly-typed access to the Go daemon's HTTP API.
 * All responses are deserialized into Kotlin data classes for type safety.
 *
 * The Go daemon exposes the following endpoints:
 * - GET /api/status - Returns current development environment state
 * - POST /api/rebuild - Triggers an asynchronous MPC rebuild
 * - POST /api/smoke-test - Runs smoke test on the deployed MPC
 * - POST /api/metrics/deploy - Deploys Prometheus and Grafana metrics stack
 * - POST /api/features/enable - Enables a specific feature (AWS or IBM)
 *
 * @property baseUrl The base URL of the Go daemon API (default: http://127.0.0.1:8765)
 * @property httpClient Optional HttpClient for dependency injection (primarily for testing)
 */
class BackendClient(
    val baseUrl: String = "http://127.0.0.1:8765",
    httpClient: HttpClient? = null
) {
    private val log = Logger.getInstance(BackendClient::class.java)

    private val client = httpClient ?: HttpClient(CIO) {
        install(ContentNegotiation) {
            json(Json {
                ignoreUnknownKeys = true
                isLenient = true
            })
        }
    }

    /**
     * Get the current development environment status.
     *
     * Calls GET /api/status and returns the DevEnvironment state which includes:
     * - Session ID and timestamps
     * - Cluster state (name, status, kubeconfig path)
     * - Repository states (branches, sync status, local changes)
     * - MPC deployment information (if deployed)
     * - Feature flags (AWS, IBM)
     *
     * @return DevEnvironment object with current status
     * @throws BackendConnectionException if the backend is not reachable or connection fails
     * @throws BackendDataException if the response cannot be parsed
     */
    suspend fun getStatus(): DevEnvironment {
        return try {
            val response = client.get("$baseUrl/api/status")
            val body: DevEnvironment = response.body()
            log.info("Successfully fetched status from backend")
            body
        } catch (e: io.ktor.client.plugins.ClientRequestException) {
            // HTTP 4xx errors
            log.error("Backend returned 4xx error", e)
            throw BackendConnectionException(e)
        } catch (e: io.ktor.client.plugins.ServerResponseException) {
            // HTTP 5xx errors
            log.error("Backend returned 5xx error", e)
            throw BackendConnectionException(e)
        } catch (e: io.ktor.serialization.JsonConvertException) {
            // JSON conversion/parsing errors (wraps SerializationException)
            log.error("Failed to parse JSON from backend", e)
            throw BackendDataException(e)
        } catch (e: kotlinx.serialization.SerializationException) {
            // JSON parsing/deserialization errors
            log.error("JSON deserialization failed", e)
            throw BackendDataException(e)
        } catch (e: Exception) {
            // Network connection errors and other unexpected errors
            // This catches ConnectException, SocketTimeoutException, etc.
            log.error("Unexpected error connecting to backend", e)
            throw BackendConnectionException(e)
        }
    }

    /**
     * Trigger a rebuild of the Multi-Platform Controller.
     *
     * Calls POST /api/rebuild which triggers the rebuild script asynchronously.
     * The API returns immediately (202 Accepted) without waiting for the rebuild
     * to complete. The actual rebuild runs in the background on the daemon.
     *
     * This executes the 05-build-mpc.sh script which:
     * 1. Builds the MPC Go binaries
     * 2. Builds Docker images (controller and OTP server)
     * 3. Pushes images to the local registry
     * 4. Updates the deployment on the cluster
     *
     * @return RebuildResponse with status message
     * @throws Exception if the API call fails (network error, backend not running, etc.)
     */
    suspend fun rebuildMpc(): RebuildResponse {
        return client.post("$baseUrl/api/rebuild").body()
    }

    /**
     * Run smoke test on the deployed Multi-Platform Controller.
     *
     * Calls POST /api/smoke-test which executes the smoke test script asynchronously.
     * The API returns immediately without waiting for the test to complete.
     *
     * This executes the lib/90-run-smoke-test.sh script which:
     * 1. Applies the smoke test TaskRun manifest
     * 2. Waits for the TaskRun to complete
     * 3. Verifies the test passed successfully
     *
     * @return RebuildResponse with status message
     * @throws BackendConnectionException if the backend is not reachable or connection fails
     */
    suspend fun runSmokeTest(): RebuildResponse {
        return try {
            client.post("$baseUrl/api/smoke-test").body()
        } catch (e: io.ktor.client.plugins.ClientRequestException) {
            throw BackendConnectionException(e)
        } catch (e: io.ktor.client.plugins.ServerResponseException) {
            throw BackendConnectionException(e)
        } catch (e: Exception) {
            throw BackendConnectionException(e)
        }
    }

    /**
     * Deploy metrics stack (Prometheus and Grafana).
     *
     * Calls POST /api/metrics/deploy which deploys the metrics infrastructure asynchronously.
     * The API returns immediately without waiting for deployment to complete.
     *
     * This executes the lib/95-deploy-metrics.sh script which:
     * 1. Deploys Prometheus to monitor MPC
     * 2. Deploys Grafana with pre-configured dashboards
     * 3. Configures service monitoring
     *
     * @return RebuildResponse with status message
     * @throws BackendConnectionException if the backend is not reachable or connection fails
     */
    suspend fun deployMetrics(): RebuildResponse {
        return try {
            client.post("$baseUrl/api/metrics/deploy").body()
        } catch (e: io.ktor.client.plugins.ClientRequestException) {
            throw BackendConnectionException(e)
        } catch (e: io.ktor.client.plugins.ServerResponseException) {
            throw BackendConnectionException(e)
        } catch (e: Exception) {
            throw BackendConnectionException(e)
        }
    }

    /**
     * Enable a specific cloud provider feature.
     *
     * Calls POST /api/features/enable with the feature name in the request body.
     * The API returns immediately without waiting for the feature to be enabled.
     *
     * Supported features:
     * - "aws" - Enable AWS cloud provider support
     * - "ibm" - Enable IBM cloud provider support
     *
     * @param featureName The name of the feature to enable ("aws" or "ibm")
     * @return RebuildResponse with status message
     * @throws BackendConnectionException if the backend is not reachable or connection fails
     */
    suspend fun enableFeature(featureName: String): RebuildResponse {
        return try {
            client.post("$baseUrl/api/features/enable") {
                contentType(ContentType.Application.Json)
                setBody(mapOf("feature" to featureName))
            }.body()
        } catch (e: io.ktor.client.plugins.ClientRequestException) {
            throw BackendConnectionException(e)
        } catch (e: io.ktor.client.plugins.ServerResponseException) {
            throw BackendConnectionException(e)
        } catch (e: Exception) {
            throw BackendConnectionException(e)
        }
    }

    /**
     * Close the HTTP client and release resources.
     *
     * Call this method when the BackendClient is no longer needed to properly
     * clean up network connections and threads.
     */
    fun dispose() {
        client.close()
    }
}
