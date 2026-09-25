package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const testPassword = "s3cret-value"

func TestClient(t *testing.T) {
	baseCreds := Credentials{AuthID: "100", Password: testPassword}

	tests := []struct {
		name      string
		creds     Credentials
		pageSize  int
		setup     func(t *testing.T) *Client
		handle    func(t *testing.T, w http.ResponseWriter, r *http.Request)
		call      func(t *testing.T, c *Client) error
		wantErr   string
		exactErr  bool
		checkHits bool
		wantHits  []string
	}{
		{
			name:      "login success",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/login/login.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/login/login.json" {
					t.Errorf("path = %s", r.URL.Path)
				}
				io.WriteString(w, `{"status":"Success","statusDescription":"Success login."}`)
			},
			call: func(t *testing.T, c *Client) error {
				return c.Login(context.Background())
			},
		},
		{
			name:      "login failure",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/login/login.json"},
			wantErr:   "Invalid authentication, incorrect auth-id or auth-password.",
			exactErr:  true,
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"status":"Failed","statusDescription":"Invalid authentication, incorrect auth-id or auth-password.","echo":"`+testPassword+`"}`)
			},
			call: func(t *testing.T, c *Client) error {
				err := c.Login(context.Background())
				var apiErr *APIError
				if !errors.As(err, &apiErr) || !apiErr.Failed {
					t.Fatalf("error = %v, want failed status", err)
				}
				return err
			},
		},
		{
			name: "login with sub-auth-user omits auth-id",
			creds: Credentials{
				AuthID:      "100",
				SubAuthID:   "5",
				SubAuthUser: "alice",
				Password:    testPassword,
			},
			checkHits: true,
			wantHits:  []string{"/login/login.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"status":"Success","statusDescription":"Success login."}`)
			},
			call: func(t *testing.T, c *Client) error {
				return c.Login(context.Background())
			},
		},
		{
			name:      "list zones follows pages",
			creds:     baseCreds,
			pageSize:  2,
			checkHits: true,
			wantHits:  []string{"/dns/list-zones.json", "/dns/list-zones.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/dns/list-zones.json" {
					t.Errorf("path = %s", r.URL.Path)
				}
				q := r.URL.Query()
				if q.Get("rows-per-page") != "2" {
					t.Errorf("rows-per-page = %q", q.Get("rows-per-page"))
				}
				switch q.Get("page") {
				case "1":
					io.WriteString(w, `[{"name":"a.example","type":"master","status":"1"},{"name":"b.example","type":"master","status":1}]`)
				case "2":
					io.WriteString(w, `[{"name":"c.example","type":"slave","status":"1"}]`)
				default:
					t.Errorf("unexpected page %q", q.Get("page"))
				}
			},
			call: func(t *testing.T, c *Client) error {
				zones, raw, err := c.ListZones(context.Background())
				if err != nil {
					return err
				}
				got := make([]string, len(zones))
				for i, z := range zones {
					got[i] = z.Name
				}
				want := []string{"a.example", "b.example", "c.example"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("zones = %#v, want %#v", got, want)
				}
				if zones[1].Status != "1" {
					t.Fatalf("numeric status = %q", zones[1].Status)
				}
				if !strings.Contains(string(raw), "c.example") {
					t.Fatalf("combined payload = %s", raw)
				}
				return nil
			},
		},
		{
			name:      "list zones uses rows-per-page 100 by default",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/dns/list-zones.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("page") != "1" || q.Get("rows-per-page") != "100" {
					t.Errorf("page=%q rows-per-page=%q", q.Get("page"), q.Get("rows-per-page"))
				}
				io.WriteString(w, `[{"name":"only.example","type":"master","status":"1"}]`)
			},
			call: func(t *testing.T, c *Client) error {
				zones, _, err := c.ListZones(context.Background())
				if err != nil {
					return err
				}
				if len(zones) != 1 || zones[0].Name != "only.example" {
					t.Fatalf("zones = %#v", zones)
				}
				return nil
			},
		},
		{
			name:      "zone object payload is not paginated",
			creds:     baseCreds,
			pageSize:  1,
			checkHits: true,
			wantHits:  []string{"/dns/list-zones.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") != "1" {
					t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
				}
				io.WriteString(w, `{"b.example":{"name":"b.example","type":"master","status":"1"},"a.example":{"name":"a.example","type":"master","status":"1"}}`)
			},
			call: func(t *testing.T, c *Client) error {
				zones, _, err := c.ListZones(context.Background())
				if err != nil {
					return err
				}
				got := []string{zones[0].Name, zones[1].Name}
				want := []string{"a.example", "b.example"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("zones = %#v", got)
				}
				return nil
			},
		},
		{
			name:      "list domains follows pages",
			creds:     baseCreds,
			pageSize:  2,
			checkHits: true,
			wantHits:  []string{"/domains/list-domains.json", "/domains/list-domains.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/domains/list-domains.json" {
					t.Errorf("path = %s", r.URL.Path)
				}
				q := r.URL.Query()
				if q.Get("rows-per-page") != "2" {
					t.Errorf("rows-per-page = %q", q.Get("rows-per-page"))
				}
				switch q.Get("page") {
				case "1":
					io.WriteString(w, `[{"name":"a.example","status":"1","expires_on":"2027-01-01","registered_on":"2025-01-01"},{"name":"b.example","status":"0","expires_on":"2020-01-01"}]`)
				case "2":
					io.WriteString(w, `[{"name":"c.example","status":"1","expire":"2028-05-01"}]`)
				default:
					t.Errorf("unexpected page %q", q.Get("page"))
				}
			},
			call: func(t *testing.T, c *Client) error {
				domains, _, err := c.ListDomains(context.Background())
				if err != nil {
					return err
				}
				got := make([]string, len(domains))
				for i, domain := range domains {
					got[i] = domain.Name
				}
				want := []string{"a.example", "b.example", "c.example"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("domains = %#v", got)
				}
				if domains[0].Expires != "2027-01-01" || domains[1].Status != "0" {
					t.Fatalf("domain fields = %#v", domains)
				}
				return nil
			},
		},
		{
			name:      "record add sends record-type and apex host",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/dns/add-record.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("record-type") != "A" {
					t.Errorf("record-type = %q", q.Get("record-type"))
				}
				if _, ok := q["type"]; ok {
					t.Error("sent type; want record-type")
				}
				if q.Get("host") != "@" {
					t.Errorf("host = %q", q.Get("host"))
				}
				if q.Get("record") != "192.0.2.10" || q.Get("ttl") != "3600" || q.Get("domain-name") != "example.com" {
					t.Errorf("domain=%q record=%q ttl=%q", q.Get("domain-name"), q.Get("record"), q.Get("ttl"))
				}
				io.WriteString(w, `{"status":"Success","statusDescription":"The record was added successfully."}`)
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.AddRecord(context.Background(), RecordSpec{Domain: "example.com", Type: "A", Host: "@", Record: "192.0.2.10", TTL: 3600})
				return err
			},
		},
		{
			name:      "upsert updates the existing record",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/dns/records.json", "/dns/mod-record.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				switch r.URL.Path {
				case "/dns/records.json":
					if q.Get("domain-name") != "example.com" || q.Get("type") != "A" || q.Get("host") != "www" {
						t.Errorf("list domain=%q type=%q host=%q", q.Get("domain-name"), q.Get("type"), q.Get("host"))
					}
					io.WriteString(w, `{"42":{"id":"42","type":"A","host":"www","record":"192.0.2.1","ttl":"3600"}}`)
				case "/dns/mod-record.json":
					if q.Get("record-id") != "42" || q.Get("domain-name") != "example.com" {
						t.Errorf("record-id=%q domain=%q", q.Get("record-id"), q.Get("domain-name"))
					}
					if q.Get("host") != "www" || q.Get("record") != "192.0.2.9" || q.Get("ttl") != "3600" {
						t.Errorf("host=%q record=%q ttl=%q", q.Get("host"), q.Get("record"), q.Get("ttl"))
					}
					if _, ok := q["record-type"]; ok {
						t.Error("mod-record sent record-type")
					}
					if _, ok := q["type"]; ok {
						t.Error("mod-record sent type")
					}
					io.WriteString(w, `{"status":"Success","statusDescription":"The record was modified successfully."}`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.UpsertRecord(context.Background(), RecordSpec{Domain: "example.com", Type: "A", Host: "www", Record: "192.0.2.9", TTL: 3600})
				return err
			},
		},
		{
			name:      "upsert adds when no record exists",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/dns/records.json", "/dns/add-record.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/dns/records.json":
					io.WriteString(w, `{}`)
				case "/dns/add-record.json":
					q := r.URL.Query()
					if q.Get("record-type") != "CNAME" || q.Get("host") != "@" || q.Get("record") != "my-site.pages.dev" {
						t.Errorf("record-type=%q host=%q record=%q", q.Get("record-type"), q.Get("host"), q.Get("record"))
					}
					io.WriteString(w, `{"status":"Success","statusDescription":"The record was added successfully."}`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.UpsertRecord(context.Background(), RecordSpec{Domain: "example.com", Type: "CNAME", Host: "@", Record: "my-site.pages.dev", TTL: 3600})
				return err
			},
		},
		{
			name:      "upsert refuses multiple matches",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/dns/records.json"},
			wantErr:   "found 2 A records",
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/dns/records.json" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				io.WriteString(w, `{"1":{"id":"1","type":"A","host":"www","record":"192.0.2.1","ttl":"60"},"2":{"id":"2","type":"A","host":"www","record":"192.0.2.2","ttl":"60"}}`)
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.UpsertRecord(context.Background(), RecordSpec{Domain: "example.com", Type: "A", Host: "www", Record: "192.0.2.9", TTL: 60})
				return err
			},
		},
		{
			name:      "nameserver get array drops empty strings",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/domains/get-nameservers.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("domain-name") != "example.com" {
					t.Errorf("domain-name = %q", r.URL.Query().Get("domain-name"))
				}
				io.WriteString(w, `["ns1.example.net","","ns2.example.net"]`)
			},
			call: func(t *testing.T, c *Client) error {
				got, _, err := c.GetNameservers(context.Background(), "example.com")
				if err != nil {
					return err
				}
				want := []string{"ns1.example.net", "ns2.example.net"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("nameservers = %#v", got)
				}
				return nil
			},
		},
		{
			name:      "nameserver get numeric object drops empty strings",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/domains/get-nameservers.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"2":"ns2.example.net","1":"ns1.example.net","3":"","10":"ns10.example.net"}`)
			},
			call: func(t *testing.T, c *Client) error {
				got, _, err := c.GetNameservers(context.Background(), "example.com")
				if err != nil {
					return err
				}
				want := []string{"ns1.example.net", "ns2.example.net", "ns10.example.net"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("nameservers = %#v", got)
				}
				return nil
			},
		},
		{
			name:      "nameserver set sends glue address",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/domains/set-nameservers.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("domain-name") != "example.be" {
					t.Errorf("domain-name = %q", q.Get("domain-name"))
				}
				got := q["nameservers[]"]
				want := []string{"ns1.example.be 203.0.113.10", "ns2.example.be"}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("nameservers[] = %#v", got)
				}
				io.WriteString(w, `{"status":"Success","statusDescription":"The nameservers were set successfully."}`)
			},
			call: func(t *testing.T, c *Client) error {
				ns, err := ParseNameserverArgs("example.be", []string{"ns1.example.be", "203.0.113.10", "ns2.example.be"})
				if err != nil {
					return err
				}
				_, err = c.SetNameservers(context.Background(), "example.be", ns)
				return err
			},
		},
		{
			name:      "glue address is not sent for other tlds",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  nil,
			wantErr:   "glue address",
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Errorf("API was called for rejected glue on %s", r.URL.Path)
			},
			call: func(t *testing.T, c *Client) error {
				_, err := ParseNameserverArgs("example.com", []string{"ns1.example.com", "203.0.113.10"})
				return err
			},
		},
		{
			name:      "invalid ttl does not call the api",
			creds:     baseCreds,
			checkHits: true,
			wantErr:   "invalid ttl 123",
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Errorf("API was called for invalid ttl on %s", r.URL.Path)
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.AddRecord(context.Background(), RecordSpec{Domain: "example.com", Type: "A", Host: "@", Record: "192.0.2.10", TTL: 123})
				return err
			},
		},
		{
			name:      "srv add sends priority weight and port",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/dns/add-record.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("record-type") != "SRV" || q.Get("host") != "_sip._tcp" || q.Get("record") != "sip.example.com" {
					t.Errorf("type=%q host=%q record=%q", q.Get("record-type"), q.Get("host"), q.Get("record"))
				}
				if q.Get("priority") != "10" || q.Get("weight") != "5" || q.Get("port") != "5060" {
					t.Errorf("priority=%q weight=%q port=%q", q.Get("priority"), q.Get("weight"), q.Get("port"))
				}
				io.WriteString(w, `{"status":"Success","statusDescription":"The record was added successfully."}`)
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.AddRecord(context.Background(), RecordSpec{
					Domain: "example.com", Type: "SRV", Host: "_sip._tcp", Record: "sip.example.com", TTL: 3600,
					Params: map[string]string{"priority": "10", "weight": "5", "port": "5060"},
				})
				return err
			},
		},
		{
			name:      "modify sends extra fields and omits type and status",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/dns/mod-record.json"},
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("record-id") != "42" || q.Get("priority") != "20" || q.Get("record") != "mail.example.com" {
					t.Errorf("id=%q priority=%q record=%q", q.Get("record-id"), q.Get("priority"), q.Get("record"))
				}
				if _, ok := q["record-type"]; ok {
					t.Error("mod-record sent record-type")
				}
				if _, ok := q["status"]; ok {
					t.Error("mod-record sent status")
				}
				io.WriteString(w, `{"status":"Success","statusDescription":"The record was modified successfully."}`)
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.ModifyRecord(context.Background(), RecordSpec{
					Domain: "example.com", Type: "MX", ID: "42", Host: "@", Record: "mail.example.com", TTL: 3600,
					Params: map[string]string{"priority": "20", "status": "0"},
				})
				return err
			},
		},
		{
			name:      "mx without priority does not call the api",
			creds:     baseCreds,
			checkHits: true,
			wantErr:   "--priority",
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Errorf("API was called for incomplete MX on %s", r.URL.Path)
			},
			call: func(t *testing.T, c *Client) error {
				_, err := c.AddRecord(context.Background(), RecordSpec{Domain: "example.com", Type: "MX", Host: "@", Record: "mail.example.com", TTL: 3600})
				return err
			},
		},
		{
			name: "network error hides the url and password",
			setup: func(t *testing.T) *Client {
				c := New(Credentials{AuthID: "100", Password: testPassword})
				c.HTTP = &http.Client{
					Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
						return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: errors.New("connection refused")}
					}),
				}
				return c
			},
			call: func(t *testing.T, c *Client) error {
				return c.Login(context.Background())
			},
			wantErr:  "GET /login/login.json: connection refused",
			exactErr: true,
		},
		{
			name:      "non-200 returns the status code and a truncated body",
			creds:     baseCreds,
			checkHits: true,
			wantHits:  []string{"/login/login.json"},
			wantErr:   "HTTP 500",
			handle: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, testPassword+strings.Repeat("x", 500))
			},
			call: func(t *testing.T, c *Client) error {
				err := c.Login(context.Background())
				if err == nil {
					return errors.New("expected HTTP error")
				}
				if !strings.Contains(err.Error(), "...") {
					t.Fatalf("expected truncated body in %q", err.Error())
				}
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			creds := tt.creds
			if creds.Password == "" {
				creds = baseCreds
			}
			var c *Client
			var (
				mu   sync.Mutex
				hits []string
			)
			if tt.setup != nil {
				c = tt.setup(t)
			} else {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet {
						t.Errorf("method = %s, want GET", r.Method)
					}
					body, _ := io.ReadAll(r.Body)
					if len(strings.TrimSpace(string(body))) != 0 {
						t.Errorf("expected empty body, got %d bytes", len(body))
					}
					expectAuth(t, r.URL.Query(), creds)
					mu.Lock()
					hits = append(hits, r.URL.Path)
					mu.Unlock()
					if tt.handle != nil {
						tt.handle(t, w, r)
					}
				}))
				t.Cleanup(srv.Close)
				c = New(creds)
				c.BaseURL = srv.URL
				if tt.pageSize != 0 {
					c.RowsPerPage = tt.pageSize
				}
			}

			err := tt.call(t, c)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			} else if tt.exactErr && err.Error() != tt.wantErr {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			} else if !tt.exactErr && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
			if err != nil {
				assertNoSecret(t, err.Error(), testPassword)
			}
			mu.Lock()
			gotHits := append([]string(nil), hits...)
			mu.Unlock()
			if tt.checkHits && !reflect.DeepEqual(gotHits, tt.wantHits) {
				t.Fatalf("paths = %#v, want %#v", gotHits, tt.wantHits)
			}
		})
	}
}

func TestParseNameserverArgs(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		args    []string
		want    []string
		wantErr string
	}{
		{
			name:   "ipv4 follows hostname on a glue tld",
			domain: "example.be",
			args:   []string{"ns1.example.be", "203.0.113.10", "ns2.example.be"},
			want:   []string{"ns1.example.be 203.0.113.10", "ns2.example.be"},
		},
		{
			name:   "hostname and ipv4 in one argument",
			domain: "Example.DE",
			args:   []string{"ns1.example.de 198.51.100.10"},
			want:   []string{"ns1.example.de 198.51.100.10"},
		},
		{
			name:   "plain delegation",
			domain: "example.com",
			args:   []string{"ada.ns.cloudflare.com", "bob.ns.cloudflare.com"},
			want:   []string{"ada.ns.cloudflare.com", "bob.ns.cloudflare.com"},
		},
		{
			name:    "glue rejected for other tlds",
			domain:  "example.com",
			args:    []string{"ns1.example.com", "203.0.113.10"},
			wantErr: "glue address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNameserverArgs(tt.domain, tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSortDomains(t *testing.T) {
	domains := []RegisteredDomain{
		{Name: "later.example", Expires: "2027-06-01"},
		{Name: "soon.example", Expires: "2026-01-02"},
		{Name: "undated.example"},
		{Name: "mid.example", Expires: "2026-12-01"},
	}
	if err := SortDomains(domains, "expires"); err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(domains))
	for i, domain := range domains {
		got[i] = domain.Name
	}
	want := []string{"soon.example", "mid.example", "later.example", "undated.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted = %#v", got)
	}
	if err := SortDomains(domains, "nope"); err == nil {
		t.Fatal("expected an error for an unknown sort")
	}
}

func expectAuth(t *testing.T, q url.Values, creds Credentials) {
	t.Helper()
	switch {
	case creds.SubAuthUser != "":
		if q.Get("sub-auth-user") != creds.SubAuthUser {
			t.Errorf("sub-auth-user = %q", q.Get("sub-auth-user"))
		}
		if q.Get("auth-id") != "" || q.Get("sub-auth-id") != "" {
			t.Error("sent more than one identity parameter")
		}
	case creds.SubAuthID != "":
		if q.Get("sub-auth-id") != creds.SubAuthID {
			t.Errorf("sub-auth-id = %q", q.Get("sub-auth-id"))
		}
		if q.Get("auth-id") != "" || q.Get("sub-auth-user") != "" {
			t.Error("sent more than one identity parameter")
		}
	default:
		if q.Get("auth-id") != creds.AuthID {
			t.Errorf("auth-id = %q", q.Get("auth-id"))
		}
		if q.Get("sub-auth-id") != "" || q.Get("sub-auth-user") != "" {
			t.Error("sent more than one identity parameter")
		}
	}
	if q.Get("auth-password") == "" || q.Get("auth-password") != creds.Password {
		t.Error("auth-password mismatch")
	}
}

func assertNoSecret(t *testing.T, msg, secret string) {
	t.Helper()
	if strings.Contains(msg, secret) || strings.Contains(msg, "auth-password=") || strings.Contains(msg, "?") || strings.Contains(msg, "http://") || strings.Contains(msg, "https://") {
		t.Fatalf("error leaks the request or password: %q", msg)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
