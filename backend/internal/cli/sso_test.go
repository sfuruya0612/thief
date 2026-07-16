package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
)

func TestAppendProfiles(t *testing.T) {
	profiles := []ProfileConfig{
		{
			Name:      "my-account-adminaccess",
			StartUrl:  "https://example.awsapps.com/start/",
			Region:    "ap-northeast-1",
			AccountId: "123456789012",
			RoleName:  "AdminAccess",
		},
	}

	t.Run("append to empty config", func(t *testing.T) {
		got, err := appendProfiles("", profiles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantContains := []string{
			"[profile my-account-adminaccess]",
			"sso_start_url = https://example.awsapps.com/start/",
			"sso_region = ap-northeast-1",
			"sso_account_id = 123456789012",
			"sso_role_name = AdminAccess",
			"region = ap-northeast-1",
		}
		for _, want := range wantContains {
			if !strings.Contains(got, want) {
				t.Errorf("output does not contain %q:\n%s", want, got)
			}
		}
	})

	t.Run("keep existing config", func(t *testing.T) {
		existing := "[profile existing]\nregion = us-east-1\n"
		got, err := appendProfiles(existing, profiles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "[profile existing]") {
			t.Errorf("existing profile is lost:\n%s", got)
		}
		if !strings.Contains(got, "[profile my-account-adminaccess]") {
			t.Errorf("new profile is missing:\n%s", got)
		}
	})

	t.Run("skip duplicated profile", func(t *testing.T) {
		existing := "[profile my-account-adminaccess]\nregion = ap-northeast-1\n"
		got, err := appendProfiles(existing, profiles)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Count(got, "[profile my-account-adminaccess]") != 1 {
			t.Errorf("duplicated profile should be skipped:\n%s", got)
		}
	})
}

func TestGenerateSSOCacheKey(t *testing.T) {
	// AWS CLI と同じ SHA-1 hex 形式であること。
	got := generateSSOCacheKey("https://example.awsapps.com/start/")
	if len(got) != 40 {
		t.Errorf("cache key length = %d, want 40 (sha1 hex)", len(got))
	}
	// 同一入力に対して安定していること。
	if got != generateSSOCacheKey("https://example.awsapps.com/start/") {
		t.Error("cache key is not deterministic")
	}
	// 入力が違えばキーも変わること。
	if got == generateSSOCacheKey("https://other.awsapps.com/start/") {
		t.Error("different inputs should produce different keys")
	}
}

func TestSelectIndices(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  []int
	}{
		{name: "all", input: "all", max: 3, want: []int{0, 1, 2}},
		{name: "all uppercase", input: "ALL", max: 2, want: []int{0, 1}},
		{name: "comma separated", input: "1,3", max: 3, want: []int{0, 2}},
		{name: "with spaces", input: " 1 , 2 ", max: 3, want: []int{0, 1}},
		{name: "out of range skipped", input: "0,4,2", max: 3, want: []int{1}},
		{name: "non-numeric skipped", input: "a,2", max: 3, want: []int{1}},
		{name: "empty", input: "", max: 3, want: []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetOut(&bytes.Buffer{})

			got := selectIndices(cmd, tt.input, tt.max, "account")
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("selectIndices(%q) mismatch (-want +got):\n%s", tt.input, diff)
			}
		})
	}
}
