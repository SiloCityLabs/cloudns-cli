package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestRecordTypeExamplesMatchAddForm(t *testing.T) {
	want := []string{
		"A", "AAAA", "ALIAS", "CAA", "CERT", "CNAME", "DNAME", "DS", "HINFO",
		"HTTPS", "LOC", "MX", "NAPTR", "NS", "OPENPGPKEY", "PTR", "RP", "SMIMEA",
		"SPF", "SRV", "SSHFP", "SVCB", "TLSA", "TXT", "WR",
	}
	if !reflect.DeepEqual(recordTypeNames(), want) {
		t.Fatalf("types = %#v", recordTypeNames())
	}
	for _, rec := range recordTypeExamples {
		if rec.Name == "" || rec.Example == "" {
			t.Fatalf("%s is missing a name or example", rec.Type)
		}
		if !strings.Contains(rec.Example, "zone record add") {
			t.Fatalf("%s example is not an add command: %s", rec.Type, rec.Example)
		}
	}
}

func TestSpecFromRecordArgs(t *testing.T) {
	add := newZoneRecordAddCmd()
	if err := add.ParseFlags([]string{"--priority", "10", "--ttl", "3600"}); err != nil {
		t.Fatal(err)
	}
	spec, err := specFromRecordArgs(add, "example.com", "MX", "", "@", "mail.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Record != "mail.example.com" || spec.Params["priority"] != "10" {
		t.Fatalf("spec = %#v", spec)
	}

	caa := newZoneRecordAddCmd()
	if err := caa.ParseFlags([]string{"--caa-flag", "0", "--caa-type", "issue"}); err != nil {
		t.Fatal(err)
	}
	spec, err = specFromRecordArgs(caa, "example.com", "CAA", "", "@", "letsencrypt.org")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Record != "" || spec.Params["caa_value"] != "letsencrypt.org" {
		t.Fatalf("caa spec = %#v", spec)
	}

	missing := newZoneRecordAddCmd()
	if err := missing.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}
	_, err = specFromRecordArgs(missing, "example.com", "MX", "", "@", "mail.example.com")
	if err == nil || !strings.Contains(err.Error(), "--priority") {
		t.Fatalf("error = %v", err)
	}

	edit := newZoneRecordEditCmd()
	if edit.Flags().Lookup("status") != nil {
		t.Fatal("edit should not accept --status")
	}
}

func TestRecordTypeExamplesAreValidCommands(t *testing.T) {
	for _, rec := range recordTypeExamples {
		args := splitCommand(rec.Example)
		if len(args) < 5 || args[3] != "add" {
			t.Fatalf("%s example: %v", rec.Type, args)
		}
		cmd := newZoneRecordAddCmd()
		if err := cmd.ParseFlags(args[4:]); err != nil {
			t.Fatalf("%s: %v", rec.Type, err)
		}
		pos := cmd.Flags().Args()
		if len(pos) < 3 || len(pos) > 4 {
			t.Fatalf("%s positional = %#v", rec.Type, pos)
		}
		value := ""
		if len(pos) == 4 {
			value = pos[3]
		}
		if _, err := specFromRecordArgs(cmd, pos[0], pos[1], "", pos[2], value); err != nil {
			t.Fatalf("%s: %v", rec.Type, err)
		}
	}
}

func splitCommand(s string) []string {
	var args []string
	var b strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
		case r == ' ':
			if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}
	return args
}
