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

type UserInfo struct {
	OdataContext       string   `json:"@odata.context"`
	MicrosoftGraphTips string   `json:"@microsoft.graph.tips"`
	UserPrincipalName  string   `json:"userPrincipalName"`
	ID                 string   `json:"id"`
	DisplayName        string   `json:"displayName"`
	Surname            string   `json:"surname"`
	GivenName          string   `json:"givenName"`
	PreferredLanguage  string   `json:"preferredLanguage"`
	Mail               string   `json:"mail"`
	MobilePhone        *string  `json:"mobilePhone"`    // Can be null
	JobTitle           *string  `json:"jobTitle"`       // Can be null
	OfficeLocation     *string  `json:"officeLocation"` // Can be null
	BusinessPhones     []string `json:"businessPhones"` // Empty array
}
