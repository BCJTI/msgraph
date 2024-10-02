package msgraph

import (
	"fmt"
)

// type to get the response from the API SendEmail that just returns a 202 status code or an error
type Http202 struct {
	Content string `json:"content"`
}

// SendEmail sends an email using Microsoft Graph API
func (c *Client) SendEmail(subject, body string, contentType ContentType, saveSentItems bool, toRecipients, ccRecipients, bccRecipients []string) error {
	if c.Token == nil {
		return fmt.Errorf("missing access token. Please obtain one first")
	}

	if !c.Token.Valid() {
		err := c.OAuthRefreshToken()
		if err != nil {
			return err
		}
	}

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
		},
		SaveToSentItems: saveSentItems,
	}

	var response Http202

	err := c.Post("/me/sendMail", emailData, nil, &response)

	return err
}

func SetRecipients(recipients []string) []Recipient {
	result := make([]Recipient, len(recipients))
	for i, recipient := range recipients {
		result[i] = Recipient{
			EmailAddress: EmailAddress{
				Address: recipient,
			},
		}
	}
	return result
}
