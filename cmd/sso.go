package cmd

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/spf13/cobra"
)

const (
	clientName = "thief"
	clientType = "public"
	grantType  = "urn:ietf:params:oauth:grant-type:device_code"
)

var ssoCmd = &cobra.Command{
	Use:   "sso",
	Short: "Manage SSO",
}

var ssoLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to SSO",
	Long:  "Authenticate with AWS SSO to obtain access credentials for AWS services.",
	RunE:  login,
}

var ssoLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from SSO. Remove all cache files.",
	Long:  "Sign out of AWS SSO by removing all cached credentials and tokens.",
	RunE:  logout,
}

type SSOTokenCache struct {
	StartURL              string `json:"startUrl"`
	Region                string `json:"region"`
	AccessToken           string `json:"accessToken"`
	ExpiresAt             string `json:"expiresAt"`
	ClientID              string `json:"clientId"`
	ClientSecret          string `json:"clientSecret"`
	RegistrationExpiresAt string `json:"registrationExpiresAt"`
}

// login authenticates with AWS SSO and caches the credentials.
func login(cmd *cobra.Command, args []string) error {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()
	url := cmd.Flag("url").Value.String()

	if url == "" {
		return fmt.Errorf("please specify the AWS SSO access portal URL with --url flag")
	}

	oidcClient, err := aws.NewSSOOidcClient(profile, region)
	if err != nil {
		return fmt.Errorf("create SSO OIDC client: %w", err)
	}

	ssoOidcOpts := aws.SSOOidcOpts{
		ClientName: clientName,
		ClientType: clientType,
	}

	registerOutput, err := aws.RegisterClient(oidcClient, aws.GenerateRegisterClientInput(ssoOidcOpts))
	if err != nil {
		return fmt.Errorf("register client: %w", err)
	}

	startUrl := fmt.Sprintf("https://%s.awsapps.com/start/", url)

	ssoOidcOpts.ClientId = *registerOutput.ClientId
	ssoOidcOpts.ClientSecret = *registerOutput.ClientSecret
	ssoOidcOpts.StartUrl = startUrl

	deviceAuth, err := aws.StartDeviceAuthorization(oidcClient, aws.GenerateStartDeviceAuthorizationInput(ssoOidcOpts))
	if err != nil {
		return fmt.Errorf("start device authorization: %w", err)
	}

	if err := openBrowser(*deviceAuth.VerificationUriComplete); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}

	ssoOidcOpts.DeviceCode = *deviceAuth.DeviceCode
	ssoOidcOpts.GrantType = grantType

	// Same output as aws sso login command.
	loginDisplay(startUrl, *deviceAuth.UserCode)

	tokenOutput, err := aws.WaitForToken(context.Background(), oidcClient, aws.GenerateCreateTokenInput(ssoOidcOpts))
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	expireAt := time.Now().UTC().Add(time.Duration(tokenOutput.ExpiresIn) * time.Second)

	cache := &SSOTokenCache{
		StartURL:              startUrl,
		Region:                region,
		AccessToken:           *tokenOutput.AccessToken,
		ExpiresAt:             expireAt.Format(time.RFC3339),
		ClientID:              *registerOutput.ClientId,
		ClientSecret:          *registerOutput.ClientSecret,
		RegistrationExpiresAt: time.Unix(registerOutput.ClientSecretExpiresAt, 0).UTC().Format(time.RFC3339),
	}

	// Create cache files in ~/.aws/sso/cache directory.
	if err = saveCacheFile(cache); err != nil {
		return fmt.Errorf("save cache file: %w", err)
	}

	// Same output as aws sso login command.
	cmd.Printf("Successfully logged into Start URL: %s\n", startUrl)
	return nil
}

// logout removes all SSO credential cache files.
func logout(cmd *cobra.Command, args []string) error {
	cacheDir, err := getCacheDir()
	if err != nil {
		return fmt.Errorf("get cache directory: %w", err)
	}

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", cacheDir)
	}

	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk directory: %w", err)
		}

		if info.IsDir() {
			return nil
		}

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("delete file %s: %w", path, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	cmd.Println("Successfully signed out of all SSO profiles.")
	return nil
}

func loginDisplay(startUrl, userCode string) {
	url := fmt.Sprintf("%s#/device", startUrl)

	fmt.Println("Attempting to automatically open the SSO authorization page in your default browser.")
	fmt.Println("If the browser does not open or you wish to use a different device to authorize this request, open the following URL:")
	fmt.Println()
	fmt.Println(url)
	fmt.Println()
	fmt.Println("Then enter the code:")
	fmt.Println()
	fmt.Println(userCode)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Run()
}

func saveCacheFile(cache *SSOTokenCache) error {
	cacheDir, err := getCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failed to create sso cache directory: %w", err)
	}

	cacheKey := generateCacheKey(cache.StartURL)
	cacheFile := filepath.Join(cacheDir, cacheKey+".json")

	jsonData, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := os.WriteFile(cacheFile, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

func getCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".aws", "sso", "cache"), nil
}

func generateCacheKey(cacheKey string) string {
	hasher := sha1.New()
	hasher.Write([]byte(cacheKey))
	return hex.EncodeToString(hasher.Sum(nil))
}
