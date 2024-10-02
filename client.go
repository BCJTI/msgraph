package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OAuthEndpoint holds OAuth2 endpoints for Microsoft Graph API
var OAuthEndpoint = oauth2.Endpoint{
	AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
}

// Config defines the configuration for Client
type Config struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURI  string   `json:"redirect_uri"`
	Scopes       []string `json:"scopes"`
}

// Client provides methods for OAuth2 and email sending
type Client struct {
	config *oauth2.Config
	Token  *oauth2.Token
}

// New creates a new Client instance
func NewClient(cfg Config) *Client {
	conf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURI,
		Scopes:       cfg.Scopes,
		Endpoint:     OAuthEndpoint,
	}

	client := &Client{config: conf}

	return client
}

// GetAuthorizationURL generates the authorization URL for user consent
func (c *Client) GetAuthorizationURL() string {
	return c.config.AuthCodeURL("state", oauth2.AccessTypeOffline)
}

// ExchangeCodeForTokens exchanges authorization code for access and refresh tokens
func (c *Client) ExchangeCodeForTokens(ctx context.Context, code string) error {
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("error exchanging code for tokens: %w", err)
	}
	c.Token = token
	return nil
}

func (c *Client) OAuthRefreshToken() error {

	if !c.Token.Valid() {
		tokenSource := c.config.TokenSource(context.Background(), c.Token)
		tmpToken, err := tokenSource.Token()
		c.Token = tmpToken
		if err != nil {
			return nil
		}
		return err
	}

	return nil
}

func (c *Client) ManualRefreshToken() error {
	if !c.Token.Valid() {
		data := url.Values{}
		data.Set("grant_type", "refresh_token")
		data.Set("client_id", c.config.ClientID)
		data.Set("client_secret", c.config.ClientSecret)
		data.Set("refresh_token", c.Token.RefreshToken)
		data.Set("scope", "https://graph.microsoft.com/.default")

		// Fazer a requisição HTTP POST
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
	}

	return nil

}
