package cli

import (
	"strings"
	"testing"
)

func TestZoneDeleteRequiresYes(t *testing.T) {
	cmd := newZoneDeleteCmd()
	cmd.SetArgs([]string{"example.com"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}
}

func TestSOASetRequiresAField(t *testing.T) {
	cmd := newZoneSOASetCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}
	_, err := soaPatchFromFlags(cmd)
	if err == nil || !strings.Contains(err.Error(), "--admin-mail") {
		t.Fatalf("error = %v", err)
	}
}

func TestSOAPatchFromFlags(t *testing.T) {
	cmd := newZoneSOASetCmd()
	if err := cmd.ParseFlags([]string{"--admin-mail", "hostmaster@example.com", "--default-ttl", "3600"}); err != nil {
		t.Fatal(err)
	}
	patch, err := soaPatchFromFlags(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if patch.AdminMail == nil || *patch.AdminMail != "hostmaster@example.com" {
		t.Fatalf("admin mail = %#v", patch.AdminMail)
	}
	if patch.DefaultTTL == nil || *patch.DefaultTTL != "3600" {
		t.Fatalf("ttl = %#v", patch.DefaultTTL)
	}
	if patch.PrimaryNS != nil || patch.Refresh != nil {
		t.Fatalf("unexpected patch %#v", patch)
	}
}

func TestZoneFileName(t *testing.T) {
	name, err := zoneFileName("example.com")
	if err != nil || name != "example.com.zone" {
		t.Fatalf("name = %q, err = %v", name, err)
	}
	if _, err := zoneFileName("../example.com"); err == nil {
		t.Fatal("accepted a path")
	}
}
