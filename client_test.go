package msgraph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// TestStaleTokenRefreshDeliveryIsDropped covers the ordering rule for
// OnTokenRefresh: deliveries reach the callback newest-last, and one overtaken
// by a newer token is dropped. Persisting it would leave the caller holding a
// refresh token Entra ID already rotated away, which is unusable after a restart.
func TestStaleTokenRefreshDeliveryIsDropped(t *testing.T) {
	client := NewClient(Config{ClientID: "test-client"})

	var delivered []string
	callback := func(token *oauth2.Token) { delivered = append(delivered, token.RefreshToken) }

	client.deliverTokenRefresh(callback, &oauth2.Token{RefreshToken: "refresh-2"}, 2)
	// Refresh 1 finished first but lost the race to the callback: it is stale now.
	client.deliverTokenRefresh(callback, &oauth2.Token{RefreshToken: "refresh-1"}, 1)
	client.deliverTokenRefresh(callback, &oauth2.Token{RefreshToken: "refresh-3"}, 3)

	want := []string{"refresh-2", "refresh-3"}
	if len(delivered) != len(want) || delivered[0] != want[0] || delivered[1] != want[1] {
		t.Errorf("delivered %v, want %v: the overtaken refresh-1 must be dropped", delivered, want)
	}
}

// TestConcurrentRefreshesDeliverNewestTokenLast checks the same ordering rule
// end to end, where the race it guards against actually happens.
func TestConcurrentRefreshesDeliverNewestTokenLast(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, okGraph)

	client := newTestClient(tokens.URL, graph.URL, &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})

	var (
		mu        sync.Mutex
		delivered []*oauth2.Token
	)

	client.OnTokenRefresh = func(token *oauth2.Token) {
		mu.Lock()
		delivered = append(delivered, token)
		mu.Unlock()
	}

	const callers = 12

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.ForceRefreshToken(context.Background()); err != nil {
				t.Errorf("ForceRefreshToken: unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(delivered) == 0 {
		t.Fatal("no token was delivered to OnTokenRefresh")
	}

	issue := func(token *oauth2.Token) int {
		n, err := strconv.Atoi(strings.TrimPrefix(token.RefreshToken, "refresh-"))
		if err != nil {
			t.Fatalf("unexpected refresh token %q", token.RefreshToken)
		}
		return n
	}

	for i := 1; i < len(delivered); i++ {
		if issue(delivered[i]) <= issue(delivered[i-1]) {
			t.Fatalf("delivery %d carried %q after %q: a stale token reached the caller",
				i, delivered[i].RefreshToken, delivered[i-1].RefreshToken)
		}
	}

	last := delivered[len(delivered)-1].RefreshToken
	if got := client.CurrentToken().RefreshToken; got != last {
		t.Errorf("last delivered refresh token = %q, but the Client holds %q", last, got)
	}
}

// TestAmbiguousUnauthorizedStaysRetryable covers a 401 whose body names no
// reason at all. It must not be escalated as a credential problem needing a
// human — but it must still be reported after the single retry, not looped.
func TestAmbiguousUnauthorizedStaysRetryable(t *testing.T) {
	tokens := newTokenServer(t, nil)
	graph := newGraphServer(t, func(call int, token string) (int, string) {
		return http.StatusUnauthorized, "<html>401 from a proxy</html>"
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

	if got := graph.calls.Load(); got != 2 {
		t.Errorf("graph calls = %d, want 2 (one retry, no loop)", got)
	}

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("error %v (%T) is not an *AuthError", err, err)
	}
	if authErr.Kind != AuthErrorKindUnknown {
		t.Errorf("kind = %q, want %q for a 401 that names no reason", authErr.Kind, AuthErrorKindUnknown)
	}
	if IsPermanentAuthError(err) {
		t.Error("IsPermanentAuthError = true: an unexplained 401 must not be escalated as a config error")
	}
	if IsClientSecretExpired(err) {
		t.Error("IsClientSecretExpired = true, want false")
	}
	if !authErr.Retryable() {
		t.Error("Retryable = false, want true")
	}
}

// TestRefreshUsesConfiguredHTTPClient makes sure the token refresh honours
// Client.HTTPClient. The refresh runs with the Client's lock held, so a token
// endpoint that never answers would otherwise stall every caller forever.
func TestRefreshUsesConfiguredHTTPClient(t *testing.T) {
	release := make(chan struct{})

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"late","token_type":"Bearer","expires_in":3600}`)
	}))
	defer func() {
		close(release)
		slow.Close()
	}()

	client := newTestClient(slow.URL, slow.URL, &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})
	transport := &countingTransport{next: http.DefaultTransport}
	supplied := &http.Client{Timeout: 50 * time.Millisecond, Transport: transport}
	client.HTTPClient = supplied

	start := time.Now()
	err := client.EnsureValidToken(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("EnsureValidToken: expected the client timeout to abort the refresh, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("refresh took %s: Client.HTTPClient did not bound it", elapsed)
	}
	if !IsTransientAuthError(err) {
		t.Errorf("IsTransientAuthError = false for %v, want a timeout reported as retryable", err)
	}
	if got := transport.count.Load(); got == 0 {
		t.Error("the token endpoint was not reached through Client.HTTPClient's transport")
	}
	if supplied.Timeout != 50*time.Millisecond {
		t.Errorf("Client.HTTPClient.Timeout = %s, want the caller's 50ms left untouched", supplied.Timeout)
	}
}

// countingTransport counts the requests it carries, so a test can tell whether a
// caller-supplied http.Client was really the one used.
type countingTransport struct {
	next  http.RoundTripper
	count atomic.Int64
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.count.Add(1)
	return t.next.RoundTrip(req)
}

// TestRefreshTimesOutWithoutConfiguredHTTPClient covers the default configuration:
// with no HTTPClient set, a token endpoint that accepts the connection and never
// answers must not wedge the Client, whose lock the refresh holds. The timeout has
// to surface as a retryable failure, never as the expired-client-secret case a
// human would be paged for.
func TestRefreshTimesOutWithoutConfiguredHTTPClient(t *testing.T) {
	release := make(chan struct{})

	silent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() {
		close(release)
		silent.Close()
	}()

	client := newTestClient(silent.URL, silent.URL, &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	})
	if client.HTTPClient != nil {
		t.Fatal("test setup: HTTPClient must be unset to exercise the default")
	}
	client.tokenHTTPTimeout = 50 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- client.EnsureValidToken(context.Background()) }()

	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("EnsureValidToken never returned: the default token client has no timeout")
	}

	if err == nil {
		t.Fatal("EnsureValidToken: expected the default token timeout to abort the refresh, got nil")
	}
	if !IsTransientAuthError(err) {
		t.Errorf("IsTransientAuthError = false for %v, want a timeout reported as retryable", err)
	}
	if IsPermanentAuthError(err) {
		t.Errorf("IsPermanentAuthError = true for %v, want a slow token endpoint kept retryable", err)
	}
}

// TestDefaultTokenTimeoutApplied checks the constant actually reaches the client
// oauth2 uses when the caller supplies none.
func TestDefaultTokenTimeoutApplied(t *testing.T) {
	client := newTestClient("http://127.0.0.1:1/token", "http://127.0.0.1:1", nil)

	if got := client.tokenHTTPClient().Timeout; got != defaultTokenHTTPTimeout {
		t.Errorf("tokenHTTPClient().Timeout = %s, want %s", got, defaultTokenHTTPTimeout)
	}

	supplied := &http.Client{}
	client.HTTPClient = supplied
	if got := client.tokenHTTPClient(); got != supplied {
		t.Error("tokenHTTPClient() replaced the caller-supplied http.Client")
	}
}

// TestNextLinkFollowsClientEndpoint checks that an absolute @odata.nextLink is
// resolved against the base URL the Client actually talks to.
func TestNextLinkFollowsClientEndpoint(t *testing.T) {
	var (
		mu     sync.Mutex
		seen   []string
		server *httptest.Server
	)

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.RequestURI())
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"value":[]}`)
	}))
	defer server.Close()

	client := newTestClient(server.URL, server.URL, &oauth2.Token{
		AccessToken:  "still-good",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	nextLink := server.URL + "/me/messages?%24skiptoken=abc"

	if _, err := client.ListMessagesByNextLink(nextLink); err != nil {
		t.Fatalf("ListMessagesByNextLink: unexpected error: %v", err)
	}
	if _, err := client.ListEventsByNextLink(server.URL + "/me/events?%24skiptoken=def"); err != nil {
		t.Fatalf("ListEventsByNextLink: unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	want := []string{"/me/messages?%24skiptoken=abc", "/me/events?%24skiptoken=def"}
	if len(seen) != len(want) {
		t.Fatalf("server saw %v, want %v", seen, want)
	}
	for i, uri := range want {
		if seen[i] != uri {
			t.Errorf("request %d hit %q, want %q", i, seen[i], uri)
		}
	}
}

// TestSuccessBodyDecodesIntoModel covers a 2xx body whose JSON shape does not fit
// ErrMessage — here "message" is an object, not a string. The response is valid
// and must decode into the caller's model instead of failing on the error-shape
// probe that only the failure path needs.
func TestSuccessBodyDecodesIntoModel(t *testing.T) {
	graph := newGraphServer(t, func(call int, token string) (int, string) {
		return http.StatusOK, `{"id":"AAA","message":{"content":"nested"}}`
	})

	client := newTestClient("http://127.0.0.1:1/token", graph.URL, &oauth2.Token{
		AccessToken:  "still-good",
		RefreshToken: "refresh-0",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	})

	var model struct {
		ID      string `json:"id"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := client.Get("/v1.0/me/messages/AAA", nil, nil, &model); err != nil {
		t.Fatalf("Get returned %v, want the 2xx body decoded into the model", err)
	}
	if model.ID != "AAA" || model.Message.Content != "nested" {
		t.Errorf("model = %+v, want ID \"AAA\" and Message.Content \"nested\"", model)
	}
}
