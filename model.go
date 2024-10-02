package msgraph

// ContentType represents the content type of an email message
type ContentType string

const (
	ContentTypeText ContentType = "text"
	ContentTypeHTML ContentType = "html"
)

func (enum ContentType) String() string {
	return string(enum)
}

type AttachContentType string

const (
	AttachContentTypePDF  AttachContentType = "application/pdf"
	AttachContentTypePNG  AttachContentType = "image/png"
	AttachContentTypeJPEG AttachContentType = "image/jpeg"
	AttachContentTypeTXT  AttachContentType = "text/plain"
	AttachContentTypeDOCX AttachContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	AttachContentTypeXLSX AttachContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	AttachContentTypeZIP  AttachContentType = "application/zip"
	AttachContentTypeHTML AttachContentType = "text/html"
	AttachContentTypeJSON AttachContentType = "application/json"
	AttachContentTypeXML  AttachContentType = "application/xml"
)

func (enum AttachContentType) String() string {
	return string(enum)
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

type Attachment struct {
	ODataType    string `json:"@odata.type"`
	Name         string `json:"name"`
	ContentType  string `json:"contentType"`
	ContentBytes string `json:"contentBytes"`
}

type Message struct {
	Subject       string       `json:"subject"`
	Body          Body         `json:"body"`
	ToRecipients  []Recipient  `json:"toRecipients"`
	CcRecipients  []Recipient  `json:"ccRecipients"`
	BccRecipients []Recipient  `json:"bccRecipients"`
	Attachments   []Attachment `json:"attachments,omitempty"`
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
	MobilePhone        *string  `json:"mobilePhone"`
	JobTitle           *string  `json:"jobTitle"`
	OfficeLocation     *string  `json:"officeLocation"`
	BusinessPhones     []string `json:"businessPhones"`
}
