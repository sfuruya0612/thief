package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sso"
)

// fakeSSOLogoutClient は ssoLogoutAPI のモック。受け取った入力を記録し、err を返す。
type fakeSSOLogoutClient struct {
	inputs []*sso.LogoutInput
	err    error
}

func (f *fakeSSOLogoutClient) Logout(_ context.Context, params *sso.LogoutInput, _ ...func(*sso.Options)) (*sso.LogoutOutput, error) {
	f.inputs = append(f.inputs, params)
	if f.err != nil {
		return nil, f.err
	}
	return &sso.LogoutOutput{}, nil
}

func TestRevokeSSOTokenSendsAccessToken(t *testing.T) {
	client := &fakeSSOLogoutClient{}
	if err := revokeSSOToken(context.Background(), client, "tok-a"); err != nil {
		t.Fatalf("revokeSSOToken() error = %v", err)
	}
	if len(client.inputs) != 1 {
		t.Fatalf("Logout called %d times, want 1", len(client.inputs))
	}
	if got := client.inputs[0].AccessToken; got == nil || *got != "tok-a" {
		t.Errorf("LogoutInput.AccessToken = %v, want tok-a", got)
	}
}

func TestRevokeSSOTokenWrapsError(t *testing.T) {
	cause := errors.New("UnauthorizedException: session expired")
	client := &fakeSSOLogoutClient{err: cause}
	err := revokeSSOToken(context.Background(), client, "tok-a")
	if !errors.Is(err, cause) {
		t.Fatalf("revokeSSOToken() = %v, want wrapping %v", err, cause)
	}
	if want := "sso logout: " + cause.Error(); err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
