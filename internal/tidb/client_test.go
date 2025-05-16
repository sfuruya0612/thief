package tidb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDigestClient(t *testing.T) {
	publicKey := "test-public-key"
	privateKey := "test-private-key"

	client := NewDigestClient(publicKey, privateKey)

	assert.NotNil(t, client)
	assert.Equal(t, publicKey, client.publicKey)
	assert.Equal(t, privateKey, client.privateKey)
	assert.NotNil(t, client.Client)
	assert.Equal(t, 1, client.nc)
}

func TestParseDigestHeader(t *testing.T) {
	header := `Digest realm="test-realm", nonce="1234567890", qop="auth", algorithm=MD5`

	params := ParseDigestHeader(header)

	assert.Equal(t, "test-realm", params["realm"])
	assert.Equal(t, "1234567890", params["nonce"])
	assert.Equal(t, "auth", params["qop"])
}

func TestCreateDigestHeader(t *testing.T) {
	client := NewDigestClient("testuser", "testpass")
	method := "GET"
	uri := "/api/v1/clusters"

	digestParams := map[string]string{
		"realm": "test-realm",
		"nonce": "1234567890",
		"qop":   "auth",
	}

	header, err := client.CreateDigestHeader(method, uri, digestParams)

	assert.NoError(t, err)
	assert.Contains(t, header, `Digest username="testuser"`)
	assert.Contains(t, header, `realm="test-realm"`)
	assert.Contains(t, header, `nonce="1234567890"`)
	assert.Contains(t, header, `uri="/api/v1/clusters"`)
	assert.Contains(t, header, `qop=auth`)
	assert.Contains(t, header, `nc=00000001`)

	// Verify that the nonce count increments
	assert.Equal(t, 2, client.nc)

	header2, err := client.CreateDigestHeader(method, uri, digestParams)
	assert.NoError(t, err)
	assert.Contains(t, header2, `nc=00000002`)
	assert.Equal(t, 3, client.nc)
}

func TestGet(t *testing.T) {
	// Setup test server
	firstCallDone := false

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstCallDone {
			// First call - return 401 with auth challenge
			w.Header().Set("WWW-Authenticate", `Digest realm="test-realm", nonce="1234567890", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			firstCallDone = true
			return
		}

		// Second call - validate auth header and return success
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			t.Error("Authorization header missing in second request")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Very basic validation
		if !strings.HasPrefix(authHeader, "Digest username=\"testuser\"") {
			t.Error("Invalid Authorization header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status":"success"}`))
		if err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer testServer.Close()

	client := NewDigestClient("testuser", "testpass")
	resp, err := client.Get(testServer.URL, "/api/v1/clusters")

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify that Accept header was set
	assert.Equal(t, "application/json", resp.Request.Header.Get("Accept"))
}

func TestGet_MissingWWWAuthenticate(t *testing.T) {
	// Setup test server that doesn't return WWW-Authenticate header
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 200 but no auth challenge
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status":"success"}`))
		if err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer testServer.Close()

	client := NewDigestClient("testuser", "testpass")
	resp, err := client.Get(testServer.URL, "/api/v1/clusters")

	// Should fail because WWW-Authenticate header is missing
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "WWW-Authenticate header not found")
}

func TestGet_FailedFirstRequest(t *testing.T) {
	// Setup a server that's going to be immediately closed
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	testServer.Close() // Close immediately to force connection failure

	client := NewDigestClient("testuser", "testpass")
	resp, err := client.Get(testServer.URL, "/api/v1/clusters")

	// Should fail because server is not available
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "Failed to send request")
}

func TestGenerateCnonce(t *testing.T) {
	// Test that cnonce generation works and returns a non-empty string
	cnonce, err := generateCnonce()
	assert.NoError(t, err)
	assert.NotEmpty(t, cnonce)

	// Generate another one and verify it's different (randomness check)
	cnonce2, err := generateCnonce()
	assert.NoError(t, err)
	assert.NotEmpty(t, cnonce2)

	// There's a tiny chance these could be equal, but it's very unlikely
	assert.NotEqual(t, cnonce, cnonce2, "Generated cnonces should be random and different")
}

func TestParseDigestHeader_ComplexHeader(t *testing.T) {
	// Test with a more complex header
	header := `Digest realm="TiDB Cloud API", domain="tidbcloud.com", nonce="abcdef123456", opaque="mystery_value", stale=false, algorithm=MD5, qop="auth,auth-int"`

	params := ParseDigestHeader(header)

	assert.Equal(t, "TiDB Cloud API", params["realm"])
	assert.Equal(t, "tidbcloud.com", params["domain"])
	assert.Equal(t, "abcdef123456", params["nonce"])
	assert.Equal(t, "mystery_value", params["opaque"])
	assert.Equal(t, "auth,auth-int", params["qop"])
	assert.Equal(t, "MD5", params["algorithm"])
}

func TestParseDigestHeader_EmptyHeader(t *testing.T) {
	// Test with empty header
	header := ""
	params := ParseDigestHeader(header)

	// Should return empty map, not nil
	assert.NotNil(t, params)
	assert.Empty(t, params)
}

func TestCreateDigestHeader_HeaderComponents(t *testing.T) {
	client := NewDigestClient("test_user", "test_password")
	method := "GET"
	uri := "/api/v1/projects"

	digestParams := map[string]string{
		"realm": "test-realm",
		"nonce": "nonce123",
		"qop":   "auth",
	}

	// Initial nonce count should be 1
	assert.Equal(t, 1, client.nc)

	header, err := client.CreateDigestHeader(method, uri, digestParams)
	assert.NoError(t, err)

	// Check all required components are present
	assert.Contains(t, header, `Digest username="test_user"`)
	assert.Contains(t, header, `realm="test-realm"`)
	assert.Contains(t, header, `nonce="nonce123"`)
	assert.Contains(t, header, `uri="/api/v1/projects"`)
	assert.Contains(t, header, `algorithm=MD5`)
	assert.Contains(t, header, `qop=auth`)
	assert.Contains(t, header, `nc=00000001`)
	assert.Contains(t, header, `response="`)
	assert.Contains(t, header, `cnonce="`)

	// Nonce count should be incremented
	assert.Equal(t, 2, client.nc)
}

func TestGet_SecondRequestFails(t *testing.T) {
	// Setup test server where second request fails
	firstCallDone := false
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstCallDone {
			// First call - return 401 with auth challenge
			w.Header().Set("WWW-Authenticate", `Digest realm="test-realm", nonce="1234567890", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			firstCallDone = true
			return
		}

		// Second call - return 500 error
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	// Create a client with a custom http.Client that has a very short timeout
	client := NewDigestClient("testuser", "testpass")

	// Make the request
	resp, err := client.Get(testServer.URL, "/api/v1/clusters")

	// Should not error (HTTP errors don't return Go errors)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
