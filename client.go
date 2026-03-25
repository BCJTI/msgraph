package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2"
)

// OAuthEndpoint holds OAuth2 endpoints for Microsoft Graph API.
// Microsoft requires credentials in the POST body (AuthStyleInParams).
var OAuthEndpoint = oauth2.Endpoint{
	AuthURL:   "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// Config defines the configuration for Client.
// TenantID is optional; when empty it defaults to "common" (multi-tenant).
type Config struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURI  string   `json:"redirect_uri"`
	TenantID     string   `json:"tenant_id"`
	Scopes       []string `json:"scopes"`
}

// Client provides methods for OAuth2 and Microsoft Graph API calls.
// All exported methods are safe for concurrent use by multiple goroutines.
// Set Debug to true to log HTTP requests and responses to stdout.
type Client struct {
	mu     sync.Mutex
	config *oauth2.Config
	Token  *oauth2.Token
	Debug  bool
}

// NewClient creates a new Client instance.
// If cfg.TenantID is set, tenant-specific OAuth endpoints are used;
// otherwise the default "common" (multi-tenant) endpoint is used.
func NewClient(cfg Config) *Client {
	endpoint := OAuthEndpoint
	if cfg.TenantID != "" {
		endpoint = oauth2.Endpoint{
			AuthURL:   fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", cfg.TenantID),
			TokenURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantID),
			AuthStyle: oauth2.AuthStyleInParams,
		}
	}

	conf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURI,
		Scopes:       cfg.Scopes,
		Endpoint:     endpoint,
	}

	client := &Client{config: conf}

	return client
}

// GetAuthorizationURL generates the authorization URL for user consent
func (c *Client) GetAuthorizationURL() string {
	return c.config.AuthCodeURL("state", oauth2.AccessTypeOffline)
}

// accessToken returns a valid access token string, refreshing it if expired.
// It is goroutine-safe and centralizes all token lifecycle management.
func (c *Client) accessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Token == nil {
		return "", fmt.Errorf("missing access token. Please obtain one first")
	}

	if !c.Token.Valid() {
		if c.Debug {
			fmt.Printf("[DEBUG] Token expired, refreshing...\n")
			fmt.Printf("[DEBUG] TokenURL: %s\n", c.config.Endpoint.TokenURL)
			fmt.Printf("[DEBUG] ClientID: %s\n", c.config.ClientID)
			fmt.Printf("[DEBUG] Scopes: %v\n", c.config.Scopes)
			fmt.Printf("[DEBUG] RefreshToken: %s...\n", c.Token.RefreshToken[:40])
		}
		tokenSource := c.config.TokenSource(context.Background(), c.Token)
		newToken, err := tokenSource.Token()
		if c.Debug && err != nil {
			fmt.Printf("[DEBUG] Refresh error: %v\n", err)
		}
		if err != nil {
			return "", fmt.Errorf("token refresh failed: %w", err)
		}
		if c.Debug {
			fmt.Printf("[DEBUG] New token obtained, expires: %s\n", newToken.Expiry)
		}
		c.Token = newToken
	}

	return c.Token.AccessToken, nil
}

// ExchangeCodeForTokens exchanges authorization code for access and refresh tokens
func (c *Client) ExchangeCodeForTokens(ctx context.Context, code string) error {
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("error exchanging code for tokens: %w", err)
	}
	c.mu.Lock()
	c.Token = token
	c.mu.Unlock()
	return nil
}

func (c *Client) OAuthRefreshToken() error {
	_, err := c.accessToken()
	return err
}

func (c *Client) ManualRefreshToken() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Token.Valid() {
		return nil
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)
	data.Set("refresh_token", c.Token.RefreshToken)
	data.Set("scope", "https://graph.microsoft.com/.default")

	client := &http.Client{}
	req, _ := http.NewRequest("POST", OAuthEndpoint.TokenURL, strings.NewReader(data.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if err := json.Unmarshal(body, &c.Token); err != nil {
		return err
	}

	return nil
}
