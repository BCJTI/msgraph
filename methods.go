package msgraph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const baseUrl = "https://graph.microsoft.com/v1.0"

// Params serves to map data for post in the request
type Params map[string]interface{}

// Headers serves to add extra request headers
type Headers map[string]string

// Make request and return the response
func (c *Client) execute(method string, path string, params interface{}, headers Headers, model interface{}) error {

	var request *http.Request

	// init vars
	endpoint := baseUrl + path

	// check for params
	if params != nil {

		// marshal params
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}

		// send as body
		if method != http.MethodGet {

			// set payload with params
			payload := strings.NewReader(string(b))

			// set request with payload
			request, _ = http.NewRequest(method, endpoint, payload)

		} else {

			var values Params

			// convert any type to params
			if err = json.Unmarshal(b, &values); err != nil {
				return err
			}

			// init request
			request, _ = http.NewRequest(method, endpoint, nil)

			// init query string
			query := request.URL.Query()

			// add params
			for key, value := range values {
				query.Add(key, AnyToString(value))
			}

			// set query string
			request.URL.RawQuery = query.Encode()

		}

	} else {

		// set request without payload
		request, _ = http.NewRequest(method, endpoint, nil)

	}

	// set basic auth

	request.Header.Add("Authorization", "Bearer "+c.Token.AccessToken)
	// define json content type
	request.Header.Add("Accept", "application/json")
	request.Header.Add("Content-type", "application/json")

	// add extra headers
	if headers != nil {
		for key, value := range headers {
			request.Header.Add(key, value)
		}
	}

	if c.Debug {
		fmt.Printf("[DEBUG] %s %s\n", request.Method, request.URL.String())
		for k, v := range request.Header {
			if k == "Authorization" {
				fmt.Printf("[DEBUG] Header %s: %s...]\n", k, v[0][:30])
			} else {
				fmt.Printf("[DEBUG] Header %s: %s\n", k, v)
			}
		}
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	// read response
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if c.Debug {
		fmt.Printf("[DEBUG] Response Status: %d\n", response.StatusCode)
		body := string(data)
		if len(body) > 500 {
			body = body[:500] + "..."
		}
		fmt.Printf("[DEBUG] Response Body: %s\n", body)
	}

	// init error response
	msg := &ErrMessage{}

	if len(data) > 0 {
		// check for error message
		if err = json.Unmarshal(data, msg); err == nil && msg.ErrorMessage != "" {
			return msg
		}

		if err != nil {
			return err
		}

		err = json.Unmarshal(data, model)

		if err != nil {
			return err
		}
	}

	// verify status code
	if NotIn(response.StatusCode, http.StatusOK, http.StatusCreated, http.StatusAccepted) {

		// return body as error message
		if len(data) > 0 {
			return errors.New(string(data))
		}

		// return http status as error
		return errors.New(response.Status)

	}

	// parse data
	return nil

}

// executeRaw makes a request and returns the raw response bytes without JSON parsing.
// Used for endpoints that return binary content (e.g. attachment downloads).
func (c *Client) executeRaw(method string, path string, headers Headers) ([]byte, error) {

	endpoint := baseUrl + path

	request, _ := http.NewRequest(method, endpoint, nil)

	request.Header.Add("Authorization", "Bearer "+c.Token.AccessToken)

	if headers != nil {
		for key, value := range headers {
			request.Header.Add(key, value)
		}
	}

	if c.Debug {
		fmt.Printf("[DEBUG] %s %s\n", request.Method, request.URL.String())
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if c.Debug {
		fmt.Printf("[DEBUG] Response Status: %d\n", response.StatusCode)
		fmt.Printf("[DEBUG] Response Size: %d bytes\n", len(data))
	}

	if NotIn(response.StatusCode, http.StatusOK, http.StatusCreated, http.StatusAccepted) {
		if len(data) > 0 {
			return nil, errors.New(string(data))
		}
		return nil, errors.New(response.Status)
	}

	return data, nil

}

// GetRaw executes GET requests and returns raw bytes
func (c *Client) GetRaw(path string, headers Headers) ([]byte, error) {
	return c.executeRaw(http.MethodGet, path, headers)
}

// Get executes GET requests
func (c *Client) Get(path string, params interface{}, headers Headers, model interface{}) error {
	return c.execute(http.MethodGet, path, params, headers, model)
}

// Post executes POST requests
func (c *Client) Post(path string, params interface{}, headers Headers, model interface{}) error {
	return c.execute(http.MethodPost, path, params, headers, model)
}

// Put executes PUT requests
func (c *Client) Put(path string, params interface{}, headers Headers, model interface{}) error {
	return c.execute(http.MethodPut, path, params, headers, model)
}

// Patch executes PATCH requests
func (c *Client) Patch(path string, params interface{}, headers Headers, model interface{}) error {
	return c.execute(http.MethodPatch, path, params, headers, model)
}

// Delete executes DELETE requests
func (c *Client) Delete(path string, params interface{}, headers Headers, model interface{}) error {
	return c.execute(http.MethodDelete, path, params, headers, model)
}
