package tests

import (
	"fmt"
	"testing"

	"github.com/bcjti/msgraph"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
)

func newTestClient() *msgraph.Client {
	sdk := msgraph.NewClient(authCfg)
	sdk.Debug = true
	sdk.Token = &oauth2.Token{
		RefreshToken: `1.AW8B4De4TKrhIUer8O1gCkPib68HOc7OG3JOkOHT2VhgqpAAAM5vAQ.BQABAwEAAAADAOz_BQD0_0V2b1N0c0FydGlmYWN0cwIAAAAAAKmKQVHYbNaAyLBoJr3cqy6jbKWGq695acKs07A9h3mT1TGWWfbjwbo0xySnraA4KWDJhqg1A8Eby0Jlg1np9GQUDNY4PFxqQsWvYnWN8vZjz21F0RU_wuvof0-2Q8wjG45QYZ0JrV9_34_vAav0sdZJASLqickEHYnUPo0HzsXHI0KDIgGVPqXZxYPyBGsabbsjTRiWIgOEeX_nJkmriG7GB_RMX6FZYovYeP7VKR2ELbck0p8C02wF0qeiEYeh91J8xnhRKIAyDXiUdH3Y85M6FRq6i9pxC3eKebkVuIZnmuljwAPI5D2JGwA8iA4isQTIfEZeGXm-AD8yQ12yh7ZgA5QucsbOfCFSQFaGNSC3ls3nlhi5GByPt-0HFlH3vi8KMD-7zKdkc1BpoE-WjQd-C2qQ6X8zyQO3XYpKEAACA4-SIo1Ls9XMIlhHI7yR3GYqhAxNwoDwy52ruXd4SdJz-z9J2_KzfjoDSbg6Efo-V0GgXkhnYJeF3DEKTgyHjCU0Vrg_uiH_ckRAPxmNBq-5bkWV3xCLPWlhG0mXdgB9USzqnoPxlg19uSeHsUXFefGxDE555yAHsjynEn515HvOdyssgBzONe15ju7aRJ33vJbDzRHhQX80CKEt057DH5sxZgMNMtTYZ77hJUjR_7so2KOF2yRfx3xPcf3XyTYgU46w2KWIhVgwhe3iAtpNZEsm-SWnxk9EGDHehS0owz1wKUPVKecGRjoDhf3g16vG9wQBSKJd7fHAxxlzYiRBoSigd3XqyfNmvXyQhiiVm4w3aHpYg-gLh1NpfgdIjoZsrxwG3XTjvtsfSXsuULIv5d43PGFHJ-Pd0_Z4Ti1DV5GCMga4huwWV_zfeg202-m1DpV94wVO3GGhtM1lzEuzbjqnvj_1l5uck6IgVNlrFoCe4gQ3-Lxl4GL_tcTZ640X70Lr2tcZlfZAJBmWPLcqXrqKzjPV_W8vfCxYXbkHWm3BGvHIYFgzT49Jk4nfdEy_hrnKBAUHxQmHD3ZD67z1NM5dqfL3VkCFfKsESbNL3Y8ZRKrFblKnYA8pLAOxx0I1B0XXCrNmTHHaSn4NCE9idLisiDZEiqYbakYwsWZiXF0LB5rh75mBnX9d2dV8rXgFI01XFjdxznVKxBSCQaGHZKgF3noRkEJ0exSeq1h3XB9X9mEtmoIP5_O0jyw60JxOz6Av1QiCcUVCLI-eF5frvTJl0iSuAdwQHQ8xhSQlDhQ88kRs-W_xOoksA_czHShjMPjI5AcBFPJSocOBb8i2qy9sfKQRzVU9h99GLX-g2EoE01JL_qbe1DUdGYN9-t3hofvYl_Uc9j9hPprHoecAKROdbvSbRIlk9ST0gDL6vY7nd2uIaKkO9-rJrWz0fgI3H_hhY2mq5Gg7JOSGtdO4t-VV1Fc0UfQ`,
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
	// print result
	fmt.Println(result)
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

	msg, err := sdk.GetMessage("AAMkAGU4NGFiN2VkLWU4YTctNDAxMC1hYmFlLTM4ZGFiZDVmZTM5MgBGAAAAAABK5lii2cv5Sq6Iy9Bqgr3ZBwBfmQBSamUrTaPpdptDpKcmAAAAAAEMAABfmQBSamUrTaPpdptDpKcmAACBRR6IAAA=")

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
