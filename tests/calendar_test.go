package tests

import (
	"fmt"
	"testing"

	"github.com/bcjti/msgraph"
	"github.com/stretchr/testify/assert"
)

func TestListEvents(t *testing.T) {
	sdk := newTestClient()

	opts := &msgraph.ListEventsOptions{
		Top:    10,
		Select: "subject,start,end,location",
	}

	result, err := sdk.ListEvents(opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	fmt.Println(result)
}

func TestListEventsNilOpts(t *testing.T) {
	sdk := newTestClient()

	result, err := sdk.ListEvents(nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestListEventsByCalendar(t *testing.T) {
	sdk := newTestClient()

	opts := &msgraph.ListEventsOptions{
		Top: 5,
	}

	result, err := sdk.ListEventsByCalendar("AAMkAGU4NGFiN2VkLWU4YTctNDAxMC1hYmFlLTM4ZGFiZDVmZTM5MgBGAAAAAABK5lii2cv5Sq6Iy9Bqgr3ZBwBfmQBSamUrTaPpdptDpKcmAAAAAAEGAABfmQBSamUrTaPpdptDpKcmAAAAAAEGAAA=", opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetEvent(t *testing.T) {
	sdk := newTestClient()

	event, err := sdk.GetEvent("AAMkAGU4NGFiN2VkLWU4YTctNDAxMC1hYmFlLTM4ZGFiZDVmZTM5MgBGAAAAAABK5lii2cv5Sq6Iy9Bqgr3ZBwBfmQBSamUrTaPpdptDpKcmAAAAAAENAABfmQBSamUrTaPpdptDpKcmAACBRR6IAAA=")

	assert.NoError(t, err)
	assert.NotNil(t, event)
}

func TestCreateEvent(t *testing.T) {
	sdk := newTestClient()

	event := &msgraph.Event{
		Subject: "Test Meeting",
		Body: &msgraph.Body{
			ContentType: "HTML",
			Content:     "<p>Agenda for the test meeting.</p>",
		},
		Start: &msgraph.DateTimeTimeZone{
			DateTime: "2026-04-01T10:00:00",
			TimeZone: "America/Sao_Paulo",
		},
		End: &msgraph.DateTimeTimeZone{
			DateTime: "2026-04-01T11:00:00",
			TimeZone: "America/Sao_Paulo",
		},
		Location: &msgraph.Location{
			DisplayName: "Conference Room A",
		},
		Attendees: []msgraph.Attendee{
			{
				Type: "required",
				EmailAddress: msgraph.EmailAddress{
					Address: "test@contoso.com",
					Name:    "Test User",
				},
			},
		},
	}

	result, err := sdk.CreateEvent(event)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	fmt.Println(result)
}

func TestUpdateEvent(t *testing.T) {
	sdk := newTestClient()

	update := &msgraph.Event{
		Subject: "Updated Meeting Subject",
	}

	result, err := sdk.UpdateEvent("AAMkAGU4NGFiN2VkLWU4YTctNDAxMC1hYmFlLTM4ZGFiZDVmZTM5MgBGAAAAAABK5lii2cv5Sq6Iy9Bqgr3ZBwBfmQBSamUrTaPpdptDpKcmAAAAAAENAABfmQBSamUrTaPpdptDpKcmAACBRR6IAAA=", update)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	fmt.Println(result)
}

func TestDeleteEvent(t *testing.T) {
	sdk := newTestClient()

	err := sdk.DeleteEvent("AAMkAGU4NGFiN2VkLWU4YTctNDAxMC1hYmFlLTM4ZGFiZDVmZTM5MgBGAAAAAABK5lii2cv5Sq6Iy9Bqgr3ZBwBfmQBSamUrTaPpdptDpKcmAAAAAAENAABfmQBSamUrTaPpdptDpKcmAACBRR6IAAA=")

	assert.NoError(t, err)
}
