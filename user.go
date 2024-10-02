package msgraph

import "fmt"

// GetUserInfo sends an email using Microsoft Graph API
func (c *Client) GetUserInfo() (*UserInfo, error) {
	if c.Token == nil {
		return nil, fmt.Errorf("missing access token. Please obtain one first")
	}

	if !c.Token.Valid() {
		err := c.OAuthRefreshToken()
		if err != nil {
			return nil, err
		}
	}

	userInfo := &UserInfo{}

	err := c.Post("/me", nil, nil, userInfo)

	return userInfo, err
}
