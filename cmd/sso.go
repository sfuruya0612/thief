package cmd

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
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

var ssoGenerateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Generate `~/.aws/config` file for the AWS CLI that uses the SSO profile.",
	RunE:  generateConfig,
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

const profileTemplate = `
[profile {{.Name}}]
sso_start_url = {{.StartUrl}}
sso_region = {{.Region}}
sso_account_id = {{.AccountId}}
sso_role_name = {{.RoleName}}
region = {{.Region}}
`

type ProfileConfig struct {
	Name      string
	StartUrl  string
	Region    string
	AccountId string
	RoleName  string
}

// login authenticates with AWS SSO and caches the credentials.
func login(cmd *cobra.Command, args []string) error {
	region := cmd.Flag("region").Value.String()
	url := cmd.Flag("url").Value.String()

	if url == "" {
		return fmt.Errorf("please specify the AWS SSO access portal URL with --url flag")
	}

	startUrl := fmt.Sprintf("https://%s.awsapps.com/start/", url)

	cache, err := getToken(region, startUrl)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
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

func generateConfig(cmd *cobra.Command, args []string) error {
	region := cmd.Flag("region").Value.String()
	url := cmd.Flag("url").Value.String()

	if url == "" {
		return fmt.Errorf("please specify the AWS SSO access portal URL with --url flag")
	}

	startUrl := fmt.Sprintf("https://%s.awsapps.com/start/", url)

	cache, err := getToken(region, startUrl)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	client, err := aws.NewSSOClient("", region)
	if err != nil {
		return fmt.Errorf("create SSO client: %w", err)
	}

	accounts, err := aws.ListAccounts(client, aws.GenerateListAccountsInput(aws.SSOOpts{AccessToken: cache.AccessToken}))
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	cmd.Printf("Found %d accounts from AWS SSO\n", len(accounts.AccountList))

	// Display accounts for selection
	cmd.Println("\nAvailable AWS accounts:")
	for i, account := range accounts.AccountList {
		cmd.Printf("[%d] %s (%s)\n", i+1, *account.AccountName, *account.AccountId)
	}

	// Interactive account selection
	accountsToProcess := make([]int, 0)
	cmd.Print("\nSelect accounts to configure (comma-separated numbers, or 'all' for all accounts): ")
	var accountInput string
	if _, err := fmt.Scanln(&accountInput); err != nil {
		// Handle empty input
		accountInput = ""
	}

	if strings.ToLower(accountInput) == "all" {
		// Add all accounts
		for i := range accounts.AccountList {
			accountsToProcess = append(accountsToProcess, i)
		}
	} else {
		// Parse selected account indices
		accountIndices := strings.Split(accountInput, ",")
		for _, indexStr := range accountIndices {
			indexStr = strings.TrimSpace(indexStr)
			if indexStr == "" {
				continue
			}

			index, err := strconv.Atoi(indexStr)
			if err != nil || index < 1 || index > len(accounts.AccountList) {
				cmd.Printf("Warning: Invalid account number '%s', skipped\n", indexStr)
				continue
			}

			// Convert to 0-based index
			accountsToProcess = append(accountsToProcess, index-1)
		}
	}

	if len(accountsToProcess) == 0 {
		return fmt.Errorf("no valid accounts selected")
	}

	// Get roles for selected accounts and create profiles
	profiles := make([]ProfileConfig, 0)
	for _, accountIndex := range accountsToProcess {
		account := accounts.AccountList[accountIndex]
		cmd.Printf("\nProcessing account %s (%s)...\n", *account.AccountName, *account.AccountId)

		roles, err := aws.ListAccountRoles(client, aws.GenerateListAccountRolesInput(aws.SSOOpts{
			AccessToken: cache.AccessToken,
			AccountId:   *account.AccountId,
		}))
		if err != nil {
			return fmt.Errorf("list account roles for %s: %w", *account.AccountId, err)
		}

		if len(roles.RoleList) == 0 {
			cmd.Printf("No roles found for account %s (%s)\n", *account.AccountName, *account.AccountId)
			continue
		}

		// Display available roles for this account
		cmd.Printf("Available roles for %s:\n", *account.AccountName)
		for i, role := range roles.RoleList {
			cmd.Printf("[%d] %s\n", i+1, *role.RoleName)
		}

		// Interactive role selection
		cmd.Print("Select roles to configure (comma-separated numbers, or 'all' for all roles): ")
		var roleInput string
		if _, err := fmt.Scanln(&roleInput); err != nil {
			// Handle empty input
			roleInput = ""
		}

		selectedRoles := make([]int, 0)
		if strings.ToLower(roleInput) == "all" {
			// Add all roles
			for i := range roles.RoleList {
				selectedRoles = append(selectedRoles, i)
			}
		} else {
			// Parse selected role indices
			roleIndices := strings.Split(roleInput, ",")
			for _, indexStr := range roleIndices {
				indexStr = strings.TrimSpace(indexStr)
				if indexStr == "" {
					continue
				}

				index, err := strconv.Atoi(indexStr)
				if err != nil || index < 1 || index > len(roles.RoleList) {
					cmd.Printf("Warning: Invalid role number '%s', skipped\n", indexStr)
					continue
				}

				// Convert to 0-based index
				selectedRoles = append(selectedRoles, index-1)
			}
		}

		// Create profiles for selected roles
		for _, roleIndex := range selectedRoles {
			role := roles.RoleList[roleIndex]
			profileName := fmt.Sprintf("%s-%s", *account.AccountName, *role.RoleName)
			profileName = strings.ToLower(strings.ReplaceAll(profileName, " ", "-"))

			profiles = append(profiles, ProfileConfig{
				Name:      profileName,
				StartUrl:  startUrl,
				Region:    region,
				AccountId: *account.AccountId,
				RoleName:  *role.RoleName,
			})
		}
	}

	if len(profiles) == 0 {
		return fmt.Errorf("no roles selected for any accounts")
	}

	cmd.Printf("\nFound %d role configurations to add\n", len(profiles))

	// Read existing config
	configPath, err := getAwsConfigPath()
	if err != nil {
		return fmt.Errorf("get AWS config path: %w", err)
	}

	existingConfig, err := readAwsConfig(configPath)
	if err != nil {
		cmd.Printf("Warning: Reading existing config: %v\n", err)
		existingConfig = ""
	}

	// Append profiles to config
	newConfig, err := appendProfiles(existingConfig, profiles)
	if err != nil {
		return fmt.Errorf("append profiles: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(newConfig), 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	cmd.Printf("Successfully updated AWS config file at %s\n", configPath)
	return nil
}

func getToken(region, url string) (*SSOTokenCache, error) {
	oidcClient, err := aws.NewSSOOidcClient("", region)
	if err != nil {
		return nil, fmt.Errorf("create SSO OIDC client: %w", err)
	}

	ssoOidcOpts := aws.SSOOidcOpts{
		ClientName: clientName,
		ClientType: clientType,
	}

	registerOutput, err := aws.RegisterClient(oidcClient, aws.GenerateRegisterClientInput(ssoOidcOpts))
	if err != nil {
		return nil, fmt.Errorf("failed to register client: %v", err)
	}

	ssoOidcOpts.ClientId = *registerOutput.ClientId
	ssoOidcOpts.ClientSecret = *registerOutput.ClientSecret
	ssoOidcOpts.StartUrl = url

	deviceAuth, err := aws.StartDeviceAuthorization(oidcClient, aws.GenerateStartDeviceAuthorizationInput(ssoOidcOpts))
	if err != nil {
		return nil, fmt.Errorf("failed to start device authorization: %v", err)
	}

	if err := openBrowser(*deviceAuth.VerificationUriComplete); err != nil {
		return nil, fmt.Errorf("failed to open browser: %v", err)
	}

	ssoOidcOpts.DeviceCode = *deviceAuth.DeviceCode
	ssoOidcOpts.GrantType = grantType

	// Same output as aws sso login command.
	loginDisplay(url, *deviceAuth.UserCode)

	tokenOutput, err := aws.WaitForToken(context.Background(), oidcClient, aws.GenerateCreateTokenInput(ssoOidcOpts))
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %v", err)
	}

	expireAt := time.Now().UTC().Add(time.Duration(tokenOutput.ExpiresIn) * time.Second)

	return &SSOTokenCache{
		StartURL:              url,
		Region:                region,
		AccessToken:           *tokenOutput.AccessToken,
		ExpiresAt:             expireAt.Format(time.RFC3339),
		ClientID:              *registerOutput.ClientId,
		ClientSecret:          *registerOutput.ClientSecret,
		RegistrationExpiresAt: time.Unix(registerOutput.ClientSecretExpiresAt, 0).UTC().Format(time.RFC3339),
	}, nil
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

// getAwsConfigPath returns the path to the AWS config file.
func getAwsConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".aws", "config")
	return configPath, nil
}

// readAwsConfig reads the AWS config file and returns its content.
func readAwsConfig(configPath string) (string, error) {
	// Create directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("create aws directory: %w", err)
	}

	// Create file if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(""), 0600); err != nil {
			return "", fmt.Errorf("create config file: %w", err)
		}
	}

	// Read file
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config file: %w", err)
	}

	return string(content), nil
}

// appendProfiles appends SSO profiles to the existing AWS config.
func appendProfiles(existingConfig string, profiles []ProfileConfig) (string, error) {
	var config strings.Builder

	// Ensure existing config ends with a newline
	if existingConfig != "" {
		existingConfig = strings.TrimSpace(existingConfig) + "\n\n"
	}

	// Start with existing config
	config.WriteString(existingConfig)

	// Create template
	tmpl, err := template.New("profile").Parse(profileTemplate)
	if err != nil {
		return "", fmt.Errorf("parse profile template: %w", err)
	}

	// Process each profile
	for _, profile := range profiles {
		// Check if profile already exists
		profileHeader := fmt.Sprintf("[profile %s]", profile.Name)
		if strings.Contains(existingConfig, profileHeader) {
			// Skip existing profile
			continue
		}

		// Render template
		var profileContent bytes.Buffer
		if err := tmpl.Execute(&profileContent, profile); err != nil {
			return "", fmt.Errorf("execute template for %s: %w", profile.Name, err)
		}

		// Add to config
		config.WriteString(profileContent.String())
		config.WriteString("\n")
	}

	return config.String(), nil
}

func generateCacheKey(cacheKey string) string {
	hasher := sha1.New()
	hasher.Write([]byte(cacheKey))
	return hex.EncodeToString(hasher.Sum(nil))
}
