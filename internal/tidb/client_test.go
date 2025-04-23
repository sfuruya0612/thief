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
}
