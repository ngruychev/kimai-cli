package kimai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TimesheetQuery filters a timesheet listing.
type TimesheetQuery struct {
	Begin    time.Time
	End      time.Time
	Project  int
	Activity int
	// Users selects whose entries to return: empty means the current user,
	// "all" means every user the token may see.
	Users string
	Size  int
	Page  int
}

// pageSize is the per-request record count, defaulting to Kimai's maximum.
func (q TimesheetQuery) pageSize() int {
	if q.Size > 0 {
		return q.Size
	}
	return 500
}

func (q TimesheetQuery) values() url.Values {
	v := url.Values{}
	// Kimai treats begin/end as local wall-clock and rejects zone offsets here.
	if !q.Begin.IsZero() {
		v.Set("begin", q.Begin.Format(kimaiLayout))
	}
	if !q.End.IsZero() {
		v.Set("end", q.End.Format(kimaiLayout))
	}
	if q.Project > 0 {
		v.Set("project", strconv.Itoa(q.Project))
	}
	if q.Activity > 0 {
		v.Set("activity", strconv.Itoa(q.Activity))
	}
	if q.Users == "all" {
		v.Set("user", "all")
	}
	v.Set("size", strconv.Itoa(q.pageSize()))
	if q.Page > 0 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	return v
}

// Timesheets lists every entry matching the query, newest first, following
// pagination so a long range is never silently truncated. Setting Page on the
// query fetches that single page instead.
func (c *Client) Timesheets(ctx context.Context, q TimesheetQuery) ([]Timesheet, error) {
	if q.Page > 0 {
		var page []Timesheet
		err := c.get(ctx, "/timesheets", q.values(), &page)
		return page, err
	}

	var all []Timesheet
	for page := 1; ; page++ {
		q.Page = page
		var batch []Timesheet
		header, err := c.doHeaders(ctx, http.MethodGet, "/timesheets", q.values(), nil, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)

		total, err := strconv.Atoi(header.Get("X-Total-Pages"))
		if err != nil || page >= total {
			// Without usable pagination metadata, a short page means the end.
			if err != nil && len(batch) == q.pageSize() {
				continue
			}
			return all, nil
		}
	}
}

// Active returns the running entries for the current user.
func (c *Client) Active(ctx context.Context) ([]Timesheet, error) {
	var out []Timesheet
	err := c.get(ctx, "/timesheets/active", nil, &out)
	return out, err
}

// Timesheet fetches one entry by ID.
func (c *Client) Timesheet(ctx context.Context, id int) (*Timesheet, error) {
	var out Timesheet
	if err := c.get(ctx, "/timesheets/"+strconv.Itoa(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TimesheetForm is the writable shape of a timesheet, used for create and edit.
// Pointer fields are omitted from the request when nil, so a PATCH only
// touches the fields the caller set.
//
// Billable is absent deliberately: Kimai calculates it from the project,
// activity and customer configuration and rejects it as an unknown field.
type TimesheetForm struct {
	Begin       *Time   `json:"begin,omitempty"`
	End         *Time   `json:"end,omitempty"`
	Project     *int    `json:"project,omitempty"`
	Activity    *int    `json:"activity,omitempty"`
	Description *string `json:"description,omitempty"`
	Tags        *string `json:"tags,omitempty"`
	Billable    *bool   `json:"billable,omitempty"`
	Exported    *bool   `json:"exported,omitempty"`
}

// inLocation rewrites the form's timestamps into loc, preserving the instant.
// Kimai receives them without an offset and reads them in the account's own
// timezone, so they must be expressed in that zone to mean the right moment.
func (f *TimesheetForm) inLocation(loc *time.Location) {
	if f.Begin != nil {
		f.Begin = &Time{Time: f.Begin.In(loc)}
	}
	if f.End != nil {
		f.End = &Time{Time: f.End.In(loc)}
	}
}

// CreateTimesheet creates an entry. An entry with no End is left running.
func (c *Client) CreateTimesheet(ctx context.Context, form TimesheetForm) (*Timesheet, error) {
	loc, err := c.Location(ctx)
	if err != nil {
		return nil, err
	}
	form.inLocation(loc)

	var out Timesheet
	if err := c.do(ctx, http.MethodPost, "/timesheets", nil, form, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTimesheet patches an existing entry.
func (c *Client) UpdateTimesheet(ctx context.Context, id int, form TimesheetForm) (*Timesheet, error) {
	loc, err := c.Location(ctx)
	if err != nil {
		return nil, err
	}
	form.inLocation(loc)

	var out Timesheet
	path := "/timesheets/" + strconv.Itoa(id)
	if err := c.do(ctx, http.MethodPatch, path, nil, form, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StopTimesheet stops a running entry.
func (c *Client) StopTimesheet(ctx context.Context, id int) (*Timesheet, error) {
	var out Timesheet
	path := "/timesheets/" + strconv.Itoa(id) + "/stop"
	if err := c.do(ctx, http.MethodPatch, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RestartTimesheet starts a new entry copying an existing one, including its
// description, tags and billable flag.
func (c *Client) RestartTimesheet(ctx context.Context, id int) (*Timesheet, error) {
	var out Timesheet
	path := "/timesheets/" + strconv.Itoa(id) + "/restart"
	// Kimai reads copy from the request body. Without it the new entry keeps
	// only the project and activity, dropping description, tags, billable
	// state and meta fields.
	body := map[string]string{"copy": "all"}
	if err := c.do(ctx, http.MethodPatch, path, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTimesheet removes an entry.
func (c *Client) DeleteTimesheet(ctx context.Context, id int) error {
	return c.do(ctx, http.MethodDelete, "/timesheets/"+strconv.Itoa(id), nil, nil, nil)
}

// StopAll stops every running entry and returns those it stopped.
func (c *Client) StopAll(ctx context.Context) ([]Timesheet, error) {
	running, err := c.Active(ctx)
	if err != nil {
		return nil, err
	}
	stopped := make([]Timesheet, 0, len(running))
	for _, entry := range running {
		done, err := c.StopTimesheet(ctx, entry.ID)
		if err != nil {
			return stopped, fmt.Errorf("stopping entry %d: %w", entry.ID, err)
		}
		stopped = append(stopped, *done)
	}
	return stopped, nil
}
