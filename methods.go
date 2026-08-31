package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const baseUrl = "https://graph.microsoft.com/v1.0"

// maxAuthAttempts bounds how many times a single call may be sent. A 401 buys
// exactly one retry with a freshly acquired token; a second 401 means the grant
// itself is no longer accepted and is reported as such instead of looping.
const maxAuthAttempts = 2

// Params serves to map data for post in the request
type Params map[string]interface{}

// Headers serves to add extra request headers
type Headers map[string]string

// request describes a single Microsoft Graph call, in a form that can be rebuilt
// from scratch when the call has to be retried with a new access token.
type request struct {
	method  string
	path    string
	params  interface{}
	headers Headers
	// json adds the JSON Accept and Content-type headers. Endpoints that return
	// binary content (attachment downloads) leave it off so Graph does not answer
	// with a JSON envelope instead of the bytes.
	json bool
}

// endpoint returns the Microsoft Graph base URL this Client talks to.
func (c *Client) endpoint() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return baseUrl
}

// httpClient returns the HTTP client to use for Graph calls.
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// build turns the request description into an *http.Request carrying token.
// Params are sent as a JSON body for every method except GET, where they become
// query string values.
func (c *Client) build(ctx context.Context, req request, token string) (*http.Request, error) {

	var (
		httpReq *http.Request
		err     error
	)

	endpoint := c.endpoint() + req.path

	// check for params
	if req.params != nil {

		// marshal params
		b, marshalErr := json.Marshal(req.params)
		if marshalErr != nil {
			return nil, marshalErr
		}

		// send as body
		if req.method != http.MethodGet {

			// set payload with params
			payload := strings.NewReader(string(b))

			// set request with payload
			if httpReq, err = http.NewRequestWithContext(ctx, req.method, endpoint, payload); err != nil {
				return nil, err
			}

		} else {

			var values Params

			// convert any type to params
			if err = json.Unmarshal(b, &values); err != nil {
				return nil, err
			}

			// init request
			if httpReq, err = http.NewRequestWithContext(ctx, req.method, endpoint, nil); err != nil {
				return nil, err
			}

			// init query string
			query := httpReq.URL.Query()

			// add params
			for key, value := range values {
				query.Add(key, AnyToString(value))
			}

			// set query string
			httpReq.URL.RawQuery = query.Encode()

		}

	} else {

		// set request without payload
		if httpReq, err = http.NewRequestWithContext(ctx, req.method, endpoint, nil); err != nil {
			return nil, err
		}

	}

	httpReq.Header.Add("Authorization", "Bearer "+token)

	if req.json {
		httpReq.Header.Add("Accept", "application/json")
		httpReq.Header.Add("Content-type", "application/json")
	}

	// add extra headers
	for key, value := range req.headers {
		httpReq.Header.Add(key, value)
	}

	return httpReq, nil
}

// do acquires an access token, sends the request, and returns the response
// status and body.
//
// A 401 is taken to mean the access token went stale — revoked, or dead earlier
// than its stated expiry — so the token is re-acquired and the call is sent once
// more. If that refresh fails, its classified *AuthError is returned unchanged,
// which is how an expired client secret reaches the caller as something it can
// tell apart from a transient fault. A 401 that survives the retry comes back as
// an *AuthError too, rather than as an opaque response body.
func (c *Client) do(ctx context.Context, req request) (int, []byte, error) {

	staleToken := ""

	for attempt := 1; ; attempt++ {

		token, err := c.ensureToken(ctx, tokenRequest{staleAccessToken: staleToken})
		if err != nil {
			return 0, nil, err
		}

		httpReq, err := c.build(ctx, req, token)
		if err != nil {
			return 0, nil, err
		}

		if c.Debug {
			fmt.Printf("[DEBUG] %s %s\n", httpReq.Method, httpReq.URL.String())
			for key, value := range httpReq.Header {
				if key == "Authorization" {
					fmt.Printf("[DEBUG] Header %s: %s\n", key, redact(value[0]))
				} else {
					fmt.Printf("[DEBUG] Header %s: %s\n", key, value)
				}
			}
		}

		status, data, err := c.send(httpReq)
		if err != nil {
			return 0, nil, err
		}

		if status != http.StatusUnauthorized {
			return status, data, nil
		}

		if attempt >= maxAuthAttempts {
			return status, data, classifyUnauthorized(status, data)
		}

		if c.Debug {
			fmt.Printf("[DEBUG] Got 401, re-acquiring the access token and retrying\n")
		}

		staleToken = token

	}

}

// send performs the request and reads the full response body.
func (c *Client) send(httpReq *http.Request) (int, []byte, error) {

	response, err := c.httpClient().Do(httpReq)
	if err != nil {
		return 0, nil, err
	}

	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, nil, err
	}

	if c.Debug {
		fmt.Printf("[DEBUG] Response Status: %d\n", response.StatusCode)
		fmt.Printf("[DEBUG] Response Body: %s\n", truncate(string(data), maxDescriptionLen))
	}

	return response.StatusCode, data, nil
}

// statusError renders a failed response as an error, preferring the API's own
// error message over the raw body.
func statusError(status int, data []byte) error {

	if len(data) > 0 {
		msg := &ErrMessage{}
		if err := json.Unmarshal(data, msg); err == nil && msg.ErrorMessage != "" {
			return msg
		}

		return errors.New(string(data))
	}

	return fmt.Errorf("%d %s", status, http.StatusText(status))
}

// Make request and return the response
func (c *Client) execute(method string, path string, params interface{}, headers Headers, model interface{}) error {

	status, data, err := c.do(context.Background(), request{
		method:  method,
		path:    path,
		params:  params,
		headers: headers,
		json:    true,
	})
	if err != nil {
		return err
	}

	// verify status code before parsing: an error body is not the model, and may
	// not even be JSON.
	if NotIn(status, http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent) {
		return statusError(status, data)
	}

	if len(data) > 0 {
		// check for error message
		msg := &ErrMessage{}
		if err = json.Unmarshal(data, msg); err == nil && msg.ErrorMessage != "" {
			return msg
		}

		if err != nil {
			return err
		}

		if err = json.Unmarshal(data, model); err != nil {
			return err
		}
	}

	// parse data
	return nil

}

// executeRaw makes a request and returns the raw response bytes without JSON parsing.
// Used for endpoints that return binary content (e.g. attachment downloads).
func (c *Client) executeRaw(method string, path string, headers Headers) ([]byte, error) {

	status, data, err := c.do(context.Background(), request{
		method:  method,
		path:    path,
		headers: headers,
	})
	if err != nil {
		return nil, err
	}

	if NotIn(status, http.StatusOK, http.StatusCreated, http.StatusAccepted) {
		return nil, statusError(status, data)
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
