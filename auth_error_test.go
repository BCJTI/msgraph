package msgraph

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// retrieveError builds the error the oauth2 package produces for a token
// endpoint failure.
func retrieveError(status int, code, description string) error {
	return &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: status},
		Body:             []byte(`{"error":"` + code + `","error_description":"` + description + `"}`),
		ErrorCode:        code,
		ErrorDescription: description,
	}
}

func TestClassifyTokenError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantKind   AuthErrorKind
		wantAADSTS string
		wantIs     error
	}{
		{
			name:       "expired client secret",
			err:        retrieveError(http.StatusBadRequest, "invalid_client", "AADSTS7000222: The provided client secret keys for app '1234' are expired."),
			wantKind:   AuthErrorKindClientSecretExpired,
			wantAADSTS: "AADSTS7000222",
			wantIs:     ErrClientSecretExpired,
		},
		{
			name:       "wrong client secret",
			err:        retrieveError(http.StatusUnauthorized, "invalid_client", "AADSTS7000215: Invalid client secret provided."),
			wantKind:   AuthErrorKindInvalidClient,
			wantAADSTS: "AADSTS7000215",
			wantIs:     ErrInvalidClient,
		},
		{
			name:       "application disabled",
			err:        retrieveError(http.StatusBadRequest, "unauthorized_client", "AADSTS7000112: Application is disabled."),
			wantKind:   AuthErrorKindInvalidClient,
			wantAADSTS: "AADSTS7000112",
			wantIs:     ErrInvalidClient,
		},
		{
			name:       "refresh token expired",
			err:        retrieveError(http.StatusBadRequest, "invalid_grant", "AADSTS700082: The refresh token has expired due to inactivity."),
			wantKind:   AuthErrorKindReauthRequired,
			wantAADSTS: "AADSTS700082",
			wantIs:     ErrReauthRequired,
		},
		{
			name:       "consent revoked",
			err:        retrieveError(http.StatusBadRequest, "invalid_grant", "AADSTS65001: The user or administrator has not consented."),
			wantKind:   AuthErrorKindReauthRequired,
			wantAADSTS: "AADSTS65001",
			wantIs:     ErrReauthRequired,
		},
		{
			name:     "unknown AADSTS code falls back to the OAuth2 error code",
			err:      retrieveError(http.StatusBadRequest, "invalid_grant", "AADSTS999999: Something new."),
			wantKind: AuthErrorKindReauthRequired,
			// The code is still reported even though it drives no decision.
			wantAADSTS: "AADSTS999999",
			wantIs:     ErrReauthRequired,
		},
		{
			name:     "server error is transient",
			err:      retrieveError(http.StatusInternalServerError, "server_error", "Try again."),
			wantKind: AuthErrorKindTransient,
			wantIs:   ErrTransientAuth,
		},
		{
			name:     "throttling is transient",
			err:      retrieveError(http.StatusTooManyRequests, "", "Slow down."),
			wantKind: AuthErrorKindTransient,
			wantIs:   ErrTransientAuth,
		},
		{
			name:     "unrecognized failure stays unknown",
			err:      retrieveError(http.StatusBadRequest, "some_new_code", "Nothing we know about."),
			wantKind: AuthErrorKindUnknown,
		},
		{
			name:     "timeout is transient",
			err:      context.DeadlineExceeded,
			wantKind: AuthErrorKindTransient,
			wantIs:   ErrTransientAuth,
		},
		{
			name:     "network failure is transient",
			err:      &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			wantKind: AuthErrorKindTransient,
			wantIs:   ErrTransientAuth,
		},
		{
			name:       "AADSTS code in a plain error is still recognized",
			err:        errors.New("oauth2: cannot fetch token: AADSTS7000222: secret expired"),
			wantKind:   AuthErrorKindClientSecretExpired,
			wantAADSTS: "AADSTS7000222",
			wantIs:     ErrClientSecretExpired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyTokenError(tc.err)

			var authErr *AuthError
			if !errors.As(err, &authErr) {
				t.Fatalf("classifyTokenError(%v) = %v (%T), want an *AuthError", tc.err, err, err)
			}

			if authErr.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", authErr.Kind, tc.wantKind)
			}
			if authErr.AADSTSCode != tc.wantAADSTS {
				t.Errorf("AADSTSCode = %q, want %q", authErr.AADSTSCode, tc.wantAADSTS)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(err, %v) = false", tc.wantIs)
			}

			// The original error stays reachable for callers that need the detail.
			if !errors.Is(err, tc.err) && !errors.As(err, new(*oauth2.RetrieveError)) {
				t.Errorf("underlying error %v is not reachable from %v", tc.err, err)
			}
		})
	}
}

func TestClassifyTokenErrorNil(t *testing.T) {
	if err := classifyTokenError(nil); err != nil {
		t.Errorf("classifyTokenError(nil) = %v, want nil", err)
	}
}

func TestClassifyTokenErrorPassesAuthErrorThrough(t *testing.T) {
	original := &AuthError{Kind: AuthErrorKindClientSecretExpired}

	if got := classifyTokenError(original); got != error(original) {
		t.Errorf("classifyTokenError returned %v, want the original *AuthError unchanged", got)
	}
}

func TestPermanentAndTransientAreExclusive(t *testing.T) {
	kinds := []AuthErrorKind{
		AuthErrorKindTransient,
		AuthErrorKindClientSecretExpired,
		AuthErrorKindInvalidClient,
		AuthErrorKindReauthRequired,
		AuthErrorKindMissingToken,
		AuthErrorKindUnknown,
	}

	for _, kind := range kinds {
		err := &AuthError{Kind: kind}

		if err.Permanent() == err.Retryable() {
			t.Errorf("kind %q: Permanent and Retryable both report %v", kind, err.Permanent())
		}
		if IsPermanentAuthError(err) && IsTransientAuthError(err) {
			t.Errorf("kind %q matches both ErrAuthPermanent and ErrTransientAuth", kind)
		}
	}

	// An unclassifiable failure must be retried rather than escalated.
	unknown := &AuthError{Kind: AuthErrorKindUnknown}
	if unknown.Permanent() {
		t.Error("an unknown auth failure reports Permanent, want it treated as retryable")
	}
}

func TestClassifyUnauthorized(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantKind AuthErrorKind
		wantCode string
	}{
		{
			name:     "graph token rejection",
			body:     `{"error":{"code":"InvalidAuthenticationToken","message":"Access token has expired."}}`,
			wantKind: AuthErrorKindReauthRequired,
			wantCode: "InvalidAuthenticationToken",
		},
		{
			name:     "expired secret surfaced by graph",
			body:     `{"error":{"code":"InvalidAuthenticationToken","message":"AADSTS7000222: The provided client secret keys are expired."}}`,
			wantKind: AuthErrorKindClientSecretExpired,
			wantCode: "InvalidAuthenticationToken",
		},
		{
			name:     "non-JSON body",
			body:     "Unauthorized",
			wantKind: AuthErrorKindReauthRequired,
		},
		{
			name:     "empty body",
			body:     "",
			wantKind: AuthErrorKindReauthRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyUnauthorized(http.StatusUnauthorized, []byte(tc.body))

			if err.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", err.Kind, tc.wantKind)
			}
			if err.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", err.Code, tc.wantCode)
			}
			if err.HTTPStatus != http.StatusUnauthorized {
				t.Errorf("HTTPStatus = %d, want 401", err.HTTPStatus)
			}
			if !IsPermanentAuthError(err) {
				t.Error("a 401 that survived a refresh must not be reported as retryable")
			}
		})
	}
}

func TestAuthErrorMessageNamesTheFix(t *testing.T) {
	err := &AuthError{
		Kind:        AuthErrorKindClientSecretExpired,
		Code:        "invalid_client",
		AADSTSCode:  "AADSTS7000222",
		Description: "The provided client secret keys for app '1234' are expired.",
	}

	got := err.Error()

	for _, want := range []string{"AADSTS7000222", "rotate", "Entra ID", "expired"} {
		if !strings.Contains(got, want) {
			t.Errorf("error message %q does not mention %q", got, want)
		}
	}
}

func TestAuthErrorDescriptionIsBounded(t *testing.T) {
	err := classifyTokenError(retrieveError(http.StatusBadRequest, "invalid_client", strings.Repeat("x", 4096)))

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected an *AuthError, got %T", err)
	}

	if len(authErr.Description) > maxDescriptionLen+3 {
		t.Errorf("description is %d bytes, want it truncated to %d", len(authErr.Description), maxDescriptionLen)
	}
}
