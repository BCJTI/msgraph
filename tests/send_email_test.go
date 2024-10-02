package tests

import (
	"github.com/bcjti/msgraph"
	"golang.org/x/oauth2"
	"testing"

	"github.com/stretchr/testify/assert"
)

var MicrosoftScopes = []string{
	"https://graph.microsoft.com/.default",
	"https://graph.microsoft.com/User.Read",
	"https://graph.microsoft.com/Mail.Send",
}

var cfg = msgraph.Config{
	ClientID:     "",
	ClientSecret: "",
	RedirectURI:  "",
	Scopes:       MicrosoftScopes,
}

func TestSendEmail(t *testing.T) {
	sdk := msgraph.NewClient(cfg)

	sdk.Token = &oauth2.Token{
		RefreshToken: "",
		TokenType:    "Bearer",
	}

	err := sdk.SendEmail("test email",
		"Application has sucessfully sent an email",
		msgraph.ContentTypeText,
		false,
		[]string{"jacocasa@gmail.com"},
		[]string{},
		[]string{},
		[]msgraph.Attachment{})

	assert.NoError(t, err)
}

func TestUserInfo(t *testing.T) {
	sdk := msgraph.NewClient(cfg)

	sdk.Token = &oauth2.Token{
		RefreshToken: "",
		TokenType:    "Bearer",
	}

	userInfo, err := sdk.GetUserInfo()

	assert.NoError(t, err)
	assert.NotNil(t, userInfo)

}
