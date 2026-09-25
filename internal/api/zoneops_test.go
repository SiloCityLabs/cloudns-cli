package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRegisterZone(t *testing.T) {
	var gotPath string
	var gotNS []string
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		q := r.URL.Query()
		if q.Get("domain-name") != "example.com" || q.Get("zone-type") != "master" {
			t.Errorf("domain=%q type=%q", q.Get("domain-name"), q.Get("zone-type"))
		}
		if q.Get("master-ip") != "" {
			t.Errorf("master-ip = %q", q.Get("master-ip"))
		}
		gotNS = q["ns[]"]
		io.WriteString(w, `{"status":"Success","statusDescription":"The zone was added."}`)
	})
	_, err := c.RegisterZone(context.Background(), "example.com", "master", "", []string{"pns1.cloudns.net", "pns2.cloudns.net"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/dns/register.json" {
		t.Fatalf("path = %s", gotPath)
	}
	wantNS := []string{"pns1.cloudns.net", "pns2.cloudns.net"}
	if !reflect.DeepEqual(gotNS, wantNS) {
		t.Fatalf("ns[] = %#v", gotNS)
	}
}

func TestDeleteRecordAndMasters(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/dns/delete-record.json":
			if q.Get("domain-name") != "example.com" || q.Get("record-id") != "42" {
				t.Errorf("domain=%q id=%q", q.Get("domain-name"), q.Get("record-id"))
			}
		case "/dns/add-master-server.json":
			if q.Get("master-ip") != "192.0.2.10" {
				t.Errorf("master-ip = %q", q.Get("master-ip"))
			}
		case "/dns/delete-master-server.json":
			if q.Get("master-id") != "7" {
				t.Errorf("master-id = %q", q.Get("master-id"))
			}
		case "/dns/get-zone-info.json":
			io.WriteString(w, `{"name":"example.com","type":"master","zone":"domain","status":1}`)
			return
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		io.WriteString(w, `{"status":"Success","statusDescription":"ok"}`)
	})
	if _, err := c.DeleteRecord(context.Background(), "example.com", "42"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteRecord(context.Background(), "example.com", ""); err == nil {
		t.Fatal("expected missing record id to fail")
	}
	if _, err := c.AddMaster(context.Background(), "example.com", "not-an-ip"); err == nil {
		t.Fatal("expected invalid master IP to fail")
	}
	if _, err := c.AddMaster(context.Background(), "example.com", "192.0.2.10"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteMaster(context.Background(), "example.com", "7"); err != nil {
		t.Fatal(err)
	}
	info, _, err := c.GetZoneInfo(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if info["name"] != "example.com" || info["zone"] != "domain" || info["status"] != "1" {
		t.Fatalf("info = %#v", info)
	}
}

func TestSortDomainPayloadKeepsJSONTogether(t *testing.T) {
	domains := []RegisteredDomain{
		{Name: "later.example", Expires: "2027-01-01"},
		{Name: "soon.example", Expires: "2026-01-01"},
	}
	raw := json.RawMessage(`[{"name":"later.example","expires_on":"2027-01-01"},{"name":"soon.example","expires_on":"2026-01-01"}]`)
	sorted, out, err := SortDomainPayload(domains, raw, "expires")
	if err != nil {
		t.Fatal(err)
	}
	if sorted[0].Name != "soon.example" || sorted[1].Name != "later.example" {
		t.Fatalf("order = %#v", sorted)
	}
	var items []map[string]string
	if err := json.Unmarshal(out, &items); err != nil {
		t.Fatal(err)
	}
	if items[0]["name"] != "soon.example" || items[1]["name"] != "later.example" {
		t.Fatalf("json order = %#v", items)
	}
}

func TestCredentialsLabel(t *testing.T) {
	label, err := (Credentials{SubAuthUser: "alice"}).Label()
	if err != nil || label != "sub-auth-user alice" {
		t.Fatalf("label = %q, err = %v", label, err)
	}
	if _, err := (Credentials{}).Label(); err == nil {
		t.Fatal("expected missing auth id to fail")
	}
}

func TestRegisterSlaveRequiresMasterIP(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("API was called for %s", r.URL.Path)
	})
	_, err := c.RegisterZone(context.Background(), "example.com", "slave", "", nil)
	if err == nil || err.Error() != "slave zones require --master-ip" {
		t.Fatalf("error = %v", err)
	}
}

func TestSetSOAMergesCurrentValues(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dns/soa-details.json":
			io.WriteString(w, `{"primaryNS":"ns1.cloudns.net","adminMail":"support@cloudns.net","refresh":"7200","retry":"1800","expire":"1209600","defaultTTL":"3600"}`)
		case "/dns/modify-soa.json":
			q := r.URL.Query()
			if q.Get("primary-ns") != "ns1.cloudns.net" || q.Get("admin-mail") != "hostmaster@example.com" {
				t.Errorf("primary-ns=%q admin-mail=%q", q.Get("primary-ns"), q.Get("admin-mail"))
			}
			if q.Get("refresh") != "7200" || q.Get("retry") != "1800" || q.Get("expire") != "1209600" || q.Get("default-ttl") != "3600" {
				t.Errorf("timers refresh=%q retry=%q expire=%q ttl=%q", q.Get("refresh"), q.Get("retry"), q.Get("expire"), q.Get("default-ttl"))
			}
			io.WriteString(w, `{"status":"Success","statusDescription":"SOA was modified."}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	mail := "hostmaster@example.com"
	_, err := c.SetSOA(context.Background(), "example.com", SOAPatch{AdminMail: &mail})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetSOARejectsRefreshBeforeModify(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dns/soa-details.json" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		io.WriteString(w, `{"primaryNS":"ns1.cloudns.net","adminMail":"support@cloudns.net","refresh":"7200","retry":"1800","expire":"1209600","defaultTTL":"3600"}`)
	})
	refresh := "60"
	_, err := c.SetSOA(context.Background(), "example.com", SOAPatch{Refresh: &refresh})
	if err == nil || err.Error() != "--refresh must be from 1200 to 43200 seconds" {
		t.Fatalf("error = %v", err)
	}
}

func TestExportAndUpdateStatus(t *testing.T) {
	c := zoneTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dns/records-export.json":
			io.WriteString(w, `{"status":"Success","zone":"$TTL 3600\n@ IN NS ns1.cloudns.net.\n"}`)
		case "/dns/is-updated.json":
			io.WriteString(w, `true`)
		case "/dns/update-status.json":
			io.WriteString(w, `[{"server":"ns2.cloudns.net","ip4":"192.0.2.2","ip6":"2001:db8::2","updated":false},{"server":"ns1.cloudns.net","ip4":"192.0.2.1","ip6":"2001:db8::1","updated":true}]`)
		case "/dns/master-servers.json":
			io.WriteString(w, `{"20":"192.0.2.20","10":"192.0.2.10"}`)
		case "/dns/delete.json":
			if r.URL.Query().Get("domain-name") != "example.com" {
				t.Errorf("domain-name = %q", r.URL.Query().Get("domain-name"))
			}
			io.WriteString(w, `{"status":"Success","statusDescription":"The zone was deleted."}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	text, _, err := c.ExportZone(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if text != "$TTL 3600\n@ IN NS ns1.cloudns.net.\n" {
		t.Fatalf("zone = %q", text)
	}
	updated, _, err := c.ZoneUpdated(context.Background(), "example.com")
	if err != nil || !updated {
		t.Fatalf("updated = %v, err = %v", updated, err)
	}
	rows, _, err := c.UpdateStatus(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Server != "ns1.cloudns.net" || !rows[0].Updated || rows[1].Updated {
		t.Fatalf("status = %#v", rows)
	}
	masters, _, err := c.ListMasters(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := []MasterServer{{ID: "10", IP: "192.0.2.10"}, {ID: "20", IP: "192.0.2.20"}}
	if !reflect.DeepEqual(masters, want) {
		t.Fatalf("masters = %#v", masters)
	}
	if _, err := c.DeleteZone(context.Background(), "example.com"); err != nil {
		t.Fatal(err)
	}
}

func zoneTestClient(t *testing.T, handle func(http.ResponseWriter, *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("auth-password") == "" {
			t.Errorf("missing auth on %s", r.URL.Path)
		}
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	c := New(Credentials{AuthID: "100", Password: testPassword})
	c.BaseURL = srv.URL
	return c
}
