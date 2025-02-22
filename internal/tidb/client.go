package tidb

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
)

type DigestClient struct {
	publicKey  string
	privateKey string
	Client     *http.Client
	nc         int
}

func NewDigestClient(publicKey, privateKey string) *DigestClient {
	return &DigestClient{
		publicKey:  publicKey,
		privateKey: privateKey,
		Client:     &http.Client{},
		nc:         1,
	}
}

func generateCnonce() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ParseDigestHeader(header string) map[string]string {
	result := make(map[string]string)
	re := regexp.MustCompile(`(\w+)="([^"]+)"`)
	matches := re.FindAllStringSubmatch(header, -1)

	for _, match := range matches {
		result[match[1]] = match[2]
	}
	return result
}

func (d *DigestClient) CreateDigestHeader(method, uri string, digestParams map[string]string) (string, error) {
	cnonce, err := generateCnonce()
	if err != nil {
		return "", fmt.Errorf("Failed to generate cnonce: %v", err)
	}

	realm := digestParams["realm"]
	nonce := digestParams["nonce"]
	qop := digestParams["qop"]

	ha1 := md5.Sum([]byte(fmt.Sprintf("%s:%s:%s", d.publicKey, realm, d.privateKey)))
	ha2 := md5.Sum([]byte(fmt.Sprintf("%s:%s", method, uri)))

	ncStr := fmt.Sprintf("%08x", d.nc)
	response := md5.Sum([]byte(fmt.Sprintf(
		"%s:%s:%s:%s:%s:%s",
		hex.EncodeToString(ha1[:]),
		nonce,
		ncStr,
		cnonce,
		qop,
		hex.EncodeToString(ha2[:]),
	)))

	auth := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=MD5, qop=%s, nc=%s, cnonce="%s", response="%s"`,
		d.publicKey,
		realm,
		nonce,
		uri,
		qop,
		ncStr,
		cnonce,
		hex.EncodeToString(response[:]),
	)

	d.nc++
	return auth, nil
}

func (d *DigestClient) Get(host, endpoint string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", host, endpoint)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request: %v", err)
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to send request: %v", err)
	}

	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader == "" {
		return nil, fmt.Errorf("WWW-Authenticate header not found")
	}

	digestParams := ParseDigestHeader(authHeader)

	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request: %v", err)
	}

	authStr, err := d.CreateDigestHeader("GET", endpoint, digestParams)
	if err != nil {
		return nil, fmt.Errorf("Failed to create auth header: %v", err)
	}

	req.Header.Set("Authorization", authStr)
	req.Header.Set("Accept", "application/json")

	resp, err = d.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to send request: %v", err)
	}

	return resp, nil
}
