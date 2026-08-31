package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/oauth2"
)

// AuthErrorKind classifies an authentication failure by the action it requires.
//
// The distinction that matters to callers is self-healing versus not: the SDK
// re-acquires expired access tokens on its own, but nothing it can do recovers
// from an expired client secret or a revoked user grant.
type AuthErrorKind string

const (
	// AuthErrorKindTransient covers failures expected to clear on their own:
	// network problems, throttling, or an Entra ID server error. Safe to retry.
	AuthErrorKindTransient AuthErrorKind = "transient"

	// AuthErrorKindClientSecretExpired means the application's client secret has
	// expired (AADSTS7000222). Retrying never recovers from this: a human must
	// rotate the secret in the Entra ID app registration and update the Config.
	AuthErrorKindClientSecretExpired AuthErrorKind = "client_secret_expired"

	// AuthErrorKindInvalidClient means the app registration or its credentials are
	// rejected for a reason other than expiry: wrong secret, wrong tenant, or a
	// disabled/missing application. Also needs a human.
	AuthErrorKindInvalidClient AuthErrorKind = "invalid_client"

	// AuthErrorKindReauthRequired means the user grant is gone: the refresh token
	// expired or was revoked, or extra consent/MFA is required. The user has to
	// walk the authorization code flow again.
	AuthErrorKindReauthRequired AuthErrorKind = "reauth_required"

	// AuthErrorKindMissingToken means no usable token was supplied to the Client.
	AuthErrorKindMissingToken AuthErrorKind = "missing_token"

	// AuthErrorKindUnknown is an authentication failure the SDK could not classify.
	// It is treated as retryable so an unrecognized blip is never mistaken for a
	// configuration problem.
	AuthErrorKindUnknown AuthErrorKind = "unknown"
)

// Sentinel errors for use with errors.Is. Every authentication failure the SDK
// returns is an *AuthError that matches exactly one of the kind sentinels, plus
// either ErrAuthPermanent or ErrTransientAuth.
var (
	// ErrClientSecretExpired reports that the app registration's client secret
	// expired and must be rotated by a human.
	ErrClientSecretExpired = errors.New("msgraph: client secret expired")

	// ErrInvalidClient reports that the app registration or its credentials were
	// rejected for a reason other than secret expiry.
	ErrInvalidClient = errors.New("msgraph: invalid client credentials")

	// ErrReauthRequired reports that the user must authorize the application again.
	ErrReauthRequired = errors.New("msgraph: re-authorization required")

	// ErrMissingToken reports that the Client has no token to work with.
	ErrMissingToken = errors.New("msgraph: missing OAuth2 token")

	// ErrTransientAuth reports an authentication failure that is expected to
	// resolve on its own.
	ErrTransientAuth = errors.New("msgraph: transient authentication failure")

	// ErrAuthPermanent reports an authentication failure that will not recover on
	// its own, no matter how often the caller retries. It is the coarse check for
	// "this needs a human" — alert on it, do not back off and retry.
	ErrAuthPermanent = errors.New("msgraph: permanent authentication failure")
)

// AuthError describes an OAuth2/Entra ID authentication failure, classified so
// callers can tell a transient hiccup apart from a credential that needs human
// intervention. It implements Is for the sentinels above and unwraps to the
// underlying error (often *oauth2.RetrieveError).
type AuthError struct {
	// Kind is the classification callers should branch on.
	Kind AuthErrorKind

	// Code is RFC 6749's "error" parameter, e.g. "invalid_client", or the
	// Microsoft Graph error code for failures on the Graph side.
	Code string

	// AADSTSCode is the Entra ID error code, e.g. "AADSTS7000222", when present.
	AADSTSCode string

	// Description is the human-readable message returned by the server.
	Description string

	// HTTPStatus is the status code of the failed response, or 0 if there was none.
	HTTPStatus int

	err error
}

// maxDescriptionLen bounds the server text copied into an AuthError so a large
// error body cannot bloat logs.
const maxDescriptionLen = 512

func (e *AuthError) Error() string {
	var b strings.Builder

	b.WriteString("msgraph: ")
	b.WriteString(kindSummary(e.Kind))

	switch {
	case e.AADSTSCode != "":
		fmt.Fprintf(&b, " [%s]", e.AADSTSCode)
	case e.Code != "":
		fmt.Fprintf(&b, " [%s]", e.Code)
	}

	if e.Description != "" {
		b.WriteString(": ")
		b.WriteString(e.Description)
	} else if e.err != nil {
		b.WriteString(": ")
		b.WriteString(e.err.Error())
	}

	return b.String()
}

// Unwrap exposes the underlying transport error, so callers can reach for
// *oauth2.RetrieveError or a net.Error with errors.As.
func (e *AuthError) Unwrap() error { return e.err }

// Is matches the package sentinel errors against this error's kind.
func (e *AuthError) Is(target error) bool {
	switch target {
	case ErrClientSecretExpired:
		return e.Kind == AuthErrorKindClientSecretExpired
	case ErrInvalidClient:
		return e.Kind == AuthErrorKindInvalidClient
	case ErrReauthRequired:
		return e.Kind == AuthErrorKindReauthRequired
	case ErrMissingToken:
		return e.Kind == AuthErrorKindMissingToken
	case ErrTransientAuth:
		return e.Kind == AuthErrorKindTransient
	case ErrAuthPermanent:
		return e.Permanent()
	}
	return false
}

// Permanent reports whether the failure needs human intervention: retrying it
// will keep failing until someone rotates a secret, fixes the app registration,
// or has the user authorize the application again.
func (e *AuthError) Permanent() bool {
	switch e.Kind {
	case AuthErrorKindClientSecretExpired,
		AuthErrorKindInvalidClient,
		AuthErrorKindReauthRequired,
		AuthErrorKindMissingToken:
		return true
	default:
		return false
	}
}

// Retryable reports whether the caller may retry the operation later. Failures
// the SDK could not classify count as retryable, so an unrecognized transient
// fault is never escalated as a configuration problem.
func (e *AuthError) Retryable() bool { return !e.Permanent() }

func kindSummary(kind AuthErrorKind) string {
	switch kind {
	case AuthErrorKindTransient:
		return "transient authentication failure, retry later"
	case AuthErrorKindClientSecretExpired:
		return "client secret expired, rotate it in the Entra ID app registration"
	case AuthErrorKindInvalidClient:
		return "client credentials rejected, check the app registration"
	case AuthErrorKindReauthRequired:
		return "user re-authorization required, run the authorization code flow again"
	case AuthErrorKindMissingToken:
		return "no OAuth2 token available"
	default:
		return "authentication failed"
	}
}

// IsClientSecretExpired reports whether err was caused by an expired client
// secret, the one failure a human must fix by rotating the secret in Entra ID.
func IsClientSecretExpired(err error) bool { return errors.Is(err, ErrClientSecretExpired) }

// IsReauthRequired reports whether err means the user must authorize the
// application again.
func IsReauthRequired(err error) bool { return errors.Is(err, ErrReauthRequired) }

// IsPermanentAuthError reports whether err is an authentication failure that
// will not recover on its own.
func IsPermanentAuthError(err error) bool { return errors.Is(err, ErrAuthPermanent) }

// IsTransientAuthError reports whether err is an authentication failure that is
// expected to clear on its own.
func IsTransientAuthError(err error) bool { return errors.Is(err, ErrTransientAuth) }

// aadstsPattern extracts the Entra ID error code from an error description or
// response body. Entra ID reports the code as free text inside the description.
var aadstsPattern = regexp.MustCompile(`AADSTS\d+`)

// aadstsKinds maps the Entra ID error codes the SDK can act on to the action
// they require. Codes not listed here fall back to the OAuth2 error code.
var aadstsKinds = map[string]AuthErrorKind{
	// The client secret itself is expired or rejected: only a human can fix it.
	"AADSTS7000222": AuthErrorKindClientSecretExpired,
	"AADSTS7000215": AuthErrorKindInvalidClient, // invalid client secret provided
	"AADSTS7000112": AuthErrorKindInvalidClient, // application disabled
	"AADSTS700016":  AuthErrorKindInvalidClient, // application not found in directory
	"AADSTS700009":  AuthErrorKindInvalidClient, // resource app not found in tenant
	"AADSTS90002":   AuthErrorKindInvalidClient, // tenant not found
	"AADSTS900023":  AuthErrorKindInvalidClient, // invalid tenant identifier

	// The user grant is gone or insufficient: the user must consent again.
	"AADSTS50173":  AuthErrorKindReauthRequired, // credentials changed after token issue
	"AADSTS700082": AuthErrorKindReauthRequired, // refresh token expired (inactivity)
	"AADSTS700084": AuthErrorKindReauthRequired, // refresh token issued to a single-page app
	"AADSTS54005":  AuthErrorKindReauthRequired, // authorization code already redeemed
	"AADSTS65001":  AuthErrorKindReauthRequired, // user has not consented
	"AADSTS50076":  AuthErrorKindReauthRequired, // MFA required
	"AADSTS50079":  AuthErrorKindReauthRequired, // MFA enrollment required
	"AADSTS50158":  AuthErrorKindReauthRequired, // external security challenge required
	"AADSTS50055":  AuthErrorKindReauthRequired, // password expired
}

// oauthCodeKinds maps RFC 6749 error codes to a classification, used when the
// response carries no recognized AADSTS code.
var oauthCodeKinds = map[string]AuthErrorKind{
	"invalid_client":          AuthErrorKindInvalidClient,
	"unauthorized_client":     AuthErrorKindInvalidClient,
	"invalid_scope":           AuthErrorKindInvalidClient,
	"invalid_grant":           AuthErrorKindReauthRequired,
	"interaction_required":    AuthErrorKindReauthRequired,
	"consent_required":        AuthErrorKindReauthRequired,
	"login_required":          AuthErrorKindReauthRequired,
	"server_error":            AuthErrorKindTransient,
	"temporarily_unavailable": AuthErrorKindTransient,
}

// classifyAuthKind picks a kind from the strongest available signal: the Entra
// ID code first, then the OAuth2 error code, then the HTTP status.
func classifyAuthKind(aadstsCode, oauthCode string, httpStatus int) AuthErrorKind {
	if kind, ok := aadstsKinds[aadstsCode]; ok {
		return kind
	}
	if kind, ok := oauthCodeKinds[oauthCode]; ok {
		return kind
	}
	if httpStatus == http.StatusTooManyRequests || httpStatus >= http.StatusInternalServerError {
		return AuthErrorKindTransient
	}
	return AuthErrorKindUnknown
}

// classifyTokenError converts an error from the OAuth2 token endpoint into a
// classified *AuthError. It returns nil for a nil error and passes an existing
// *AuthError through unchanged.
func classifyTokenError(err error) error {
	if err == nil {
		return nil
	}

	var already *AuthError
	if errors.As(err, &already) {
		return err
	}

	authErr := &AuthError{Kind: AuthErrorKindUnknown, err: err}

	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		authErr.Code = retrieveErr.ErrorCode
		authErr.Description = truncate(strings.TrimSpace(retrieveErr.ErrorDescription), maxDescriptionLen)
		if retrieveErr.Response != nil {
			authErr.HTTPStatus = retrieveErr.Response.StatusCode
		}
		if authErr.Description == "" {
			authErr.Description = truncate(strings.TrimSpace(string(retrieveErr.Body)), maxDescriptionLen)
		}
		authErr.AADSTSCode = aadstsPattern.FindString(retrieveErr.ErrorDescription + " " + string(retrieveErr.Body))
		authErr.Kind = classifyAuthKind(authErr.AADSTSCode, authErr.Code, authErr.HTTPStatus)
		return authErr
	}

	// Network-level failures never carry an AADSTS code and always deserve a retry.
	var netErr net.Error
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) {
		authErr.Kind = AuthErrorKindTransient
		authErr.Description = truncate(err.Error(), maxDescriptionLen)
		return authErr
	}

	// Last resort: some transports surface the AADSTS code only as free text.
	authErr.Description = truncate(err.Error(), maxDescriptionLen)
	authErr.AADSTSCode = aadstsPattern.FindString(err.Error())
	authErr.Kind = classifyAuthKind(authErr.AADSTSCode, "", 0)
	return authErr
}

// graphErrorBody is Microsoft Graph's error envelope.
type graphErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// graphTokenErrorCodes maps the Microsoft Graph error codes that explicitly name
// a token or grant problem to the action they require. A 401 carrying one of
// these survived a fresh token, so the grant itself is no longer accepted.
// Keys are lowercased, since Graph is not consistent about casing.
var graphTokenErrorCodes = map[string]AuthErrorKind{
	"invalidauthenticationtoken": AuthErrorKindReauthRequired,
	"expiredauthenticationtoken": AuthErrorKindReauthRequired,
	"compacttoken":               AuthErrorKindReauthRequired,
	"invalidtoken":               AuthErrorKindReauthRequired,
	"tokennotfound":              AuthErrorKindReauthRequired,
	"invalidgrant":               AuthErrorKindReauthRequired,
	"invalid_grant":              AuthErrorKindReauthRequired,
}

// classifyUnauthorized turns a Microsoft Graph 401 that survived a token
// refresh into a classified *AuthError.
//
// Only a body that names the problem is escalated as permanent: a recognized
// AADSTS code, or a Graph error code that says the token or the grant is the
// reason. A 401 that names nothing — an empty body, a proxy's HTML page, an
// error code the SDK does not know — stays AuthErrorKindUnknown and therefore
// retryable, so an ambiguous failure is never mistaken for the expired client
// secret only a human can fix.
func classifyUnauthorized(status int, body []byte) *AuthError {
	text := strings.TrimSpace(string(body))

	authErr := &AuthError{
		Kind:        AuthErrorKindUnknown,
		HTTPStatus:  status,
		AADSTSCode:  aadstsPattern.FindString(text),
		Description: truncate(text, maxDescriptionLen),
	}

	var graphErr graphErrorBody
	if err := json.Unmarshal(body, &graphErr); err == nil && graphErr.Error.Code != "" {
		authErr.Code = graphErr.Error.Code
		if graphErr.Error.Message != "" {
			authErr.Description = truncate(strings.TrimSpace(graphErr.Error.Message), maxDescriptionLen)
		}
	}

	// An expired client secret can surface on the Graph side too, when the token
	// was minted by a broker that revalidates the app credentials.
	if kind, ok := aadstsKinds[authErr.AADSTSCode]; ok {
		authErr.Kind = kind
		return authErr
	}

	if kind, ok := graphTokenErrorCodes[strings.ToLower(authErr.Code)]; ok {
		authErr.Kind = kind
	}

	return authErr
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
