package tidb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewDigestClient(t *testing.T) {
	publicKey := "test-public-key"
	privateKey := "test-private-key"

	client := NewDigestClient(publicKey, privateKey)

	if client == nil {
		t.Fatal("expected non-nil client, got nil")
	}
	if client.publicKey != publicKey {
		t.Errorf("expected publicKey %q, got %q", publicKey, client.publicKey)
	}
	if client.privateKey != privateKey {
		t.Errorf("expected privateKey %q, got %q", privateKey, client.privateKey)
	}
	if client.Client == nil {
		t.Error("expected non-nil Client, got nil")
	}
	if client.nc != 1 {
		t.Errorf("expected nc 1, got %d", client.nc)
	}
}

func TestParseDigestHeader(t *testing.T) {
	header := `Digest realm="test-realm", nonce="1234567890", qop="auth", algorithm=MD5`

	params := ParseDigestHeader(header)

	if params["realm"] != "test-realm" {
		t.Errorf("expected realm 'test-realm', got %q", params["realm"])
	}
	if params["nonce"] != "1234567890" {
		t.Errorf("expected nonce '1234567890', got %q", params["nonce"])
	}
	if params["qop"] != "auth" {
		t.Errorf("expected qop 'auth', got %q", params["qop"])
	}
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

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(header, `Digest username="testuser"`) {
		t.Errorf("expected header to contain 'Digest username=\"testuser\"', got %q", header)
	}
	if !strings.Contains(header, `realm="test-realm"`) {
		t.Errorf("expected header to contain 'realm=\"test-realm\"', got %q", header)
	}
	if !strings.Contains(header, `nonce="1234567890"`) {
		t.Errorf("expected header to contain 'nonce=\"1234567890\"', got %q", header)
	}
	if !strings.Contains(header, `uri="/api/v1/clusters"`) {
		t.Errorf("expected header to contain 'uri=\"/api/v1/clusters\"', got %q", header)
	}
	if !strings.Contains(header, `qop=auth`) {
		t.Errorf("expected header to contain 'qop=auth', got %q", header)
	}
	if !strings.Contains(header, `nc=00000001`) {
		t.Errorf("expected header to contain 'nc=00000001', got %q", header)
	}

	if client.nc != 2 {
		t.Errorf("expected nc 2 after first call, got %d", client.nc)
	}

	header2, err := client.CreateDigestHeader(method, uri, digestParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(header2, `nc=00000002`) {
		t.Errorf("expected header2 to contain 'nc=00000002', got %q", header2)
	}
	if client.nc != 3 {
		t.Errorf("expected nc 3 after second call, got %d", client.nc)
	}
}

func TestGet(t *testing.T) {
	firstCallDone := false

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstCallDone {
			w.Header().Set("WWW-Authenticate", `Digest realm="test-realm", nonce="1234567890", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			firstCallDone = true
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			t.Error("Authorization header missing in second request")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

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

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response, got nil")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if resp.Request.Header.Get("Accept") != "application/json" {
		t.Errorf("expected Accept header 'application/json', got %q", resp.Request.Header.Get("Accept"))
	}
}

func TestGet_MissingWWWAuthenticate(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status":"success"}`))
		if err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer testServer.Close()

	client := NewDigestClient("testuser", "testpass")
	resp, err := client.Get(testServer.URL, "/api/v1/clusters")

	if err == nil {
		t.Error("expected error, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "WWW-Authenticate header not found") {
		t.Errorf("expected error to contain 'WWW-Authenticate header not found', got %q", err.Error())
	}
}

func TestGet_FailedFirstRequest(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	testServer.Close()

	client := NewDigestClient("testuser", "testpass")
	resp, err := client.Get(testServer.URL, "/api/v1/clusters")

	if err == nil {
		t.Error("expected error, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "failed to send request") {
		t.Errorf("expected error to contain 'failed to send request', got %q", err.Error())
	}
}

func TestGenerateCnonce(t *testing.T) {
	cnonce, err := generateCnonce()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cnonce == "" {
		t.Error("expected non-empty cnonce, got empty string")
	}

	cnonce2, err := generateCnonce()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cnonce2 == "" {
		t.Error("expected non-empty cnonce2, got empty string")
	}

	if cnonce == cnonce2 {
		t.Error("Generated cnonces should be random and different")
	}
}

func TestParseDigestHeader_ComplexHeader(t *testing.T) {
	header := `Digest realm="TiDB Cloud API", domain="tidbcloud.com", nonce="abcdef123456", opaque="mystery_value", stale=false, algorithm=MD5, qop="auth,auth-int"`

	params := ParseDigestHeader(header)

	if params["realm"] != "TiDB Cloud API" {
		t.Errorf("expected realm 'TiDB Cloud API', got %q", params["realm"])
	}
	if params["domain"] != "tidbcloud.com" {
		t.Errorf("expected domain 'tidbcloud.com', got %q", params["domain"])
	}
	if params["nonce"] != "abcdef123456" {
		t.Errorf("expected nonce 'abcdef123456', got %q", params["nonce"])
	}
	if params["opaque"] != "mystery_value" {
		t.Errorf("expected opaque 'mystery_value', got %q", params["opaque"])
	}
	if params["qop"] != "auth,auth-int" {
		t.Errorf("expected qop 'auth,auth-int', got %q", params["qop"])
	}
	if params["algorithm"] != "MD5" {
		t.Errorf("expected algorithm 'MD5', got %q", params["algorithm"])
	}
}

func TestParseDigestHeader_EmptyHeader(t *testing.T) {
	header := ""
	params := ParseDigestHeader(header)

	if params == nil {
		t.Error("expected non-nil map, got nil")
	}
	if len(params) != 0 {
		t.Errorf("expected empty map, got %v", params)
	}
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

	if client.nc != 1 {
		t.Errorf("expected initial nc 1, got %d", client.nc)
	}

	header, err := client.CreateDigestHeader(method, uri, digestParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		`Digest username="test_user"`,
		`realm="test-realm"`,
		`nonce="nonce123"`,
		`uri="/api/v1/projects"`,
		`algorithm=MD5`,
		`qop=auth`,
		`nc=00000001`,
		`response="`,
		`cnonce="`,
	} {
		if !strings.Contains(header, want) {
			t.Errorf("expected header to contain %q, got %q", want, header)
		}
	}

	if client.nc != 2 {
		t.Errorf("expected nc 2 after call, got %d", client.nc)
	}
}

func TestGet_SecondRequestFails(t *testing.T) {
	firstCallDone := false
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstCallDone {
			w.Header().Set("WWW-Authenticate", `Digest realm="test-realm", nonce="1234567890", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			firstCallDone = true
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	client := NewDigestClient("testuser", "testpass")
	resp, err := client.Get(testServer.URL, "/api/v1/clusters")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response, got nil")
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}
}
