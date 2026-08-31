package tests

import (
	"fmt"
	"testing"

	"github.com/bcjti/msgraph"
	"github.com/stretchr/testify/assert"
)

func TestListMessages(t *testing.T) {
	sdk := newTestClient(t)

	opts := &msgraph.ListMessagesOptions{
		Top:    10,
		Select: "sender,subject",
	}

	result, err := sdk.ListMessages(opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// print result
	fmt.Println(result)
}

func TestListMessagesNilOpts(t *testing.T) {
	sdk := newTestClient(t)

	result, err := sdk.ListMessages(nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestListMessagesByFolder(t *testing.T) {
	sdk := newTestClient(t)

	opts := &msgraph.ListMessagesOptions{
		Top: 5,
	}

	result, err := sdk.ListMessagesByFolder("inbox", opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMessage(t *testing.T) {
	sdk := newTestClient(t)

	msg, err := sdk.GetMessage("AAMkAGU4NGFiN2VkLWU4YTctNDAxMC1hYmFlLTM4ZGFiZDVmZTM5MgBGAAAAAABK5lii2cv5Sq6Iy9Bqgr3ZBwBfmQBSamUrTaPpdptDpKcmAAAAAAEMAABfmQBSamUrTaPpdptDpKcmAACBRR6IAAA=")

	assert.NoError(t, err)
	assert.NotNil(t, msg)
}

func TestListAttachments(t *testing.T) {
	sdk := newTestClient(t)

	result, err := sdk.ListAttachments("test-message-id")

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetAttachment(t *testing.T) {
	sdk := newTestClient(t)

	attachment, err := sdk.GetAttachment("test-message-id", "test-attachment-id")

	assert.NoError(t, err)
	assert.NotNil(t, attachment)
}

func TestDownloadAttachment(t *testing.T) {
	sdk := newTestClient(t)

	data, err := sdk.DownloadAttachment("test-message-id", "test-attachment-id")

	assert.NoError(t, err)
	assert.NotNil(t, data)
}
