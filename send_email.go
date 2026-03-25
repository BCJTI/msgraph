package msgraph

// Http202 captures the response from API calls that return a 202 status code.
type Http202 struct {
	Content string `json:"content"`
}

// SendEmail sends an email using Microsoft Graph API
func (c *Client) SendEmail(subject, body string, contentType ContentType, saveSentItems bool, toRecipients, ccRecipients, bccRecipients []string, attachs []Attachment) error {
	emailData := SendMailRequest{
		Message: Message{
			Subject: subject,
			Body: Body{
				ContentType: contentType.String(),
				Content:     body,
			},
			ToRecipients:  SetRecipients(toRecipients),
			CcRecipients:  SetRecipients(ccRecipients),
			BccRecipients: SetRecipients(bccRecipients),
			Attachments:   attachs,
		},
		SaveToSentItems: saveSentItems,
	}

	var response Http202

	err := c.Post("/me/sendMail", emailData, nil, &response)

	return err
}

func SetRecipients(recipients []string) []Recipient {
	result := make([]Recipient, 0)
	for _, recipient := range recipients {
		if recipient != "" {
			result = append(result, Recipient{
				EmailAddress: EmailAddress{
					Address: recipient,
				}})
		}
	}
	return result
}
