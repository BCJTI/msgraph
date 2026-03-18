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
	Name    string `json:"name,omitempty"`
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
	ID                         string       `json:"id,omitempty"`
	CreatedDateTime            string       `json:"createdDateTime,omitempty"`
	LastModifiedDateTime       string       `json:"lastModifiedDateTime,omitempty"`
	ReceivedDateTime           string       `json:"receivedDateTime,omitempty"`
	SentDateTime               string       `json:"sentDateTime,omitempty"`
	HasAttachments             bool         `json:"hasAttachments,omitempty"`
	InternetMessageID          string       `json:"internetMessageId,omitempty"`
	Subject                    string       `json:"subject"`
	BodyPreview                string       `json:"bodyPreview,omitempty"`
	Importance                 string       `json:"importance,omitempty"`
	ParentFolderID             string       `json:"parentFolderId,omitempty"`
	ConversationID             string       `json:"conversationId,omitempty"`
	IsDeliveryReceiptRequested bool         `json:"isDeliveryReceiptRequested,omitempty"`
	IsReadReceiptRequested     bool         `json:"isReadReceiptRequested,omitempty"`
	IsRead                     bool         `json:"isRead,omitempty"`
	IsDraft                    bool         `json:"isDraft,omitempty"`
	WebLink                    string       `json:"webLink,omitempty"`
	InferenceClassification    string       `json:"inferenceClassification,omitempty"`
	Body                       Body         `json:"body"`
	Sender                     *Recipient   `json:"sender,omitempty"`
	From                       *Recipient   `json:"from,omitempty"`
	ReplyTo                    []Recipient  `json:"replyTo,omitempty"`
	ToRecipients               []Recipient  `json:"toRecipients,omitempty"`
	CcRecipients               []Recipient  `json:"ccRecipients,omitempty"`
	BccRecipients              []Recipient  `json:"bccRecipients,omitempty"`
	Attachments                []Attachment `json:"attachments,omitempty"`
}

type SendMailRequest struct {
	Message         Message `json:"message"`
	SaveToSentItems bool    `json:"saveToSentItems"`
}

type MessageListResponse struct {
	ODataContext  string    `json:"@odata.context,omitempty"`
	ODataNextLink string    `json:"@odata.nextLink,omitempty"`
	Value         []Message `json:"value"`
}

type FileAttachment struct {
	ODataType            string `json:"@odata.type,omitempty"`
	ID                   string `json:"id,omitempty"`
	LastModifiedDateTime string `json:"lastModifiedDateTime,omitempty"`
	Name                 string `json:"name"`
	ContentType          string `json:"contentType"`
	Size                 int    `json:"size,omitempty"`
	IsInline             bool   `json:"isInline,omitempty"`
	ContentID            string `json:"contentId,omitempty"`
	ContentLocation      string `json:"contentLocation,omitempty"`
	ContentBytes         string `json:"contentBytes,omitempty"`
}

type AttachmentListResponse struct {
	ODataContext string           `json:"@odata.context,omitempty"`
	Value        []FileAttachment `json:"value"`
}

type ListMessagesOptions struct {
	Top     int
	Skip    int
	Select  string
	Filter  string
	OrderBy string
	Search  string
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
