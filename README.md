# msgraph

A small Go SDK for Microsoft Graph: OAuth2 authorization code flow, mail, and calendar.

```go
import "github.com/bcjti/msgraph"
```

## Token lifecycle

The `Client` owns its token. Callers do not have to schedule refreshes or restart
anything when a token ages out:

- Before every Graph call the access token is re-acquired from the refresh token
  if it has expired or falls inside a two-minute refresh window. A token carrying
  no recorded expiry is used as-is rather than forced through a refresh; the `401`
  retry below is what recovers it if it turns out to be dead.
- If Graph answers `401` anyway — a token revoked early, or one whose recorded
  expiry was wrong — the token is re-acquired and the call is sent once more.
  Exactly once: a second `401` is reported, not retried.
- Concurrent callers hitting an expired token produce a single token request.
  Entra ID rotates the refresh token on every use, so a stampede would invalidate
  the ones in flight.
- With no `HTTPClient` set the two paths are deliberately bounded differently:
  Graph calls use `http.DefaultClient` and are **not** bounded by a timeout, while
  the OAuth2 token-endpoint calls — the refresh and the authorization code
  exchange — use a client carrying a 30s default timeout. The token request runs
  with the Client's lock held, so a token endpoint that accepts the connection and
  never answers would wedge every goroutine sharing the Client; a hanging Graph
  call blocks only its own caller.
- Set `HTTPClient` and it is used as-is for both paths, its timeout respected and
  never overridden. Because it then also serves the token endpoint, it replaces
  that 30s default guard: give it a timeout, or a token endpoint that never
  answers is unbounded again on the path that holds the lock.

Set `OnTokenRefresh` to persist the rotated refresh token, so it survives a
process restart:

```go
client := msgraph.NewClient(cfg)
client.Token = storedToken
client.OnTokenRefresh = func(token *oauth2.Token) {
    store.Save(token) // the refresh token rotates on every refresh
}
```

`OnTokenRefresh` deliveries are serialized and ordered: the callback is never
called concurrently, and a delivery overtaken by a newer refresh is dropped, so
it cannot persist a refresh token Entra ID has already rotated away. The callback
runs without the Client's lock held, so it may call `CurrentToken` or `SetToken`.

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

That applies to a `401` from Graph too. It is escalated as permanent only when
the response body says why — a known `AADSTS` code (`AADSTS7000222` stays
`client_secret_expired` even when it surfaces through Graph), or a Graph error
code naming the token or the grant, such as `InvalidAuthenticationToken`. A `401`
that explains nothing — an empty body, a proxy's HTML page, an error code the SDK
does not know — is reported as `unknown` and stays retryable. Either way it is
still reported after the single retry, never retried in a loop.

`*AuthError` carries the detail behind the classification — `Kind`, `Code`,
`AADSTSCode`, `Description`, `HTTPStatus` — and unwraps to the underlying
`*oauth2.RetrieveError`.

## Tests

`go test .` runs the SDK unit tests; they need no credentials. `go test ./tests`
runs integration tests against a live mailbox and skips unless the OAuth
environment variables documented in `tests/helpers_test.go` are set.
