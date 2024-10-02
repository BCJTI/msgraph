package msgraph

import "fmt"

// GetUserInfo sends an email using Microsoft Graph API
func (c *Client) GetUserInfo() error {
	if c.Token == nil {
		return fmt.Errorf("missing access token. Please obtain one first")
	}

	if !c.Token.Valid() {
		err := c.OAuthRefreshToken()
		if err != nil {
			return err
		}
	}

	userInfo := UserInfo{}

	err := c.Get("/me", nil, nil, &userInfo)

	return err
}
