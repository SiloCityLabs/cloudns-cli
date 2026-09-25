package api

import (
	"strings"
	"testing"
)

func TestRecordSpecValidate(t *testing.T) {
	base := func(typ string) RecordSpec {
		return RecordSpec{Domain: "example.com", Type: typ, Host: "@", TTL: 3600, Params: map[string]string{}}
	}
	tests := []struct {
		name    string
		spec    RecordSpec
		wantErr string
	}{
		{name: "a", spec: RecordSpec{Domain: "example.com", Type: "a", Host: "www", Record: "192.0.2.10", TTL: 3600}},
		{name: "mx missing priority", spec: RecordSpec{Domain: "example.com", Type: "MX", Host: "@", Record: "mail.example.com", TTL: 3600}, wantErr: "--priority"},
		{name: "mx", spec: RecordSpec{Domain: "example.com", Type: "MX", Host: "@", Record: "mail.example.com", TTL: 3600, Params: map[string]string{"priority": "10"}}},
		{name: "a rejects priority", spec: RecordSpec{Domain: "example.com", Type: "A", Host: "www", Record: "192.0.2.10", TTL: 3600, Params: map[string]string{"priority": "10"}}, wantErr: "do not use --priority"},
		{name: "caa flag", spec: RecordSpec{Domain: "example.com", Type: "CAA", Host: "@", TTL: 3600, Params: map[string]string{"caa_flag": "7", "caa_type": "issue", "caa_value": "letsencrypt.org"}}, wantErr: "--caa-flag"},
		{name: "caa", spec: RecordSpec{Domain: "example.com", Type: "CAA", Host: "@", TTL: 3600, Params: map[string]string{"caa_flag": "0", "caa_type": "Issue", "caa_value": "letsencrypt.org"}}},
		{name: "hinfo", spec: RecordSpec{Domain: "example.com", Type: "HINFO", Host: "www", TTL: 3600, Params: map[string]string{"cpu": "AMD", "os": "Linux"}}},
		{name: "hinfo rejects value", spec: RecordSpec{Domain: "example.com", Type: "HINFO", Host: "www", Record: "AMD", TTL: 3600, Params: map[string]string{"cpu": "AMD", "os": "Linux"}}, wantErr: "do not take a value"},
		{name: "loc", spec: RecordSpec{Domain: "example.com", Type: "LOC", Host: "@", TTL: 3600, Params: map[string]string{"lat_deg": "42", "lat_dir": "n", "long_deg": "71", "long_dir": "W", "altitude": "10"}}},
		{name: "rp", spec: RecordSpec{Domain: "example.com", Type: "RP", Host: "@", TTL: 3600, Params: map[string]string{"mail": "admin@example.com", "txt": "txt.example.com"}}},
		{name: "https", spec: RecordSpec{Domain: "example.com", Type: "HTTPS", Host: "@", Record: ".", TTL: 3600, Params: map[string]string{"priority": "1", "parameters": "alpn=h2"}}},
		{name: "unknown", spec: base("FOO"), wantErr: "unsupported record type"},
		{name: "redirect", spec: RecordSpec{Domain: "example.com", Type: "WR", Host: "www", Record: "https://example.com/", TTL: 3600, Params: map[string]string{"redirect_type": "200"}}, wantErr: "--redirect-type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
	spec := RecordSpec{Domain: "example.com", Type: "CAA", Host: "@", TTL: 3600, Params: map[string]string{"caa_flag": "0", "caa_type": "Issue", "caa_value": "letsencrypt.org"}}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	if spec.Params["caa_type"] != "issue" {
		t.Fatalf("caa_type = %q", spec.Params["caa_type"])
	}
}
