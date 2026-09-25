package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ldrrp/cloudns-cli/internal/api"
)

func TestDomainStatus(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"0", "expired"},
		{"1", "active"},
		{"2", "transfer"},
		{"3", "transfer failed"},
		{"expire", "expire"},
	}
	for _, tt := range tests {
		if got := domainStatus(tt.in); got != tt.want {
			t.Errorf("domainStatus(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCellAndDisplayHost(t *testing.T) {
	if cell("a\tb\nc") != "a b c" {
		t.Fatalf("cell = %q", cell("a\tb\nc"))
	}
	if displayHost("  ") != "@" || displayHost("www") != "www" {
		t.Fatalf("displayHost blank=%q www=%q", displayHost("  "), displayHost("www"))
	}
}

func TestPrintRecordsAndResult(t *testing.T) {
	out := captureStdout(t, func() {
		if err := printRecords([]api.Record{{ID: "9", Type: "A", Host: "", Value: "192.0.2.1", TTL: "3600"}}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "ID") || !strings.Contains(out, "@") || !strings.Contains(out, "192.0.2.1") {
		t.Fatalf("records table = %q", out)
	}

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	out = captureStdout(t, func() {
		if err := printResult(cmd, []byte(`{"status":"Success","statusDescription":"The record was added successfully."}`)); err != nil {
			t.Fatal(err)
		}
	})
	if strings.TrimSpace(out) != "The record was added successfully." {
		t.Fatalf("result = %q", out)
	}

	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		if err := printResult(cmd, []byte(`{"status":"Success"}`)); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "{\n  \"status\": \"Success\"\n}") {
		t.Fatalf("json = %q", out)
	}
}

func TestPrintZoneInfoAndNameservers(t *testing.T) {
	out := captureStdout(t, func() {
		if err := printZoneInfo(map[string]string{"name": "example.com", "zone": "domain", "extra": "yes"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "example.com") || !strings.Contains(out, "EXTRA") {
		t.Fatalf("zone info = %q", out)
	}
	out = captureStdout(t, func() {
		printNameserverSet("New nameservers", []string{"ns1.example.net"})
		printNameserverSet("Previous nameservers", nil)
	})
	if !strings.Contains(out, "ns1.example.net") || !strings.Contains(out, "(none)") {
		t.Fatalf("nameservers = %q", out)
	}
}

func TestPrintFailoverSettingsFlattensCheckSettings(t *testing.T) {
	out := captureStdout(t, func() {
		raw := []byte(`{"check_type":17,"check_settings":{"ping_threshold":"25","timeout":"2"},"main_ip":"192.0.2.10"}`)
		if err := printFailoverSettings(raw); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "PING_THRESHOLD") || !strings.Contains(out, "25") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "CHECK_SETTINGS") {
		t.Fatalf("nested object was not flattened: %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = old }()
	fn()
	writer.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
