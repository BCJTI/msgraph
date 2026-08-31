package msgraph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// secretExpiredBody is the response Entra ID returns once the app registration's
// client secret has expired: the failure that triggered this work in production.
const secretExpiredBody = `{
  "error": "invalid_client",
  "error_description": "AADSTS7000222: The provided client secret keys for app '11111111-2222-3333-4444-555555555555' are expired. Visit the Azure portal to create new keys for your app: https://aka.ms/NewClientSecret.\r\nTrace ID: 0f0f0f0f-0000-0000-0000-000000000000\r\nTimestamp: 2026-08-31 12:00:00Z",
  "error_codes": [7000222]
}`

// tokenServer is a stand-in for the Entra ID token endpoint.
type tokenServer struct {
	*httptest.Server

	// calls counts refresh_token grants received.
	calls atomic.Int32

	mu      sync.Mutex
	failure func(issue int) (status int, body string)
}

// newTokenServer starts a token endpoint that issues a distinct access token on
// every call. When failure is non-nil and returns a non-zero status for the given
// (1-based) call number, that error response is served instead.
func newTokenServer(t *testing.T, failure func(issue int) (int, string)) *tokenServer {
	t.Helper()

	ts := &tokenServer{failure: failure}

	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("token endpoint: parsing form: %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Errorf("token endpoint: grant_type = %q, want refresh_token", got)
		}
		if r.PostForm.Get("refresh_token") == "" {
			t.Error("token endpoint: refresh_token is empty")
		}

		issue := int(ts.calls.Add(1))

		ts.mu.Lock()
		failure := ts.failure
		ts.mu.Unlock()

		if failure != nil {
			if status, body := failure(issue); status != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, body)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","refresh_token":%q,"expires_in":3600}`,
			fmt.Sprintf("access-%d", issue), fmt.Sprintf("refresh-%d", issue))
	}))

	t.Cleanup(ts.Close)

	return ts
}

// graphServer is a stand-in for the Microsoft Graph API.
type graphServer struct {
	*httptest.Server

	// calls counts requests received, and tokens records the bearer token of each.
	calls  atomic.Int32
	mu     sync.Mutex
	tokens []string
}

// newGraphServer starts a Graph endpoint driven by respond, which is given the
// (1-based) request number and the bearer token it carried.
func newGraphServer(t *testing.T, respond func(call int, token string) (int, string)) *graphServer {
	t.Helper()

	gs := &graphServer{}

	gs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

		call := int(gs.calls.Add(1))

		gs.mu.Lock()
		gs.tokens = append(gs.tokens, token)
		gs.mu.Unlock()

		status, body := respond(call, token)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))

	t.Cleanup(gs.Close)

	return gs
}

func (gs *graphServer) seenTokens() []string {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	return append([]string(nil), gs.tokens...)
}

// newTestClient builds a Client wired to the fake token and Graph endpoints.
func newTestClient(tokenURL, graphURL string, token *oauth2.Token) *Client {
	client := NewClient(Config{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "http://localhost/callback",
		Scopes:       []string{"Mail.Read", "offline_access"},
	})

	client.config.Endpoint.TokenURL = tokenURL
	client.baseURL = graphURL
	client.Token = token

	return client
}

func okGraph(call int, token string) (int, string) { return http.StatusOK, `{"value":[]}` }

const unauthorizedBody = `{"error":{"code":"InvalidAuthenticationToken","message":"Access token has expired or is not yet valid."}}`

// TestTokenRefreshedBeforeExpiry covers the normal case: the cached access token
// is inside the refresh window, so the SDK re-acquires one and the caller's Graph
// call succeeds without anything being restarted.
func TestTokenRefreshedBeforeExpiry(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	var refreshed []*oauth2.Token

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "about-to-expire",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		// Still valid by the clock, but inside the refresh window.
		Expiry: time.Now().Add(30 * time.Second),
	})
	client.OnTokenRefresh = func(token *oauth2.Token) { refreshed = append(refreshed, token) }

	var result struct {
		Value []string `json:"value"`
	}

	if err := client.Get("/me/messages", nil, nil, &result); err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}

	if got := tokens.calls.Load(); got != 1 {
		t.Errorf("token endpoint calls = %d, want 1", got)
	}

	if got := graph.seenTokens(); len(got) != 1 || got[0] != "access-1" {
		t.Errorf("graph saw tokens %v, want [access-1]", got)
	}

	current := client.CurrentToken()
	if current.AccessToken != "access-1" {
		t.Errorf("cached access token = %q, want access-1", current.AccessToken)
	}
	if current.RefreshToken != "refresh-1" {
		t.Errorf("cached refresh token = %q, want the rotated refresh-1", current.RefreshToken)
	}

	if len(refreshed) != 1 || refreshed[0].RefreshToken != "refresh-1" {
		t.Errorf("OnTokenRefresh got %d callbacks with %+v, want one carrying refresh-1", len(refreshed), refreshed)
	}
}

// TestExpiredTokenRefreshed covers a token that is already past its expiry.
func TestExpiredTokenRefreshed(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "long-expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})

	var result struct{}

	if err := client.Get("/me/messages", nil, nil, &result); err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}

	if got := graph.seenTokens(); len(got) != 1 || got[0] != "access-1" {
		t.Errorf("graph saw tokens %v, want [access-1]", got)
	}
}

// TestFreshTokenNotRefreshed makes sure a token with plenty of life left is
// reused instead of burning a refresh on every call.
func TestFreshTokenNotRefreshed(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "still-good",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	var result struct{}

	for i := 0; i < 3; i++ {
		if err := client.Get("/me/messages", nil, nil, &result); err != nil {
			t.Fatalf("Get %d: unexpected error: %v", i, err)
		}
	}

	if got := tokens.calls.Load(); got != 0 {
		t.Errorf("token endpoint calls = %d, want 0", got)
	}

	for i, token := range graph.seenTokens() {
		if token != "still-good" {
			t.Errorf("graph call %d used token %q, want still-good", i, token)
		}
	}
}

// TestUnauthorizedTriggersRefreshAndRetry covers a token that Graph rejects even
// though its recorded expiry says it is fine: the SDK re-acquires one and retries
// the call transparently.
func TestUnauthorizedTriggersRefreshAndRetry(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, func(call int, token string) (int, string) {
		if call == 1 {
			return http.StatusUnauthorized, unauthorizedBody
		}
		return http.StatusOK, `{"value":[]}`
	})

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "revoked-but-unexpired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	var result struct{}

	if err := client.Get("/me/messages", nil, nil, &result); err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}

	if got := tokens.calls.Load(); got != 1 {
		t.Errorf("token endpoint calls = %d, want 1", got)
	}

	want := []string{"revoked-but-unexpired", "access-1"}
	got := graph.seenTokens()
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("graph saw tokens %v, want %v", got, want)
	}
}

// TestUnauthorizedRetriedOnlyOnce makes sure a persistent 401 stops after a
// single retry and surfaces as a classified error rather than an opaque body.
func TestUnauthorizedRetriedOnlyOnce(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, func(call int, token string) (int, string) {
		return http.StatusUnauthorized, unauthorizedBody
	})

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "not-accepted",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	var result struct{}

	err := client.Get("/me/messages", nil, nil, &result)
	if err == nil {
		t.Fatal("Get: expected an error, got nil")
	}

	if got := graph.calls.Load(); got != 2 {
		t.Errorf("graph calls = %d, want 2 (one retry, no loop)", got)
	}

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("error %v (%T) is not an *AuthError", err, err)
	}
	if authErr.Kind != AuthErrorKindReauthRequired {
		t.Errorf("kind = %q, want %q", authErr.Kind, AuthErrorKindReauthRequired)
	}
	if authErr.Code != "InvalidAuthenticationToken" {
		t.Errorf("code = %q, want InvalidAuthenticationToken", authErr.Code)
	}
	if !IsPermanentAuthError(err) {
		t.Error("IsPermanentAuthError = false, want true: the caller must stop retrying")
	}
	if IsClientSecretExpired(err) {
		t.Error("IsClientSecretExpired = true, want false: the secret is fine here")
	}
}

// TestExpiredClientSecretOnRefresh is the production failure: the access token
// needs renewing, but the app registration's client secret has expired, so no
// retry can recover. The SDK must say so unmistakably instead of pretending to
// succeed or retrying forever.
func TestExpiredClientSecretOnRefresh(t *testing.T) {
	tokens := newTokenServer(t, func(int) (int, string) {
		return http.StatusBadRequest, secretExpiredBody
	})
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})

	var result struct{}

	err := client.Get("/me/messages", nil, nil, &result)
	if err == nil {
		t.Fatal("Get: expected an error, got nil")
	}

	if got := tokens.calls.Load(); got != 1 {
		t.Errorf("token endpoint calls = %d, want 1: the SDK must not retry an expired secret", got)
	}
	if got := graph.calls.Load(); got != 0 {
		t.Errorf("graph calls = %d, want 0: the call must not go out without a token", got)
	}

	if !IsClientSecretExpired(err) {
		t.Fatalf("IsClientSecretExpired = false for %v", err)
	}
	if !IsPermanentAuthError(err) {
		t.Error("IsPermanentAuthError = false, want true")
	}
	if IsTransientAuthError(err) {
		t.Error("IsTransientAuthError = true, want false")
	}
	if IsReauthRequired(err) {
		t.Error("IsReauthRequired = true, want false: re-consent does not fix an expired secret")
	}

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("error %v (%T) is not an *AuthError", err, err)
	}
	if authErr.AADSTSCode != "AADSTS7000222" {
		t.Errorf("AADSTSCode = %q, want AADSTS7000222", authErr.AADSTSCode)
	}
	if authErr.Code != "invalid_client" {
		t.Errorf("Code = %q, want invalid_client", authErr.Code)
	}
	if authErr.Retryable() {
		t.Error("Retryable = true, want false")
	}
	if !strings.Contains(authErr.Error(), "rotate it in the Entra ID app registration") {
		t.Errorf("error message %q does not tell the operator what to do", authErr.Error())
	}

	// The caller must not be left thinking the token was renewed.
	if got := client.CurrentToken().AccessToken; got != "expired" {
		t.Errorf("cached access token = %q, want the failed refresh to leave it untouched", got)
	}
}

// TestExpiredClientSecretAfterUnauthorized covers the same expired secret
// reaching the caller through the 401 retry path rather than through expiry.
func TestExpiredClientSecretAfterUnauthorized(t *testing.T) {
	tokens := newTokenServer(t, func(int) (int, string) {
		return http.StatusBadRequest, secretExpiredBody
	})
	graph := newGraphServer(t, func(call int, token string) (int, string) {
		return http.StatusUnauthorized, unauthorizedBody
	})

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "stale-but-unexpired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	var result struct{}

	err := client.Get("/me/messages", nil, nil, &result)
	if err == nil {
		t.Fatal("Get: expected an error, got nil")
	}

	if !IsClientSecretExpired(err) {
		t.Fatalf("IsClientSecretExpired = false for %v", err)
	}
	if got := graph.calls.Load(); got != 1 {
		t.Errorf("graph calls = %d, want 1: the retry cannot go out without a token", got)
	}
}

// TestTransientRefreshFailureIsRetryable makes sure an Entra ID outage is not
// mistaken for a credential that needs a human.
func TestTransientRefreshFailureIsRetryable(t *testing.T) {
	tokens := newTokenServer(t, func(int) (int, string) {
		return http.StatusServiceUnavailable, `{"error":"temporarily_unavailable","error_description":"Service is temporarily unavailable."}`
	})
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})

	err := client.EnsureValidToken(context.Background())
	if err == nil {
		t.Fatal("EnsureValidToken: expected an error, got nil")
	}

	if !IsTransientAuthError(err) {
		t.Errorf("IsTransientAuthError = false for %v", err)
	}
	if IsPermanentAuthError(err) {
		t.Error("IsPermanentAuthError = true, want false")
	}
	if IsClientSecretExpired(err) {
		t.Error("IsClientSecretExpired = true, want false")
	}
}

// TestRevokedGrantRequiresReauth covers a refresh token Entra ID no longer
// accepts: a human is needed, but a different one, and for a different reason.
func TestRevokedGrantRequiresReauth(t *testing.T) {
	tokens := newTokenServer(t, func(int) (int, string) {
		return http.StatusBadRequest, `{"error":"invalid_grant","error_description":"AADSTS700082: The refresh token has expired due to inactivity."}`
	})
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})

	err := client.EnsureValidToken(context.Background())
	if err == nil {
		t.Fatal("EnsureValidToken: expected an error, got nil")
	}

	if !IsReauthRequired(err) {
		t.Errorf("IsReauthRequired = false for %v", err)
	}
	if IsClientSecretExpired(err) {
		t.Error("IsClientSecretExpired = true, want false")
	}
	if !IsPermanentAuthError(err) {
		t.Error("IsPermanentAuthError = false, want true")
	}
}

func TestMissingTokenIsDistinguishable(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, nil)

	var result struct{}

	err := client.Get("/me/messages", nil, nil, &result)
	if err == nil {
		t.Fatal("Get: expected an error, got nil")
	}

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("error %v (%T) is not an *AuthError", err, err)
	}
	if authErr.Kind != AuthErrorKindMissingToken {
		t.Errorf("kind = %q, want %q", authErr.Kind, AuthErrorKindMissingToken)
	}
	if !IsPermanentAuthError(err) {
		t.Error("IsPermanentAuthError = false, want true")
	}
}

func TestExpiredTokenWithoutRefreshTokenRequiresReauth(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken: "expired",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(-time.Hour),
	})

	err := client.EnsureValidToken(context.Background())
	if err == nil {
		t.Fatal("EnsureValidToken: expected an error, got nil")
	}

	if !IsReauthRequired(err) {
		t.Errorf("IsReauthRequired = false for %v", err)
	}
	if got := tokens.calls.Load(); got != 0 {
		t.Errorf("token endpoint calls = %d, want 0: there is nothing to refresh with", got)
	}
}

// TestForceRefreshToken checks the explicit refresh path, which ignores how much
// life the cached token has left.
func TestForceRefreshToken(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "still-good",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	if err := client.ForceRefreshToken(context.Background()); err != nil {
		t.Fatalf("ForceRefreshToken: unexpected error: %v", err)
	}

	if got := client.CurrentToken().AccessToken; got != "access-1" {
		t.Errorf("access token = %q, want access-1", got)
	}
}

// TestConcurrentCallsRefreshOnce checks that a burst of concurrent callers
// hitting an expired token produces a single refresh. Entra ID rotates the
// refresh token on every use, so a stampede would invalidate the ones in flight.
func TestConcurrentCallsRefreshOnce(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})

	const callers = 12

	var wg sync.WaitGroup
	errs := make([]error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var result struct{}
			errs[i] = client.Get("/me/messages", nil, nil, &result)
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: unexpected error: %v", i, err)
		}
	}

	if got := tokens.calls.Load(); got != 1 {
		t.Errorf("token endpoint calls = %d, want 1", got)
	}
	if got := graph.calls.Load(); got != callers {
		t.Errorf("graph calls = %d, want %d", got, callers)
	}
}

// TestExchangeCodeForTokensClassifiesFailure makes sure the initial exchange
// reports an expired secret as clearly as the refresh path does.
func TestExchangeCodeForTokensClassifiesFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, secretExpiredBody)
	}))
	defer server.Close()

	client := newTestClient(server.URL, server.URL, nil)

	err := client.ExchangeCodeForTokens(context.Background(), "some-code")
	if err == nil {
		t.Fatal("ExchangeCodeForTokens: expected an error, got nil")
	}

	if !IsClientSecretExpired(err) {
		t.Errorf("IsClientSecretExpired = false for %v", err)
	}
}

// TestExecuteRawRetriesOnUnauthorized covers the binary download path, which
// builds its requests separately from the JSON one.
func TestExecuteRawRetriesOnUnauthorized(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, func(call int, token string) (int, string) {
		if call == 1 {
			return http.StatusUnauthorized, unauthorizedBody
		}
		return http.StatusOK, "raw-bytes"
	})

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "stale-but-unexpired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	data, err := client.GetRaw("/me/messages/1/attachments/2/$value", nil)
	if err != nil {
		t.Fatalf("GetRaw: unexpected error: %v", err)
	}

	if string(data) != "raw-bytes" {
		t.Errorf("data = %q, want raw-bytes", data)
	}
	if got := graph.calls.Load(); got != 2 {
		t.Errorf("graph calls = %d, want 2", got)
	}
}

// TestRawRequestSendsNoJSONHeaders guards the binary path against asking Graph
// for a JSON envelope, which would replace the attachment bytes.
func TestRawRequestSendsNoJSONHeaders(t *testing.T) {
	client := newTestClient("http://token.invalid", "http://graph.invalid", nil)

	httpReq, err := client.build(context.Background(), request{
		method: http.MethodGet,
		path:   "/me/messages/1/attachments/2/$value",
	}, "token")
	if err != nil {
		t.Fatalf("build: unexpected error: %v", err)
	}

	if got := httpReq.Header.Get("Accept"); got != "" {
		t.Errorf("Accept = %q, want it unset on a raw request", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer token" {
		t.Errorf("Authorization = %q, want Bearer token", got)
	}
}

// TestNonAuthErrorsAreUnchanged makes sure ordinary Graph failures keep flowing
// through as before instead of being reclassified as auth problems.
func TestNonAuthErrorsAreUnchanged(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, func(call int, token string) (int, string) {
		return http.StatusForbidden, `{"error":{"code":"ErrorAccessDenied","message":"Access is denied."}}`
	})

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "still-good",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	var result struct{}

	err := client.Get("/me/messages", nil, nil, &result)
	if err == nil {
		t.Fatal("Get: expected an error, got nil")
	}

	var authErr *AuthError
	if errors.As(err, &authErr) {
		t.Errorf("403 was reported as an *AuthError (%v); it is a permissions problem, not an auth one", authErr)
	}
	if !strings.Contains(err.Error(), "ErrorAccessDenied") {
		t.Errorf("error %q lost the Graph error code", err)
	}
	if got := graph.calls.Load(); got != 1 {
		t.Errorf("graph calls = %d, want 1: a 403 must not be retried", got)
	}
}

// TestRedactHidesSecrets guards the debug logging against leaking a refresh token.
func TestRedactHidesSecrets(t *testing.T) {
	secret := "1.AW8B4De4TKrhIUer8O1gCkPib68HOc7OG3JO"

	got := redact(secret)
	if strings.Contains(got, secret) {
		t.Errorf("redact(%q) = %q, which still contains the secret", secret, got)
	}

	// Short values must not panic or be echoed verbatim.
	if got := redact("abc"); strings.Contains(got, "abc") {
		t.Errorf("redact(\"abc\") = %q, which still contains the secret", got)
	}
	if got := redact(""); got != "<empty>" {
		t.Errorf("redact(\"\") = %q, want <empty>", got)
	}
}
