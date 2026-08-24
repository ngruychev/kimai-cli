package kimai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one Kimai instance.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client

	// loc is the account's own timezone, resolved on first write.
	loc *time.Location
}

// Location returns the timezone configured on the Kimai account, caching it.
//
// Kimai reads the naive timestamps in a write as wall-clock time in the
// account's timezone, which need not match this machine's. Writes are
// converted into it so that "09:00" means 09:00 as the account sees it.
func (c *Client) Location(ctx context.Context) (*time.Location, error) {
	if c.loc != nil {
		return c.loc, nil
	}
	user, err := c.Me(ctx)
	if err != nil {
		return nil, err
	}
	if user.Timezone == "" {
		c.loc = time.Local
		return c.loc, nil
	}
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		return nil, fmt.Errorf("account timezone %q is unknown to this system: %w", user.Timezone, err)
	}
	c.loc = loc
	return c.loc, nil
}

// New returns a Client with a timeout short enough for interactive use.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

// APIError is a non-2xx response from Kimai.
type APIError struct {
	Status int
	Body   string
	Path   string
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	if decoded := decodeMessage(msg); decoded != "" {
		msg = decoded
	}
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("kimai %s: %d %s", e.Path, e.Status, msg)
}

// decodeMessage pulls a human-readable message out of Kimai's error shapes.
func decodeMessage(body string) string {
	var payload struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Errors  struct {
			Errors   []string `json:"errors"`
			Children map[string]struct {
				Errors []string `json:"errors"`
			} `json:"children"`
		} `json:"errors"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return ""
	}
	fields := append([]string(nil), payload.Errors.Errors...)
	for name, child := range payload.Errors.Children {
		if len(child.Errors) > 0 {
			fields = append(fields, fmt.Sprintf("%s: %s", name, strings.Join(child.Errors, ", ")))
		}
	}
	if len(fields) > 0 {
		return strings.Join(fields, "; ")
	}
	if payload.Message != "" {
		return payload.Message
	}
	return payload.Detail
}

// do performs a request against the API and decodes a JSON response into out.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	_, err := c.doHeaders(ctx, method, path, query, body, out)
	return err
}

// doHeaders performs a request and additionally returns the response headers.
func (c *Client) doHeaders(ctx context.Context, method, path string, query url.Values, body, out any) (http.Header, error) {
	endpoint := c.BaseURL + "/api" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Header, &APIError{Status: resp.StatusCode, Body: string(raw), Path: path}
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return resp.Header, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return resp.Header, fmt.Errorf("decoding %s: %w", path, err)
	}
	return resp.Header, nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, q, nil, out)
}
