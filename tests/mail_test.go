package tests

import (
	"testing"

	"github.com/bcjti/msgraph"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
)

func newTestClient() *msgraph.Client {
	sdk := msgraph.NewClient(cfg)
	sdk.Token = &oauth2.Token{
		RefreshToken: "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",
		TokenType:    "Bearer",
	}
	return sdk
}

func TestListMessages(t *testing.T) {
	sdk := newTestClient()

	opts := &msgraph.ListMessagesOptions{
		Top:    10,
		Select: "sender,subject",
	}

	result, err := sdk.ListMessages(opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestListMessagesNilOpts(t *testing.T) {
	sdk := newTestClient()

	result, err := sdk.ListMessages(nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestListMessagesByFolder(t *testing.T) {
	sdk := newTestClient()

	opts := &msgraph.ListMessagesOptions{
		Top: 5,
	}

	result, err := sdk.ListMessagesByFolder("inbox", opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMessage(t *testing.T) {
	sdk := newTestClient()

	msg, err := sdk.GetMessage("test-message-id")

	assert.NoError(t, err)
	assert.NotNil(t, msg)
}

func TestListAttachments(t *testing.T) {
	sdk := newTestClient()

	result, err := sdk.ListAttachments("test-message-id")

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAttachment(t *testing.T) {
	sdk := newTestClient()

	attachment, err := sdk.GetAttachment("test-message-id", "test-attachment-id")

	assert.NoError(t, err)
	assert.NotNil(t, attachment)
}

func TestDownloadAttachment(t *testing.T) {
	sdk := newTestClient()

	data, err := sdk.DownloadAttachment("test-message-id", "test-attachment-id")

	assert.NoError(t, err)
	assert.NotNil(t, data)
}
