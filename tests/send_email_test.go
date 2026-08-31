package tests

import (
	"os"
	"testing"

	"github.com/bcjti/msgraph"

	"github.com/stretchr/testify/assert"
)

func TestSendEmail(t *testing.T) {
	sdk := newTestClient(t)

	recipient := os.Getenv("OAUTH_TEST_RECIPIENT")
	if recipient == "" {
		t.Skip("missing OAUTH_TEST_RECIPIENT; set it to the address the test email should go to")
	}

	err := sdk.SendEmail("test email",
		"Application has sucessfully sent an email",
		msgraph.ContentTypeText,
		false,
		[]string{recipient},
		[]string{},
		[]string{},
		[]msgraph.Attachment{})

	assert.NoError(t, err)
}

func TestUserInfo(t *testing.T) {
	sdk := newTestClient(t)

	userInfo, err := sdk.GetUserInfo()

	assert.NoError(t, err)
	assert.NotNil(t, userInfo)
}
