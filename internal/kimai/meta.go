package kimai

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Projects lists projects. Passing visible=false includes hidden ones.
func (c *Client) Projects(ctx context.Context, visibleOnly bool) ([]Project, error) {
	var out []Project
	err := c.get(ctx, "/projects", visibility(visibleOnly), &out)
	return out, err
}

// Customers lists customers.
func (c *Client) Customers(ctx context.Context, visibleOnly bool) ([]Customer, error) {
	var out []Customer
	err := c.get(ctx, "/customers", visibility(visibleOnly), &out)
	return out, err
}

// Activities lists activities. A project ID restricts the list to that
// project's activities plus the global ones.
func (c *Client) Activities(ctx context.Context, project int, visibleOnly bool) ([]Activity, error) {
	q := visibility(visibleOnly)
	if project > 0 {
		q.Set("project", strconv.Itoa(project))
		q.Set("globals", "true")
	}
	var out []Activity
	err := c.get(ctx, "/activities", q, &out)
	return out, err
}

// Tags lists the tag names known to the instance.
func (c *Client) Tags(ctx context.Context) ([]string, error) {
	var out []string
	if err := c.get(ctx, "/tags", nil, &out); err == nil {
		return out, nil
	}
	// Older instances return objects rather than bare strings.
	var objects []Tag
	if err := c.get(ctx, "/tags", nil, &objects); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(objects))
	for _, t := range objects {
		names = append(names, t.Name)
	}
	return names, nil
}

// Me returns the account owning the API token.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var out User
	if err := c.get(ctx, "/users/me", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Version returns the Kimai version string reported by the instance.
func (c *Client) Version(ctx context.Context) (string, error) {
	var out struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, "/version", nil, &out); err != nil {
		return "", err
	}
	return out.Version, nil
}

func visibility(visibleOnly bool) url.Values {
	if visibleOnly {
		return url.Values{"visible": []string{"1"}}
	}
	return url.Values{"visible": []string{"3"}}
}

// Lookup resolves names to IDs and IDs back to names for display.
type Lookup struct {
	Projects   []Project
	Customers  []Customer
	Activities []Activity

	projectByID  map[int]Project
	customerByID map[int]Customer
	activityByID map[int]Activity
}

// NewLookup fetches the metadata needed to render and resolve entries.
func (c *Client) NewLookup(ctx context.Context) (*Lookup, error) {
	projects, err := c.Projects(ctx, false)
	if err != nil {
		return nil, err
	}
	customers, err := c.Customers(ctx, false)
	if err != nil {
		return nil, err
	}
	activities, err := c.Activities(ctx, 0, false)
	if err != nil {
		return nil, err
	}

	l := &Lookup{
		Projects:     projects,
		Customers:    customers,
		Activities:   activities,
		projectByID:  make(map[int]Project, len(projects)),
		customerByID: make(map[int]Customer, len(customers)),
		activityByID: make(map[int]Activity, len(activities)),
	}
	for _, p := range projects {
		l.projectByID[p.ID] = p
	}
	for _, cu := range customers {
		l.customerByID[cu.ID] = cu
	}
	for _, a := range activities {
		l.activityByID[a.ID] = a
	}
	return l, nil
}

// ProjectName returns the project's name, or a placeholder if unknown.
func (l *Lookup) ProjectName(id int) string {
	if p, ok := l.projectByID[id]; ok {
		return p.Name
	}
	return unknown(id)
}

// CustomerName returns the name of the customer owning the given project.
func (l *Lookup) CustomerName(projectID int) string {
	p, ok := l.projectByID[projectID]
	if !ok {
		return ""
	}
	if cu, ok := l.customerByID[p.Customer]; ok {
		return cu.Name
	}
	return ""
}

// ActivityName returns the activity's name, or a placeholder if unknown.
func (l *Lookup) ActivityName(id int) string {
	if a, ok := l.activityByID[id]; ok {
		return a.Name
	}
	return unknown(id)
}

func unknown(id int) string {
	if id == 0 {
		return ""
	}
	return "#" + strconv.Itoa(id)
}

// FindProject resolves a project by exact ID, exact name, or unique
// case-insensitive substring.
func (l *Lookup) FindProject(term string) (*Project, error) {
	if id, err := strconv.Atoi(term); err == nil {
		if p, ok := l.projectByID[id]; ok {
			return &p, nil
		}
		return nil, fmt.Errorf("no project with id %d", id)
	}
	var matches []Project
	for _, p := range l.Projects {
		if strings.EqualFold(p.Name, term) {
			return &p, nil
		}
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(term)) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no project matching %q", term)
	default:
		names := make([]string, len(matches))
		for i, p := range matches {
			names[i] = p.Name
		}
		return nil, fmt.Errorf("%q matches several projects: %s", term, strings.Join(names, ", "))
	}
}

// FindActivity resolves an activity across every activity in the instance.
// Prefer MatchActivity with a project-scoped list when a project is known,
// so that activities belonging to other projects cannot be selected.
func (l *Lookup) FindActivity(term string) (*Activity, error) {
	if id, err := strconv.Atoi(term); err == nil {
		if a, ok := l.activityByID[id]; ok {
			return &a, nil
		}
		return nil, fmt.Errorf("no activity with id %d", id)
	}
	return MatchActivity(l.Activities, term)
}

// MatchActivity resolves an activity within the given candidates by exact ID,
// exact name, or unique case-insensitive substring.
func MatchActivity(candidates []Activity, term string) (*Activity, error) {
	if id, err := strconv.Atoi(term); err == nil {
		for _, a := range candidates {
			if a.ID == id {
				return &a, nil
			}
		}
		return nil, fmt.Errorf("no activity with id %d for this project", id)
	}
	var matches []Activity
	for _, a := range candidates {
		if strings.EqualFold(a.Name, term) {
			return &a, nil
		}
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(term)) {
			matches = append(matches, a)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no activity matching %q", term)
	default:
		names := make([]string, len(matches))
		for i, a := range matches {
			names[i] = a.Name
		}
		return nil, fmt.Errorf("%q matches several activities: %s", term, strings.Join(names, ", "))
	}
}

// FindCustomer resolves a customer the same way FindProject resolves projects.
func (l *Lookup) FindCustomer(term string) (*Customer, error) {
	if id, err := strconv.Atoi(term); err == nil {
		if cu, ok := l.customerByID[id]; ok {
			return &cu, nil
		}
		return nil, fmt.Errorf("no customer with id %d", id)
	}
	for _, cu := range l.Customers {
		if strings.EqualFold(cu.Name, term) {
			return &cu, nil
		}
	}
	for _, cu := range l.Customers {
		if strings.Contains(strings.ToLower(cu.Name), strings.ToLower(term)) {
			return &cu, nil
		}
	}
	return nil, fmt.Errorf("no customer matching %q", term)
}
