package api

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestActivateFailoverSendsCheckParameters(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dns/failover-activate.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		want := map[string]string{
			"domain-name":         "example.com",
			"record-id":           "42",
			"check_type":          "18",
			"down_event_handler":  "2",
			"up_event_handler":    "1",
			"main_ip":             "192.0.2.10",
			"backup_ip_1":         "192.0.2.11",
			"backup_ip_2":         "192.0.2.12",
			"host":                "www.example.com",
			"port":                "443",
			"http_protocol":       "https",
			"path":                "/health",
			"content":             "OK",
			"check_period":        "300",
			"monitoring_region":   "eur",
			"http_request_type":   "GET",
			"custom_header":       "X-Check",
			"custom_header_value": "1",
			"timeout":             "5",
		}
		for key, value := range want {
			if q.Get(key) != value {
				t.Errorf("%s = %q, want %q", key, q.Get(key), value)
			}
		}
		if q.Get("state") != "" {
			t.Errorf("activate sent state %q", q.Get("state"))
		}
		io.WriteString(w, `{"status":"Success","statusDescription":"The failover was activated."}`)
	})
	_, err := c.ActivateFailover(context.Background(), FailoverCheck{
		Domain:      "example.com",
		RecordID:    "42",
		Type:        "web",
		Down:        "replace",
		Up:          "activate",
		MainIP:      "192.0.2.10",
		Backups:     []string{"192.0.2.11", "192.0.2.12"},
		Region:      "eur",
		Host:        "www.example.com",
		Protocol:    "https",
		Port:        "443",
		Path:        "/health",
		Content:     "OK",
		Period:      "300",
		RequestType: "get",
		Header:      "X-Check",
		HeaderValue: "1",
		Timeout:     "5",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFailoverValidationDoesNotCallTheAPI(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	})
	base := FailoverCheck{
		Domain:   "example.com",
		RecordID: "42",
		Type:     "ping",
		Down:     "monitor",
		Up:       "monitor",
		MainIP:   "192.0.2.10",
		Backups:  []string{"192.0.2.11"},
	}
	web := base
	web.Type = "web"
	if _, err := c.ActivateFailover(context.Background(), web); err == nil {
		t.Fatal("expected web without host to fail")
	}
	base.State = "1"
	if _, err := c.ActivateFailover(context.Background(), base); err == nil {
		t.Fatal("expected --state on add to fail")
	}
	if _, err := c.ModifyFailover(context.Background(), FailoverCheck{Domain: "example.com", RecordID: "42", Type: "dns", Down: "monitor", Up: "monitor", MainIP: "192.0.2.10", Backups: []string{"192.0.2.11"}}); err == nil {
		t.Fatal("expected dns without query fields to fail")
	}
}

func TestModifyFailoverSendsState(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dns/failover-modify.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("state") != "0" || r.URL.Query().Get("check_type") != "17" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		io.WriteString(w, `{"status":"Success","statusDescription":"ok"}`)
	})
	_, err := c.ModifyFailover(context.Background(), FailoverCheck{
		Domain: "example.com", RecordID: "42", Type: "ping", Down: "monitor", Up: "ignore",
		MainIP: "192.0.2.10", Backups: []string{"192.0.2.11"}, State: "0",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFailoverUsageAndNodes(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dns/get-failover-stats.json":
			io.WriteString(w, `{"count":"0","limit":"1"}`)
		case "/dns/get-failover-servers.json":
			io.WriteString(w, `[{"id":1,"ip":"192.0.2.1","ip6":"2001:db8::1","location":"Test"}]`)
		case "/dns/failover-deactivate.json":
			if r.URL.Query().Get("domain-name") != "example.com" || r.URL.Query().Get("record-id") != "9" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"status":"Success","statusDescription":"deactivated"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	usage, _, err := c.FailoverUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Count != "0" || usage.Limit != "1" {
		t.Fatalf("usage = %#v", usage)
	}
	nodes, _, err := c.FailoverNodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "1" || nodes[0].Location != "Test" {
		t.Fatalf("nodes = %#v", nodes)
	}
	if _, err := c.DeactivateFailover(context.Background(), "example.com", "9"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeactivateFailover(context.Background(), "example.com", ""); err == nil {
		t.Fatal("expected missing record id to fail")
	}
}
