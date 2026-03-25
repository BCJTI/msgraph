package msgraph

// GetUserInfo retrieves the signed-in user's profile from Microsoft Graph API.
func (c *Client) GetUserInfo() (*UserInfo, error) {
	userInfo := &UserInfo{}

	err := c.Get("/me", nil, nil, userInfo)

	return userInfo, err
}
