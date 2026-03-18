package tests

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/bcjti/msgraph"
	"github.com/stretchr/testify/assert"
)

var authCfg = msgraph.Config{
	ClientID:     "",
	ClientSecret: "",
	RedirectURI:  "http://localhost:4201/callback/microsoft",
	//TenantID:     "dbca83ec-2d34-4865-b5a8-3468cb882dd5",
	Scopes: []string{
		"User.Read",
		"Mail.Send",
		"Mail.Read",
		"offline_access",
	},
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		// Use rundll32 instead of `cmd /c start` because the OAuth URL contains
		// `&` query separators, which `cmd` may interpret and truncate.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// TestGenerateAccessToken is an interactive test that performs the full OAuth2
// authorization code flow. It starts a local HTTP server, opens the browser
// for user consent, captures the callback code, and exchanges it for tokens.
//
// Run with: go test -v -run TestGenerateAccessToken -timeout 120s
func TestGenerateAccessToken(t *testing.T) {
	sdk := msgraph.NewClient(authCfg)

	authURL := sdk.GetAuthorizationURL()
	fmt.Println("=== OAuth2 Authorization Code Flow ===")
	fmt.Println("Opening browser for authorization...")
	fmt.Println("If the browser does not open, visit this URL manually:")
	fmt.Println(authURL)
	fmt.Println()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback/microsoft", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			errDesc := r.URL.Query().Get("error_description")
			http.Error(w, "Authorization failed: "+errMsg, http.StatusBadRequest)
			errCh <- fmt.Errorf("authorization failed: %s - %s", errMsg, errDesc)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this window.</p></body></html>")
		codeCh <- code
	})

	server := &http.Server{
		Addr:    ":4201",
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("failed to start callback server: %w", err)
		}
	}()

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open browser automatically:", err)
	}

	fmt.Println("Waiting for authorization callback (timeout: 90s)...")

	var code string
	select {
	case code = <-codeCh:
		fmt.Println("Authorization code received!")
	case err := <-errCh:
		t.Fatalf("Error during authorization: %v", err)
	case <-time.After(90 * time.Second):
		t.Fatal("Timeout waiting for authorization callback")
	}

	err := sdk.ExchangeCodeForTokens(context.Background(), code)
	assert.NoError(t, err)

	if sdk.Token != nil {
		fmt.Println()
		fmt.Println("=== Token Generated Successfully ===")
		fmt.Printf("Access Token:  %s\n", sdk.Token.AccessToken[0:50])
		fmt.Printf("Token Type:    %s\n", sdk.Token.TokenType)
		fmt.Printf("Expiry:        %s\n", sdk.Token.Expiry.Format(time.RFC3339))
		if sdk.Token.RefreshToken != "" {
			fmt.Printf("Refresh Token: %s...\n", sdk.Token.RefreshToken)
		}
		fmt.Println()

		userInfo, err := sdk.GetUserInfo()
		if err == nil && userInfo != nil {
			fmt.Println("=== Authenticated User ===")
			fmt.Printf("Name:  %s\n", userInfo.DisplayName)
			fmt.Printf("Email: %s\n", userInfo.Mail)
			fmt.Printf("ID:    %s\n", userInfo.ID)
		}
	}
}
