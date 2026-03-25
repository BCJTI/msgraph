package msgraph

type DateTimeTimeZone struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type PhysicalAddress struct {
	Street          string `json:"street,omitempty"`
	City            string `json:"city,omitempty"`
	State           string `json:"state,omitempty"`
	CountryOrRegion string `json:"countryOrRegion,omitempty"`
	PostalCode      string `json:"postalCode,omitempty"`
}

type GeoCoordinates struct {
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

type Location struct {
	DisplayName  string           `json:"displayName,omitempty"`
	LocationType string           `json:"locationType,omitempty"`
	UniqueID     string           `json:"uniqueId,omitempty"`
	UniqueIDType string           `json:"uniqueIdType,omitempty"`
	Address      *PhysicalAddress `json:"address,omitempty"`
	Coordinates  *GeoCoordinates  `json:"coordinates,omitempty"`
}

type ResponseStatus struct {
	Response string `json:"response,omitempty"`
	Time     string `json:"time,omitempty"`
}

type Attendee struct {
	Type         string         `json:"type,omitempty"`
	Status       ResponseStatus `json:"status,omitempty"`
	EmailAddress EmailAddress   `json:"emailAddress"`
}

type OnlineMeetingInfo struct {
	JoinURL      string `json:"joinUrl,omitempty"`
	ConferenceID string `json:"conferenceId,omitempty"`
	TollNumber   string `json:"tollNumber,omitempty"`
}

type RecurrencePattern struct {
	Type           string   `json:"type,omitempty"`
	Interval       int      `json:"interval,omitempty"`
	DaysOfWeek     []string `json:"daysOfWeek,omitempty"`
	Month          int      `json:"month,omitempty"`
	DayOfMonth     int      `json:"dayOfMonth,omitempty"`
	FirstDayOfWeek string   `json:"firstDayOfWeek,omitempty"`
	Index          string   `json:"index,omitempty"`
}

type RecurrenceRange struct {
	Type                string `json:"type,omitempty"`
	StartDate           string `json:"startDate,omitempty"`
	EndDate             string `json:"endDate,omitempty"`
	RecurrenceTimeZone  string `json:"recurrenceTimeZone,omitempty"`
	NumberOfOccurrences int    `json:"numberOfOccurrences,omitempty"`
}

type PatternedRecurrence struct {
	Pattern RecurrencePattern `json:"pattern"`
	Range   RecurrenceRange   `json:"range"`
}

type Event struct {
	ID                         string               `json:"id,omitempty"`
	Subject                    string               `json:"subject,omitempty"`
	Body                       *Body                `json:"body,omitempty"`
	BodyPreview                string               `json:"bodyPreview,omitempty"`
	Start                      *DateTimeTimeZone    `json:"start,omitempty"`
	End                        *DateTimeTimeZone    `json:"end,omitempty"`
	Location                   *Location            `json:"location,omitempty"`
	Locations                  []Location           `json:"locations,omitempty"`
	Attendees                  []Attendee           `json:"attendees,omitempty"`
	Organizer                  *Recipient           `json:"organizer,omitempty"`
	Recurrence                 *PatternedRecurrence `json:"recurrence,omitempty"`
	IsAllDay                   bool                 `json:"isAllDay,omitempty"`
	IsCancelled                bool                 `json:"isCancelled,omitempty"`
	IsDraft                    bool                 `json:"isDraft,omitempty"`
	IsOrganizer                bool                 `json:"isOrganizer,omitempty"`
	IsOnlineMeeting            bool                 `json:"isOnlineMeeting,omitempty"`
	OnlineMeetingProvider      string               `json:"onlineMeetingProvider,omitempty"`
	OnlineMeeting              *OnlineMeetingInfo   `json:"onlineMeeting,omitempty"`
	OnlineMeetingURL           string               `json:"onlineMeetingUrl,omitempty"`
	Importance                 string               `json:"importance,omitempty"`
	Sensitivity                string               `json:"sensitivity,omitempty"`
	ShowAs                     string               `json:"showAs,omitempty"`
	Type                       string               `json:"type,omitempty"`
	SeriesMasterID             string               `json:"seriesMasterId,omitempty"`
	ResponseRequested          bool                 `json:"responseRequested,omitempty"`
	ResponseStatus             *ResponseStatus      `json:"responseStatus,omitempty"`
	AllowNewTimeProposals      bool                 `json:"allowNewTimeProposals,omitempty"`
	HideAttendees              bool                 `json:"hideAttendees,omitempty"`
	HasAttachments             bool                 `json:"hasAttachments,omitempty"`
	IsReminderOn               bool                 `json:"isReminderOn,omitempty"`
	ReminderMinutesBeforeStart int                  `json:"reminderMinutesBeforeStart,omitempty"`
	Categories                 []string             `json:"categories,omitempty"`
	WebLink                    string               `json:"webLink,omitempty"`
	CreatedDateTime            string               `json:"createdDateTime,omitempty"`
	LastModifiedDateTime       string               `json:"lastModifiedDateTime,omitempty"`
	OriginalStartTimeZone      string               `json:"originalStartTimeZone,omitempty"`
	OriginalEndTimeZone        string               `json:"originalEndTimeZone,omitempty"`
	ICalUID                    string               `json:"iCalUId,omitempty"`
	ChangeKey                  string               `json:"changeKey,omitempty"`
	TransactionID              string               `json:"transactionId,omitempty"`
}

type EventListResponse struct {
	ODataContext  string  `json:"@odata.context,omitempty"`
	ODataNextLink string  `json:"@odata.nextLink,omitempty"`
	Value         []Event `json:"value"`
}

type ListEventsOptions struct {
	Top     int
	Skip    int
	Select  string
	Filter  string
	OrderBy string
}
