package com.redhat.mpcdev

import com.redhat.mpcdev.exceptions.BackendConnectionException
import com.redhat.mpcdev.exceptions.BackendDataException
import com.redhat.mpcdev.model.*
import com.redhat.mpcdev.services.BackendClient
import io.ktor.client.*
import io.ktor.client.engine.mock.*
import io.ktor.client.plugins.contentnegotiation.*
import io.ktor.http.*
import io.ktor.serialization.kotlinx.json.*
import io.ktor.utils.io.*
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import org.junit.Assert.*
import org.junit.After
import org.junit.Before
import org.junit.Test

/**
 * Comprehensive test suite for BackendClient.
 *
 * Uses Ktor's MockEngine to simulate HTTP responses from the Go daemon API.
 * Tests cover:
 * - Successful API calls with proper deserialization into strongly-typed data classes
 * - Error handling for network failures
 * - Error handling for API errors (404, 500)
 * - Correct HTTP method usage (GET vs POST)
 * - Correct endpoint paths
 * - JSON deserialization for all model classes
 */
class BackendClientTest {

    private lateinit var client: BackendClient

    @Before
    fun setup() {
        // Will be initialized per test with appropriate mock engine
    }

    @After
    fun tearDown() {
        if (::client.isInitialized) {
            client.dispose()
        }
    }

    // ======================
    // GET /api/status Tests
    // ======================

    @Test
    fun `getStatus should return DevEnvironment on successful response`() = runBlocking {
        val mockEngine = MockEngine { request ->
            // Verify the request
            assertEquals(HttpMethod.Get, request.method)
            assertEquals("/api/status", request.url.encodedPath)

            // Return mock successful response matching Go API format
            respond(
                content = ByteReadChannel("""
                    {
                      "session_id": "test-session-123",
                      "created_at": "2024-01-15T10:30:00Z",
                      "last_active": "2024-01-15T14:45:00Z",
                      "cluster": {
                        "name": "konflux-mpc-debug",
                        "created_at": "2024-01-15T10:35:00Z",
                        "status": "running",
                        "kubeconfig_path": "/home/user/.kube/config",
                        "konflux_deployed": true
                      },
                      "repositories": {
                        "multi-platform-controller": {
                          "name": "multi-platform-controller",
                          "path": "/home/user/work/multi-platform-controller",
                          "current_branch": "fix-aws-timeout",
                          "last_synced": "2024-01-15T10:00:00Z",
                          "upstream_commits_ahead": 0,
                          "has_local_changes": false
                        }
                      },
                      "mpc_deployment": {
                        "controller_image": "localhost:5001/multi-platform-controller:debug",
                        "otp_image": "localhost:5001/multi-platform-otp:debug",
                        "deployed_at": "2024-01-15T11:00:00Z",
                        "source_git_hash": "abc123def456"
                      },
                      "features": {
                        "aws_enabled": false,
                        "ibm_enabled": true
                      }
                    }
                """.trimIndent()),
                status = HttpStatusCode.OK,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json {
                    ignoreUnknownKeys = true
                    isLenient = true
                })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)
        val result = client.getStatus()

        // Verify deserialization
        assertNotNull(result)
        assertEquals("test-session-123", result!!.sessionId)
        assertEquals("2024-01-15T10:30:00Z", result.createdAt)
        assertEquals("2024-01-15T14:45:00Z", result.lastActive)

        // Verify cluster state
        assertEquals("konflux-mpc-debug", result.cluster.name)
        assertEquals("running", result.cluster.status)
        assertEquals(true, result.cluster.konfluxDeployed)

        // Verify repositories
        assertEquals(1, result.repositories.size)
        val mpcRepo = result.repositories["multi-platform-controller"]
        assertNotNull(mpcRepo)
        assertEquals("fix-aws-timeout", mpcRepo!!.currentBranch)
        assertEquals(0, mpcRepo.upstreamCommitsAhead)
        assertEquals(false, mpcRepo.hasLocalChanges)

        // Verify MPC deployment
        assertNotNull(result.mpcDeployment)
        assertEquals("localhost:5001/multi-platform-controller:debug", result.mpcDeployment?.controllerImage)
        assertEquals("abc123def456", result.mpcDeployment?.sourceGitHash)

        // Verify features
        assertEquals(false, result.features.awsEnabled)
        assertEquals(true, result.features.ibmEnabled)
    }

    @Test
    fun `getStatus should handle null mpc_deployment correctly`() = runBlocking {
        val mockEngine = MockEngine { request ->
            respond(
                content = ByteReadChannel("""
                    {
                      "session_id": "test-session-123",
                      "created_at": "2024-01-15T10:30:00Z",
                      "last_active": "2024-01-15T14:45:00Z",
                      "cluster": {
                        "name": "konflux-mpc-debug",
                        "created_at": "2024-01-15T10:35:00Z",
                        "status": "running",
                        "kubeconfig_path": "/home/user/.kube/config",
                        "konflux_deployed": true
                      },
                      "repositories": {},
                      "features": {
                        "aws_enabled": false,
                        "ibm_enabled": false
                      }
                    }
                """.trimIndent()),
                status = HttpStatusCode.OK,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)
        val result = client.getStatus()

        assertNotNull(result)
        assertNull(result!!.mpcDeployment)
        assertEquals("test-session-123", result.sessionId)
    }

    @Test
    fun `getStatus should throw BackendConnectionException when backend is not reachable`() = runBlocking {
        // Mock engine that simulates connection failure
        val mockEngine = MockEngine {
            throw Exception("Connection refused")
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:9999", httpClient)

        // Should throw BackendConnectionException
        try {
            client.getStatus()
            fail("Expected BackendConnectionException to be thrown")
        } catch (e: BackendConnectionException) {
            // Expected exception
            assertNotNull(e)
            assertNotNull(e.cause)
        }
    }

    @Test
    fun `getStatus should make GET request to correct endpoint`() = runBlocking {
        val mockEngine = MockEngine { request ->
            // Verify HTTP method
            assertEquals(HttpMethod.Get, request.method)

            // Verify endpoint path
            assertEquals("/api/status", request.url.encodedPath)

            // Verify base URL (host and port)
            assertEquals("127.0.0.1", request.url.host)
            assertEquals(8765, request.url.port)

            respond(
                content = ByteReadChannel("""
                    {
                      "session_id": "test",
                      "created_at": "2024-01-15T10:30:00Z",
                      "last_active": "2024-01-15T14:45:00Z",
                      "cluster": {
                        "name": "test",
                        "created_at": "2024-01-15T10:35:00Z",
                        "status": "running",
                        "kubeconfig_path": "/test",
                        "konflux_deployed": true
                      },
                      "repositories": {},
                      "features": {
                        "aws_enabled": false,
                        "ibm_enabled": false
                      }
                    }
                """.trimIndent()),
                status = HttpStatusCode.OK,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)
        val result = client.getStatus()

        assertNotNull(result)
    }

    // ======================
    // POST /api/rebuild Tests
    // ======================

    @Test
    fun `rebuildMpc should return RebuildResponse on successful request`() = runBlocking {
        val mockEngine = MockEngine { request ->
            assertEquals(HttpMethod.Post, request.method)
            assertEquals("/api/rebuild", request.url.encodedPath)

            respond(
                content = ByteReadChannel("""{"status": "rebuild initiated"}"""),
                status = HttpStatusCode.Accepted,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)
        val result = client.rebuildMpc()

        assertNotNull(result)
        assertEquals("rebuild initiated", result.status)
    }

    @Test
    fun `rebuildMpc should make POST request to correct endpoint`() = runBlocking {
        val mockEngine = MockEngine { request ->
            // Verify HTTP method
            assertEquals(HttpMethod.Post, request.method)

            // Verify endpoint path
            assertEquals("/api/rebuild", request.url.encodedPath)

            // Verify base URL
            assertEquals("127.0.0.1", request.url.host)
            assertEquals(8765, request.url.port)

            respond(
                content = ByteReadChannel("""{"status":"rebuild initiated"}"""),
                status = HttpStatusCode.Accepted,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)
        val result = client.rebuildMpc()

        assertNotNull(result)
    }

    @Test
    fun `rebuildMpc should handle 202 Accepted status`() = runBlocking {
        // The Go API returns 202 Accepted (not 200 OK)
        val mockEngine = MockEngine { request ->
            respond(
                content = ByteReadChannel("""{"status":"rebuild initiated"}"""),
                status = HttpStatusCode.Accepted, // 202
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)
        val result = client.rebuildMpc()

        // Verify 202 is treated as success
        assertEquals("rebuild initiated", result.status)
    }

    // ======================
    // Error Handling Tests
    // ======================

    @Test
    fun `should throw BackendConnectionException for 404 Not Found error`() = runBlocking {
        val mockEngine = MockEngine { request ->
            respond(
                content = ByteReadChannel("Not Found"),
                status = HttpStatusCode.NotFound,
                headers = headersOf(HttpHeaders.ContentType, "text/plain")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)

        // Should throw BackendConnectionException for HTTP 4xx errors
        try {
            client.getStatus()
            fail("Expected BackendConnectionException to be thrown")
        } catch (e: BackendConnectionException) {
            assertNotNull(e)
        }
    }

    @Test
    fun `should handle 500 Internal Server Error for rebuildMpc`() = runBlocking {
        val mockEngine = MockEngine { request ->
            respond(
                content = ByteReadChannel("Internal Server Error"),
                status = HttpStatusCode.InternalServerError,
                headers = headersOf(HttpHeaders.ContentType, "text/plain")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)

        // rebuildMpc should propagate the error
        try {
            client.rebuildMpc()
            fail("Should have thrown exception")
        } catch (e: Exception) {
            // Expected
            assertNotNull(e)
        }
    }

    @Test
    fun `should throw BackendDataException for malformed JSON response`() = runBlocking {
        val mockEngine = MockEngine { request ->
            respond(
                content = ByteReadChannel("not valid json {{{"),
                status = HttpStatusCode.OK,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)

        // Should throw BackendDataException for JSON parsing errors
        try {
            client.getStatus()
            fail("Expected BackendDataException to be thrown")
        } catch (e: BackendDataException) {
            assertNotNull(e)
            assertNotNull(e.cause)
        }
    }

    @Test
    fun `should throw BackendConnectionException for 500 Internal Server Error on getStatus`() = runBlocking {
        val mockEngine = MockEngine { request ->
            respond(
                content = ByteReadChannel("Internal Server Error"),
                status = HttpStatusCode.InternalServerError,
                headers = headersOf(HttpHeaders.ContentType, "text/plain")
            )
        }

        val httpClient = HttpClient(mockEngine) {
            install(ContentNegotiation) {
                json(Json { ignoreUnknownKeys = true; isLenient = true })
            }
        }

        client = BackendClient("http://127.0.0.1:8765", httpClient)

        // Should throw BackendConnectionException for HTTP 5xx errors
        try {
            client.getStatus()
            fail("Expected BackendConnectionException to be thrown")
        } catch (e: BackendConnectionException) {
            assertNotNull(e)
        }
    }

    // ======================
    // Configuration Tests
    // ======================

    @Test
    fun `backend URL should be configurable`() {
        val customClient = BackendClient("http://custom:9999")
        assertEquals("http://custom:9999", customClient.baseUrl)
        customClient.dispose()
    }
}
