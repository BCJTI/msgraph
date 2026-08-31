# msgraph

A small Go SDK for Microsoft Graph: OAuth2 authorization code flow, mail, and calendar.

```go
import "github.com/bcjti/msgraph"
```

## Token lifecycle

The `Client` owns its token. Callers do not have to schedule refreshes or restart
anything when a token ages out:

- Before every Graph call the access token is re-acquired from the refresh token
  if it has expired or falls inside a two-minute refresh window.
- If Graph answers `401` anyway — a token revoked early, or one whose recorded
  expiry was wrong — the token is re-acquired and the call is sent once more.
  Exactly once: a second `401` is reported, not retried.
- Concurrent callers hitting an expired token produce a single token request.
  Entra ID rotates the refresh token on every use, so a stampede would invalidate
  the ones in flight.

Set `OnTokenRefresh` to persist the rotated refresh token, so it survives a
process restart:

```go
client := msgraph.NewClient(cfg)
client.Token = storedToken
client.OnTokenRefresh = func(token *oauth2.Token) {
    store.Save(token) // the refresh token rotates on every refresh
}
```

Read the current token with `CurrentToken()` rather than through the `Token`
field, which the Client writes to while it is in use.

## Authentication errors

Some failures cannot be fixed by retrying. The SDK never hides those behind a
generic error or an endless retry: every authentication failure comes back as an
`*AuthError` classified by the action it requires.

```go
_, err := client.ListMessages(nil)
if err != nil {
    switch {
    case msgraph.IsClientSecretExpired(err):
        // AADSTS7000222 — a human must rotate the secret in the Entra ID app
        // registration. Alert; do not retry.
    case msgraph.IsReauthRequired(err):
        // The user grant is gone. Send the user through the authorization flow.
    case msgraph.IsPermanentAuthError(err):
        // Any other credential problem needing a human.
    default:
        // Transient or unrelated: safe to retry later.
    }
}
```

| Kind | Sentinel | Recovers on its own? |
| --- | --- | --- |
| `client_secret_expired` | `ErrClientSecretExpired` | No — rotate the secret in Entra ID |
| `invalid_client` | `ErrInvalidClient` | No — fix the app registration |
| `reauth_required` | `ErrReauthRequired` | No — the user must authorize again |
| `missing_token` | `ErrMissingToken` | No — supply a token |
| `transient` | `ErrTransientAuth` | Yes — retry later |
| `unknown` | — | Treated as retryable |

`IsPermanentAuthError` is the coarse check: true for everything that will keep
failing until someone intervenes. An unclassified failure is deliberately *not*
permanent, so an unrecognized blip is never escalated as a config error.

`*AuthError` carries the detail behind the classification — `Kind`, `Code`,
`AADSTSCode`, `Description`, `HTTPStatus` — and unwraps to the underlying
`*oauth2.RetrieveError`.

## Tests

`go test .` runs the SDK unit tests; they need no credentials. `go test ./tests`
runs integration tests against a live mailbox and skips unless the OAuth
environment variables documented in `tests/helpers_test.go` are set.
