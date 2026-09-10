package datadogauth

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestCodeChallengeS256 は RFC 7636 Appendix B の試験ベクタで S256 変換を確かめる。
func TestCodeChallengeS256(t *testing.T) {
	tests := []struct {
		name     string
		verifier string
		want     string
	}{
		{
			name:     "RFC 7636 Appendix B",
			verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			want:     "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		},
		{
			name:     "empty verifier",
			verifier: "",
			want:     "47DEQpj8HBSa-_TImW-5JCeuQeRkm5NMpJWZG3hSuFU",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeChallengeS256(tt.verifier); got != tt.want {
				t.Errorf("codeChallengeS256(%q) = %q, want %q", tt.verifier, got, tt.want)
			}
		})
	}
}

func TestNewPKCE(t *testing.T) {
	p, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE() err = %v", err)
	}

	verifier := p.Verifier.value()
	if len(verifier) != CodeVerifierLength {
		t.Errorf("code verifier length = %d, want %d", len(verifier), CodeVerifierLength)
	}
	// RFC 7636 §4.1 は 43 文字以上 128 文字以下、かつ unreserved 文字だけを許す。
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("code verifier length %d is out of the RFC 7636 range [43, 128]", len(verifier))
	}
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	if i := strings.IndexFunc(verifier, func(r rune) bool { return !strings.ContainsRune(unreserved, r) }); i >= 0 {
		t.Errorf("code verifier has a character outside the unreserved set at %d: %q", i, verifier)
	}
	if strings.Contains(verifier, "=") {
		t.Errorf("code verifier must not be padded: %q", verifier)
	}

	if p.Method != codeChallengeMethodS256 {
		t.Errorf("method = %q, want %q", p.Method, codeChallengeMethodS256)
	}
	if want := codeChallengeS256(verifier); p.Challenge != want {
		t.Errorf("challenge = %q, want %q", p.Challenge, want)
	}
	if _, err := base64.RawURLEncoding.DecodeString(p.Challenge); err != nil {
		t.Errorf("challenge is not raw base64url: %v", err)
	}
}

// TestNewPKCEIsRandom は生成のたびに異なる code_verifier が出ることを確かめる。
func TestNewPKCEIsRandom(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		p, err := NewPKCE()
		if err != nil {
			t.Fatalf("NewPKCE() err = %v", err)
		}
		if seen[p.Verifier.value()] {
			t.Fatalf("duplicate code verifier at iteration %d", i)
		}
		seen[p.Verifier.value()] = true
	}
}

func TestNewState(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		state, err := newState()
		if err != nil {
			t.Fatalf("newState() err = %v", err)
		}
		if state == "" {
			t.Fatal("state is empty")
		}
		if _, err := base64.RawURLEncoding.DecodeString(state); err != nil {
			t.Errorf("state is not raw base64url: %v", err)
		}
		if seen[state] {
			t.Fatalf("duplicate state at iteration %d", i)
		}
		seen[state] = true
	}
}
