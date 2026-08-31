package msgraph

import (
	"context"
	"errors"
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

// defaultTokenHTTPTimeout bounds a call to the OAuth2 token endpoint when the
// caller has not supplied an HTTPClient. The token endpoint gets a default
// timeout that ordinary Graph calls do not because the refresh runs with the
// Client lock held: a token endpoint that accepts the connection and never
// answers would stall every goroutine sharing the Client, while an unbounded
// Graph call only blocks its own caller.
const defaultTokenHTTPTimeout = 30 * time.Second

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
	// HTTPClient is used for both Microsoft Graph calls and the OAuth2 token
	// endpoint (the refresh and the authorization code exchange). When set it is
	// used as given for both, its timeout respected and never overridden.
	//
	// When nil the two paths differ on purpose: Graph calls fall back to
	// http.DefaultClient and are not bounded by a timeout, while token endpoint
	// calls get a client carrying defaultTokenHTTPTimeout. The token request runs
	// with the Client's lock held, so an endpoint that accepts the connection and
	// never answers would stall every goroutine sharing the Client, whereas a
	// hanging Graph call only blocks its own caller.
	//
	// Supplying an HTTPClient therefore replaces that default guard as well: set a
	// timeout on it, or the token endpoint is unbounded again on the locked path.
	HTTPClient *http.Client
	// OnTokenRefresh, when set, is called with a copy of every token the Client
	// acquires: each refresh, and the authorization code exchange. Callers that
	// persist the refresh token should use this to store the rotated one, so the
	// token survives a process restart.
	//
	// It is called without the Client's lock held, so a callback may use
	// CurrentToken or SetToken. Deliveries are serialized and ordered: a delivery
	// overtaken by a newer token is dropped rather than persisted, so a callback
	// never stores a refresh token Entra ID has already rotated away.
	OnTokenRefresh func(*oauth2.Token)
	// baseURL overrides the Microsoft Graph base URL. Used by tests.
	baseURL string
	// tokenHTTPTimeout overrides defaultTokenHTTPTimeout for token endpoint calls
	// made when HTTPClient is nil. Used by tests.
	tokenHTTPTimeout time.Duration

	// refreshSeq numbers every token the Client stores, so a delivery that lost the
	// race to a newer one can be recognized as stale. Guarded by mu.
	refreshSeq uint64

	// callbackMu serializes OnTokenRefresh deliveries and guards deliveredSeq, the
	// newest token already handed to the callback. It is deliberately not mu, which
	// a callback calling CurrentToken or SetToken would deadlock on.
	callbackMu   sync.Mutex
	deliveredSeq uint64
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

// errNoRefreshToken marks the one token-acquisition failure that says nothing
// about the credentials themselves: the cached token simply cannot be renewed,
// because it carries no refresh token. A caller that already has a real
// authentication failure in hand — a Microsoft Graph 401 — reports that instead
// of this stand-in, so an ambiguous 401 is not escalated to "a human must act"
// purely because renewal was impossible.
var errNoRefreshToken = errors.New("msgraph: no refresh token available")

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
			err:         errNoRefreshToken,
		}
	}

	newToken, seq, err := c.refreshLocked(ctx)
	if err != nil {
		c.mu.Unlock()
		return "", err
	}

	accessToken := newToken.AccessToken
	delivery := cloneToken(newToken)
	callback := c.OnTokenRefresh
	c.mu.Unlock()

	c.deliverTokenRefresh(callback, delivery, seq)

	return accessToken, nil
}

// storeTokenLocked records a newly acquired token and returns its sequence
// number, which orders the OnTokenRefresh delivery that follows.
// The caller must hold c.mu.
func (c *Client) storeTokenLocked(token *oauth2.Token) uint64 {
	c.Token = token
	c.refreshSeq++

	return c.refreshSeq
}

// deliverTokenRefresh hands a newly acquired token to the OnTokenRefresh
// callback, one delivery at a time and never out of order.
//
// Two goroutines can leave ensureToken with tokens acquired in one order and
// reach the callback in the other; delivering the older one last would have the
// caller persist a refresh token Entra ID already invalidated, so it is dropped.
func (c *Client) deliverTokenRefresh(callback func(*oauth2.Token), token *oauth2.Token, seq uint64) {
	if callback == nil {
		return
	}

	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()

	if seq <= c.deliveredSeq {
		return
	}

	c.deliveredSeq = seq

	callback(token)
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

// refreshLocked exchanges the refresh token for a new access token and stores it,
// returning the token and the sequence number that orders its delivery.
// The caller must hold c.mu.
func (c *Client) refreshLocked(ctx context.Context) (*oauth2.Token, uint64, error) {
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
	source := c.config.TokenSource(c.oauthContext(ctx), &oauth2.Token{RefreshToken: c.Token.RefreshToken})

	newToken, err := source.Token()
	if err != nil {
		authErr := classifyTokenError(err)
		if c.Debug {
			fmt.Printf("[DEBUG] Refresh failed: %v\n", authErr)
		}
		return nil, 0, authErr
	}

	if c.Debug {
		fmt.Printf("[DEBUG] New token obtained, expires: %s\n", newToken.Expiry)
	}

	return newToken, c.storeTokenLocked(newToken), nil
}

// oauthContext makes the oauth2 package talk to the token endpoint through this
// Client's HTTP client. Without it oauth2 falls back to http.DefaultClient, which
// has no timeout — and the refresh runs with c.mu held, so an endpoint that never
// answers would stall every other caller indefinitely.
func (c *Client) oauthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.tokenHTTPClient())
}

// tokenHTTPClient returns the HTTP client used for OAuth2 token endpoint calls.
// A caller-supplied HTTPClient is used exactly as given, timeout included: that
// is the caller's choice. Only when none is set does the token path get its own
// client carrying defaultTokenHTTPTimeout, instead of the untimed
// http.DefaultClient the Graph path falls back to.
func (c *Client) tokenHTTPClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	timeout := c.tokenHTTPTimeout
	if timeout <= 0 {
		timeout = defaultTokenHTTPTimeout
	}

	return &http.Client{Timeout: timeout}
}

// ExchangeCodeForTokens exchanges authorization code for access and refresh tokens
func (c *Client) ExchangeCodeForTokens(ctx context.Context, code string) error {
	token, err := c.config.Exchange(c.oauthContext(ctx), code)
	if err != nil {
		return classifyTokenError(err)
	}

	c.mu.Lock()
	seq := c.storeTokenLocked(token)
	delivery := cloneToken(token)
	callback := c.OnTokenRefresh
	c.mu.Unlock()

	c.deliverTokenRefresh(callback, delivery, seq)

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
