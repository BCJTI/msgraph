package msgraph

import (
	"fmt"
)

// ContentType represents the content type of an email message
type ContentType int

const (
	TextContentType ContentType = iota
	HtmlContentType
)

// String returns the string representation of a ContentType
func (ct ContentType) String() string {
	switch ct {
	case TextContentType:
		return "text"
	case HtmlContentType:
		return "html"
	default:
		return fmt.Sprintf("UnknownContentType(%d)", ct)
	}
}

type EmailAddress struct {
	Address string `json:"address"`
}

type Recipient struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

type Body struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type Message struct {
	Subject       string      `json:"subject"`
	Body          Body        `json:"body"`
	ToRecipients  []Recipient `json:"toRecipients"`
	CcRecipients  []Recipient `json:"ccRecipients"`
	BccRecipients []Recipient `json:"bccRecipients"`
}

type SendMailRequest struct {
	Message         Message `json:"message"`
	SaveToSentItems bool    `json:"saveToSentItems"`
}
