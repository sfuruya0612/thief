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
	Run:   login,
}

var ssoLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from SSO. Remove all cache files.",
	Run:   logout,
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

func login(cmd *cobra.Command, args []string) {
	profile := cmd.Flag("profile").Value.String()
	region := cmd.Flag("region").Value.String()
	url := cmd.Flag("url").Value.String()

	if url == "" {
		fmt.Println("Please specify the AWS SSO access portal URL.")
		return
	}

	oidcClient := aws.NewSSOOidcClient(profile, region)

	ssoOidcOpts := aws.SSOOidcOpts{
		ClientName: clientName,
		ClientType: clientType,
	}

	registerOutput, err := aws.RegisterClient(oidcClient, aws.GenerateRegisterClientInput(ssoOidcOpts))
	if err != nil {
		fmt.Printf("failed to register client: %v\n", err)
		return
	}

	startUrl := fmt.Sprintf("https://%s.awsapps.com/start/", url)

	ssoOidcOpts.ClientId = *registerOutput.ClientId
	ssoOidcOpts.ClientSecret = *registerOutput.ClientSecret
	ssoOidcOpts.StartUrl = startUrl

	deviceAuth, err := aws.StartDeviceAuthorization(oidcClient, aws.GenerateStartDeviceAuthorizationInput(ssoOidcOpts))
	if err != nil {
		fmt.Printf("failed to start device authorization: %v\n", err)
		return
	}

	if err := openBrowser(*deviceAuth.VerificationUriComplete); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
		return
	}

	ssoOidcOpts.DeviceCode = *deviceAuth.DeviceCode
	ssoOidcOpts.GrantType = grantType

	// Same output as aws sso login command.
	loginDisplay(startUrl, *deviceAuth.UserCode)

	tokenOutput, err := aws.WaitForToken(context.Background(), oidcClient, aws.GenerateCreateTokenInput(ssoOidcOpts))
	if err != nil {
		fmt.Printf("failed to get token: %v", err)
		return
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
		fmt.Printf("failed to save cache file: %v\n", err)
		return
	}

	// Same output as aws sso login command.
	fmt.Printf("Successfully logged into Start URL: %s\n", startUrl)
}

func logout(cmd *cobra.Command, args []string) {
	cacheDir, err := getCacheDir()
	if err != nil {
		fmt.Printf("failed to get cache directory: %v", err)
		return
	}

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		fmt.Printf("directory does not exist: %s", cacheDir)
		return
	}

	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory: %v", err)
		}

		if info.IsDir() {
			return nil
		}

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to delete file %s: %v", path, err)
		}

		return nil
	})
	if err != nil {
		fmt.Printf("error walking directory: %v", err)
		return
	}

	fmt.Println("Successfully signed out of all SSO profiles.")
}

func loginDisplay(startUrl, userCode string) {
	fmt.Println("Attempting to automatically open the SSO authorization page in your default browser.")
	fmt.Println("If the browser does not open or you wish to use a different device to authorize this request, open the following URL:")
	fmt.Println()
	fmt.Println(fmt.Sprintf("%s#/device", startUrl))
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
