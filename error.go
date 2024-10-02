package msgraph

type ErrMessage struct {
	ErrorMessage string      `json:"errorMessage"`
	Message      string      `json:"message"`
	Status       interface{} `json:"status"`
}

func (err ErrMessage) Error() string {
	if err.ErrorMessage != "" {
		return err.ErrorMessage
	}
	return err.Message
}
