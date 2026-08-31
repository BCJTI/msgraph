package msgraph

import (
	"fmt"
	"strconv"
	"strings"
)

func (opts *ListMessagesOptions) toQueryParams() Params {
	if opts == nil {
		return nil
	}

	params := Params{}

	if opts.Top > 0 {
		params["$top"] = strconv.Itoa(opts.Top)
	}
	if opts.Skip > 0 {
		params["$skip"] = strconv.Itoa(opts.Skip)
	}
	if opts.Select != "" {
		params["$select"] = opts.Select
	}
	if opts.Filter != "" {
		params["$filter"] = opts.Filter
	}
	if opts.OrderBy != "" {
		params["$orderby"] = opts.OrderBy
	}
	if opts.Search != "" {
		params["$search"] = opts.Search
	}

	if len(params) == 0 {
		return nil
	}

	return params
}

// ListMessages retrieves messages from the signed-in user's mailbox.
// Supports OData query parameters via ListMessagesOptions.
// GET /me/messages
func (c *Client) ListMessages(opts *ListMessagesOptions) (*MessageListResponse, error) {
	result := &MessageListResponse{}

	if err := c.Get("/me/messages", opts.toQueryParams(), nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// ListMessagesByFolder retrieves messages from a specific mail folder.
// GET /me/mailFolders/{folderID}/messages
func (c *Client) ListMessagesByFolder(folderID string, opts *ListMessagesOptions) (*MessageListResponse, error) {
	result := &MessageListResponse{}
	path := fmt.Sprintf("/me/mailFolders/%s/messages", folderID)

	if err := c.Get(path, opts.toQueryParams(), nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// ListMessagesByNextLink follows an @odata.nextLink URL for pagination.
// The nextLink is an absolute URL returned by a previous ListMessages call.
func (c *Client) ListMessagesByNextLink(nextLink string) (*MessageListResponse, error) {
	path := strings.TrimPrefix(nextLink, c.endpoint())
	result := &MessageListResponse{}

	if err := c.Get(path, nil, nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetMessage retrieves a single message by its ID.
// GET /me/messages/{messageID}
func (c *Client) GetMessage(messageID string) (*Message, error) {
	msg := &Message{}
	path := fmt.Sprintf("/me/messages/%s", messageID)

	if err := c.Get(path, nil, nil, msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// ListAttachments retrieves all attachments for a given message.
// GET /me/messages/{messageID}/attachments
func (c *Client) ListAttachments(messageID string) (*AttachmentListResponse, error) {
	result := &AttachmentListResponse{}
	path := fmt.Sprintf("/me/messages/%s/attachments", messageID)

	if err := c.Get(path, nil, nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetAttachment retrieves a single attachment's metadata and content (base64-encoded).
// GET /me/messages/{messageID}/attachments/{attachmentID}
func (c *Client) GetAttachment(messageID, attachmentID string) (*FileAttachment, error) {
	attachment := &FileAttachment{}
	path := fmt.Sprintf("/me/messages/%s/attachments/%s", messageID, attachmentID)

	if err := c.Get(path, nil, nil, attachment); err != nil {
		return nil, err
	}

	return attachment, nil
}

// DownloadAttachment retrieves the raw binary content of an attachment.
// GET /me/messages/{messageID}/attachments/{attachmentID}/$value
func (c *Client) DownloadAttachment(messageID, attachmentID string) ([]byte, error) {
	path := fmt.Sprintf("/me/messages/%s/attachments/%s/$value", messageID, attachmentID)

	data, err := c.GetRaw(path, nil)
	if err != nil {
		return nil, err
	}

	return data, nil
}
