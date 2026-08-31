// Package tests holds integration tests that talk to a live Microsoft Graph
// mailbox. They skip unless the OAuth environment variables below are set:
//
//	OAUTH_CLIENT_ID        app registration (client) ID
//	OAUTH_CLIENT_SECRET    a current client secret
//	OAUTH_REDIRECT_URI     redirect URI, only needed by TestGenerateAccessToken
//	OAUTH_REFRESH_TOKEN    a refresh token for the mailbox under test
//	OAUTH_TEST_RECIPIENT   address TestSendEmail delivers to
//	MSGRAPH_DEBUG          set to any value to log requests and responses
//
// Unit tests for the SDK itself live in the root package and need no credentials.
package tests

import (
	"os"
	"testing"

	"github.com/bcjti/msgraph"
	"golang.org/x/oauth2"
)

// newTestClient returns a Client seeded with the refresh token from the
// environment, skipping the test when the mailbox credentials are not configured.
func newTestClient(t *testing.T) *msgraph.Client {
	t.Helper()

	refreshToken := os.Getenv("OAUTH_REFRESH_TOKEN")

	if authCfg.ClientID == "" || authCfg.ClientSecret == "" || refreshToken == "" {
		t.Skip("missing OAuth environment variables; set OAUTH_CLIENT_ID, OAUTH_CLIENT_SECRET and OAUTH_REFRESH_TOKEN")
	}

	sdk := msgraph.NewClient(authCfg)
	sdk.Debug = os.Getenv("MSGRAPH_DEBUG") != ""
	sdk.Token = &oauth2.Token{
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
	}

	return sdk
}
