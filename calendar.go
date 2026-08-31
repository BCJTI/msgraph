package msgraph

import (
	"fmt"
	"strconv"
	"strings"
)

func (opts *ListEventsOptions) toQueryParams() Params {
	if opts == nil {
		return nil
	}

	params := Params{}

	if opts.Top > 0 {
		params["$top"] = strconv.Itoa(opts.Top)
	}
	if opts.Skip > 0 {
		params["$skip"] = strconv.Itoa(opts.Skip)
	}
	if opts.Select != "" {
		params["$select"] = opts.Select
	}
	if opts.Filter != "" {
		params["$filter"] = opts.Filter
	}
	if opts.OrderBy != "" {
		params["$orderby"] = opts.OrderBy
	}

	if len(params) == 0 {
		return nil
	}

	return params
}

// ListEvents retrieves events from the signed-in user's default calendar.
// Supports OData query parameters via ListEventsOptions.
// GET /me/events
func (c *Client) ListEvents(opts *ListEventsOptions) (*EventListResponse, error) {
	result := &EventListResponse{}

	if err := c.Get("/me/events", opts.toQueryParams(), nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// ListEventsByCalendar retrieves events from a specific calendar.
// GET /me/calendars/{calendarID}/events
func (c *Client) ListEventsByCalendar(calendarID string, opts *ListEventsOptions) (*EventListResponse, error) {
	result := &EventListResponse{}
	path := fmt.Sprintf("/me/calendars/%s/events", calendarID)

	if err := c.Get(path, opts.toQueryParams(), nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// ListEventsByNextLink follows an @odata.nextLink URL for pagination.
// The nextLink is an absolute URL returned by a previous ListEvents call.
func (c *Client) ListEventsByNextLink(nextLink string) (*EventListResponse, error) {
	path := strings.TrimPrefix(nextLink, c.endpoint())
	result := &EventListResponse{}

	if err := c.Get(path, nil, nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetEvent retrieves a single event by its ID.
// GET /me/events/{eventID}
func (c *Client) GetEvent(eventID string) (*Event, error) {
	event := &Event{}
	path := fmt.Sprintf("/me/events/%s", eventID)

	if err := c.Get(path, nil, nil, event); err != nil {
		return nil, err
	}

	return event, nil
}

// CreateEvent creates a new event in the signed-in user's default calendar.
// POST /me/events
func (c *Client) CreateEvent(event *Event) (*Event, error) {
	result := &Event{}

	if err := c.Post("/me/events", event, nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// CreateEventInCalendar creates a new event in a specific calendar.
// POST /me/calendars/{calendarID}/events
func (c *Client) CreateEventInCalendar(calendarID string, event *Event) (*Event, error) {
	result := &Event{}
	path := fmt.Sprintf("/me/calendars/%s/events", calendarID)

	if err := c.Post(path, event, nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateEvent updates an existing event by its ID.
// Only the fields set in the event parameter are updated (PATCH semantics).
// PATCH /me/events/{eventID}
func (c *Client) UpdateEvent(eventID string, event *Event) (*Event, error) {
	result := &Event{}
	path := fmt.Sprintf("/me/events/%s", eventID)

	if err := c.Patch(path, event, nil, result); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteEvent removes an event by its ID.
// If the event is a meeting, deleting it on the organizer's calendar
// sends a cancellation message to attendees.
// DELETE /me/events/{eventID}
func (c *Client) DeleteEvent(eventID string) error {
	path := fmt.Sprintf("/me/events/%s", eventID)

	return c.Delete(path, nil, nil, &Http202{})
}
