// Package api is a ClouDNS HTTP client. Every call is a GET with query
// parameters. The request URL contains the password, so errors include only
// the API path, never the URL or the query string.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultBaseURL is the ClouDNS HTTP API origin.
	DefaultBaseURL = "https://api.cloudns.net"

	// ClouDNS accepts only 10, 20, 30, 50, or 100 for rows-per-page.
	defaultRowsPerPage = 100
	maxPages           = 10000
	maxBody            = 10 << 20
	maxErrorBody       = 400
)

// Credentials are the identity and password for one request.
// Exactly one identity field is sent: sub-auth-user, else sub-auth-id, else auth-id.
type Credentials struct {
	AuthID      string
	SubAuthID   string
	SubAuthUser string
	Password    string
}

// Label describes the identity that will be sent, without the password.
func (c Credentials) Label() (string, error) {
	switch {
	case c.SubAuthUser != "":
		return "sub-auth-user " + c.SubAuthUser, nil
	case c.SubAuthID != "":
		return "sub-auth-id " + c.SubAuthID, nil
	case c.AuthID != "":
		return "auth-id " + c.AuthID, nil
	default:
		return "", errors.New("missing auth id; run `cloudns auth login`")
	}
}

// Client talks to the ClouDNS HTTP API.
type Client struct {
	BaseURL     string
	Creds       Credentials
	HTTP        *http.Client
	RowsPerPage int
}

// New returns a client for the production API.
func New(creds Credentials) *Client {
	return &Client{
		BaseURL:     DefaultBaseURL,
		Creds:       creds,
		RowsPerPage: defaultRowsPerPage,
		HTTP: &http.Client{
			Timeout: 60 * time.Second,
			// Do not follow redirects. A redirect would replay the password to another host.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// APIError is a user-facing API failure. The Error string never includes the request URL.
type APIError struct {
	Path        string
	StatusCode  int
	Body        string
	Description string
	Failed      bool
	Detail      string
}

func (e *APIError) Error() string {
	if e.Failed {
		if e.Description != "" {
			return e.Description
		}
		return "request failed"
	}
	if e.StatusCode != 0 {
		if e.Body == "" {
			return fmt.Sprintf("GET %s: HTTP %d", e.Path, e.StatusCode)
		}
		return fmt.Sprintf("GET %s: HTTP %d: %s", e.Path, e.StatusCode, e.Body)
	}
	if e.Detail != "" {
		return fmt.Sprintf("GET %s: %s", e.Path, e.Detail)
	}
	return "GET " + e.Path
}

// Login checks credentials with GET /login/login.json.
// A Failed status is returned as an error and must not be saved.
func (c *Client) Login(ctx context.Context) error {
	raw, err := c.Do(ctx, "/login/login.json", nil)
	if err != nil {
		return err
	}
	var st statusBody
	if err := json.Unmarshal(raw, &st); err != nil || !strings.EqualFold(st.Status, "Success") {
		return &APIError{Failed: true, Description: "unexpected login response"}
	}
	return nil
}

// Do sends a GET request. params are copied; authentication is added by the client.
func (c *Client) Do(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	q := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	if err := c.Creds.addAuth(q); err != nil {
		return nil, err
	}

	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	// The URL contains the password. Keep it local to this function.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, &APIError{Path: path, Detail: "invalid request"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cloudns-cli")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, &APIError{Path: path, Detail: publicNetErr(err, c.Creds.Password)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, &APIError{Path: path, Detail: publicNetErr(err, c.Creds.Password)}
	}
	if len(body) > maxBody {
		return nil, &APIError{Path: path, Detail: "response too large"}
	}

	if resp.StatusCode != http.StatusOK {
		shown := oneLine(truncate(scrub(string(body), c.Creds.Password), maxErrorBody))
		return nil, &APIError{Path: path, StatusCode: resp.StatusCode, Body: shown}
	}
	if desc, ok := failedStatus(body); ok {
		desc = oneLine(truncate(scrub(desc, c.Creds.Password), maxErrorBody))
		return nil, &APIError{Failed: true, Description: desc}
	}
	return json.RawMessage(body), nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) pageSize() int {
	if c.RowsPerPage <= 0 {
		return defaultRowsPerPage
	}
	return c.RowsPerPage
}

func (c Credentials) addAuth(q url.Values) error {
	switch {
	case c.SubAuthUser != "":
		q.Set("sub-auth-user", c.SubAuthUser)
	case c.SubAuthID != "":
		q.Set("sub-auth-id", c.SubAuthID)
	case c.AuthID != "":
		q.Set("auth-id", c.AuthID)
	default:
		return errors.New("missing auth id; run `cloudns auth login`")
	}
	if c.Password == "" {
		return errors.New("missing auth password; run `cloudns auth login`")
	}
	q.Set("auth-password", c.Password)
	return nil
}

type statusBody struct {
	Status            string `json:"status"`
	StatusDescription string `json:"statusDescription"`
}

func failedStatus(body []byte) (string, bool) {
	var probe statusBody
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", false
	}
	if !strings.EqualFold(strings.TrimSpace(probe.Status), "Failed") {
		return "", false
	}
	msg := strings.TrimSpace(probe.StatusDescription)
	if msg == "" {
		msg = "request failed"
	}
	return msg, true
}

func publicNetErr(err error, secret string) string {
	for {
		var uerr *url.Error
		if !errors.As(err, &uerr) || uerr.Err == nil || uerr.Err == err {
			break
		}
		err = uerr.Err
	}
	msg := oneLine(scrub(err.Error(), secret))
	if msg == "" {
		return "request failed"
	}
	return msg
}

var (
	urlPattern    = regexp.MustCompile(`https?://[^\s"]+`)
	queryPattern  = regexp.MustCompile(`\?[^\s"]*`)
	secretPattern = regexp.MustCompile(`auth-password=[^&\s"]*`)
)

func scrub(msg, secret string) string {
	msg = urlPattern.ReplaceAllString(msg, "")
	msg = queryPattern.ReplaceAllString(msg, "")
	msg = secretPattern.ReplaceAllString(msg, "auth-password=[redacted]")
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
		if esc := url.QueryEscape(secret); esc != secret {
			msg = strings.ReplaceAll(msg, esc, "[redacted]")
		}
	}
	return strings.TrimSpace(msg)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	for !utf8.ValidString(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
}

func trimBody(raw json.RawMessage) json.RawMessage {
	return bytes.TrimSpace(raw)
}
