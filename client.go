package msgraph

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// OAuthEndpoint holds OAuth2 endpoints for Microsoft Graph API.
// Microsoft requires credentials in the POST body (AuthStyleInParams).
var OAuthEndpoint = oauth2.Endpoint{
	AuthURL:   "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// tokenRefreshWindow is how long before its recorded expiry the SDK proactively
// re-acquires the access token. Entra ID access tokens last about an hour, and
// this margin absorbs clock skew plus the round trip of the call about to be made.
const tokenRefreshWindow = 2 * time.Minute

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
//
// Token lifecycle is handled automatically: the Client re-acquires the access
// token from the refresh token before it expires, and again if Microsoft Graph
// still answers 401. Failures that no retry can fix — an expired client secret,
// a revoked user grant — are returned as a classified *AuthError instead
// (see IsClientSecretExpired and IsPermanentAuthError).
//
// All exported methods are safe for concurrent use by multiple goroutines. The
// exported fields are not: set them before the Client is shared across goroutines,
// and read the token back with CurrentToken rather than through the Token field.
type Client struct {
	mu     sync.Mutex
	config *oauth2.Config
	// Token holds the current OAuth2 token. Assign it once before first use;
	// afterwards the Client owns it and refreshes it in place. Use CurrentToken
	// to read it safely while the Client is in use.
	Token *oauth2.Token
	// Debug logs HTTP requests and responses to stdout. Secrets are redacted.
	Debug bool
	// HTTPClient is used for Microsoft Graph calls. When nil, http.DefaultClient
	// is used. Set one with a timeout to bound how long a Graph call may hang.
	HTTPClient *http.Client
	// OnTokenRefresh, when set, is called with a copy of the new token every time
	// the Client refreshes it. Callers that persist the refresh token should use
	// this to store the rotated one, so the token survives a process restart.
	// It is called without the Client's lock held.
	OnTokenRefresh func(*oauth2.Token)
	// baseURL overrides the Microsoft Graph base URL. Used by tests.
	baseURL string
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

// CurrentToken returns a copy of the token the Client currently holds, or nil if
// it has none. Safe to call while other goroutines are using the Client.
func (c *Client) CurrentToken() *oauth2.Token {
	c.mu.Lock()
	defer c.mu.Unlock()

	return cloneToken(c.Token)
}

// SetToken replaces the token the Client works with. Safe to call while other
// goroutines are using the Client.
func (c *Client) SetToken(token *oauth2.Token) {
	c.mu.Lock()
	c.Token = cloneToken(token)
	c.mu.Unlock()
}

// tokenRequest says how strict a caller is about reusing the cached access token.
type tokenRequest struct {
	// staleAccessToken names an access token the caller already found unusable —
	// Microsoft Graph rejected it with 401. The cached token is reused only if it
	// differs from this value, meaning another goroutine already refreshed it.
	staleAccessToken string

	// force skips the cache entirely and always re-acquires the token.
	force bool
}

// ensureToken returns a usable access token, refreshing it when needed.
//
// The lock is held across the refresh on purpose: it collapses a burst of
// concurrent callers into a single token request, which matters because Entra ID
// rotates the refresh token on every use.
func (c *Client) ensureToken(ctx context.Context, req tokenRequest) (string, error) {
	c.mu.Lock()

	if c.Token == nil {
		c.mu.Unlock()
		return "", &AuthError{
			Kind:        AuthErrorKindMissingToken,
			Description: "no OAuth2 token set; obtain one with ExchangeCodeForTokens or assign Client.Token",
		}
	}

	if tokenUsable(c.Token, req) {
		token := c.Token.AccessToken
		c.mu.Unlock()
		return token, nil
	}

	if c.Token.RefreshToken == "" {
		c.mu.Unlock()
		return "", &AuthError{
			Kind:        AuthErrorKindReauthRequired,
			Description: "the access token needs renewing and no refresh token is available",
		}
	}

	newToken, err := c.refreshLocked(ctx)
	if err != nil {
		c.mu.Unlock()
		return "", err
	}

	callback := c.OnTokenRefresh
	c.mu.Unlock()

	if callback != nil {
		callback(cloneToken(newToken))
	}

	return newToken.AccessToken, nil
}

// tokenUsable reports whether a cached token can be used as-is. Callers read it
// under the Client's lock, since the token it inspects is the cached one.
func tokenUsable(token *oauth2.Token, req tokenRequest) bool {
	if req.force || token.AccessToken == "" {
		return false
	}

	// The caller was rejected while holding this exact token, so it is not usable
	// regardless of what its expiry claims. A different value means another
	// goroutine already refreshed it and this caller can use the new one.
	if req.staleAccessToken != "" && token.AccessToken == req.staleAccessToken {
		return false
	}

	// A zero expiry means the issuer did not say when the token expires; oauth2
	// treats that as "never expires" and so do we. The 401 retry recovers if the
	// assumption turns out to be wrong.
	if token.Expiry.IsZero() {
		return true
	}

	return time.Now().Add(tokenRefreshWindow).Before(token.Expiry)
}

// refreshLocked exchanges the refresh token for a new access token and stores it.
// The caller must hold c.mu.
func (c *Client) refreshLocked(ctx context.Context) (*oauth2.Token, error) {
	if c.Debug {
		fmt.Printf("[DEBUG] Refreshing access token\n")
		fmt.Printf("[DEBUG] TokenURL: %s\n", c.config.Endpoint.TokenURL)
		fmt.Printf("[DEBUG] ClientID: %s\n", c.config.ClientID)
		fmt.Printf("[DEBUG] Scopes: %v\n", c.config.Scopes)
		fmt.Printf("[DEBUG] RefreshToken: %s\n", redact(c.Token.RefreshToken))
	}

	// A token source seeded with only the refresh token always performs the
	// refresh; passing the cached token would make oauth2 hand it back unchanged
	// whenever it still looks valid, which defeats the forced-refresh path.
	source := c.config.TokenSource(ctx, &oauth2.Token{RefreshToken: c.Token.RefreshToken})

	newToken, err := source.Token()
	if err != nil {
		authErr := classifyTokenError(err)
		if c.Debug {
			fmt.Printf("[DEBUG] Refresh failed: %v\n", authErr)
		}
		return nil, authErr
	}

	if c.Debug {
		fmt.Printf("[DEBUG] New token obtained, expires: %s\n", newToken.Expiry)
	}

	c.Token = newToken

	return newToken, nil
}

// ExchangeCodeForTokens exchanges authorization code for access and refresh tokens
func (c *Client) ExchangeCodeForTokens(ctx context.Context, code string) error {
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		return classifyTokenError(err)
	}

	c.mu.Lock()
	c.Token = token
	callback := c.OnTokenRefresh
	c.mu.Unlock()

	if callback != nil {
		callback(cloneToken(token))
	}

	return nil
}

// EnsureValidToken makes sure the Client holds a usable access token, refreshing
// it if it is expired or about to expire. Callers do not need to invoke it: every
// Graph call does this on its own. It is useful to validate credentials up front.
//
// A returned *AuthError says whether the failure is worth retrying; see
// IsPermanentAuthError and IsClientSecretExpired.
func (c *Client) EnsureValidToken(ctx context.Context) error {
	_, err := c.ensureToken(ctx, tokenRequest{})
	return err
}

// ForceRefreshToken re-acquires the access token from the refresh token even when
// the cached one still looks valid.
func (c *Client) ForceRefreshToken(ctx context.Context) error {
	_, err := c.ensureToken(ctx, tokenRequest{force: true})
	return err
}

// OAuthRefreshToken refreshes the access token if it is expired or about to expire.
//
// Deprecated: token refresh is automatic on every Graph call. Use EnsureValidToken
// when you want to validate credentials explicitly.
func (c *Client) OAuthRefreshToken() error {
	return c.EnsureValidToken(context.Background())
}

// ManualRefreshToken refreshes the access token if it is expired or about to expire.
//
// Deprecated: token refresh is automatic on every Graph call. Use EnsureValidToken,
// or ForceRefreshToken to refresh unconditionally.
func (c *Client) ManualRefreshToken() error {
	return c.EnsureValidToken(context.Background())
}

func cloneToken(token *oauth2.Token) *oauth2.Token {
	if token == nil {
		return nil
	}

	clone := *token

	return &clone
}

// redact renders a secret as a short, non-reversible hint for debug logs.
func redact(secret string) string {
	if secret == "" {
		return "<empty>"
	}

	const visible = 6
	if len(secret) <= visible {
		return fmt.Sprintf("<redacted, %d chars>", len(secret))
	}

	return fmt.Sprintf("%s...<redacted, %d chars>", secret[:visible], len(secret))
}
