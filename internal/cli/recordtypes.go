package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// recordTypeExamples are the types in the ClouDNS add-record form.
// WR is shown there as "Web Redirect".
var recordTypeExamples = []recordTypeExample{
	{Type: "A", Name: "IPv4 address", Example: "cloudns zone record add example.com A www 192.0.2.10"},
	{Type: "AAAA", Name: "IPv6 address", Example: "cloudns zone record add example.com AAAA www 2001:db8::1"},
	{Type: "ALIAS", Name: "Apex alias to a hostname", Example: "cloudns zone record add example.com ALIAS @ example.net"},
	{Type: "CAA", Name: "Certificate authority authorization", Example: "cloudns zone record add example.com CAA @ letsencrypt.org --caa-flag 0 --caa-type issue"},
	{Type: "CERT", Name: "Certificate or CRL", Example: "cloudns zone record add example.com CERT @ <base64> --cert-type 6 --cert-key-tag 0 --cert-algorithm 0"},
	{Type: "CNAME", Name: "Canonical name", Example: "cloudns zone record add example.com CNAME www target.example.net"},
	{Type: "DNAME", Name: "Delegation of a subtree", Example: "cloudns zone record add example.com DNAME sub example.net"},
	{Type: "DS", Name: "DNSSEC delegation signer. That host also needs an NS record", Example: "cloudns zone record add example.com DS ds <digest> --key-tag 12345 --algorithm 13 --digest-type 2"},
	{Type: "HINFO", Name: "Host CPU and operating system", Example: "cloudns zone record add example.com HINFO www --cpu AMD --os Linux"},
	{Type: "HTTPS", Name: "HTTPS service binding", Example: "cloudns zone record add example.com HTTPS @ . --priority 1 --parameters alpn=h2"},
	{Type: "LOC", Name: "Geographic location", Example: "cloudns zone record add example.com LOC @ --lat-deg 42 --lat-min 21 --lat-sec 30 --lat-dir N --long-deg 71 --long-min 5 --long-sec 30 --long-dir W --altitude 10"},
	{Type: "MX", Name: "Mail exchange", Example: "cloudns zone record add example.com MX @ mail.example.com --priority 10"},
	{Type: "NAPTR", Name: "Naming authority pointer", Example: "cloudns zone record add example.com NAPTR @ --order 100 --pref 10 --flag U --params E2U+sip --regexp '!^.*$!sip:info@example.com!'"},
	{Type: "NS", Name: "Nameserver inside the zone", Example: "cloudns zone record add example.com NS @ ns1.example.com"},
	{Type: "OPENPGPKEY", Name: "OpenPGP public key", Example: "cloudns zone record add example.com OPENPGPKEY <hashed-name> <base64>"},
	{Type: "PTR", Name: "Pointer, usually in a reverse zone", Example: "cloudns zone record add 2.0.192.in-addr.arpa PTR 10 host.example.com"},
	{Type: "RP", Name: "Responsible person", Example: "cloudns zone record add example.com RP @ --mail admin@example.com --txt txt.example.com"},
	{Type: "SMIMEA", Name: "S/MIME certificate association", Example: "cloudns zone record add example.com SMIMEA name <certificate> --smimea-usage 3 --smimea-selector 1 --smimea-matching-type 1"},
	{Type: "SPF", Name: "Sender policy (obsolete; prefer TXT)", Example: "cloudns zone record add example.com SPF @ \"v=spf1 a -all\""},
	{Type: "SRV", Name: "Service location", Example: "cloudns zone record add example.com SRV _sip._tcp sip.example.com --priority 10 --weight 5 --port 5060"},
	{Type: "SSHFP", Name: "SSH key fingerprint", Example: "cloudns zone record add example.com SSHFP www <fingerprint> --algorithm 2 --fp-type 1"},
	{Type: "SVCB", Name: "Service binding", Example: "cloudns zone record add example.com SVCB _dns dns.example.com --priority 1 --parameters alpn=h2"},
	{Type: "TLSA", Name: "TLS certificate association", Example: "cloudns zone record add example.com TLSA _443._tcp <certificate> --tlsa-usage 3 --tlsa-selector 1 --tlsa-matching-type 1"},
	{Type: "TXT", Name: "Text", Example: "cloudns zone record add example.com TXT @ \"v=spf1 include:_spf.example.com ~all\""},
	{Type: "WR", Name: "Web redirect", Example: "cloudns zone record add example.com WR www https://example.com/ --redirect-type 301"},
}

type recordTypeExample struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Example string `json:"example"`
}

func newZoneTypesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "Show every record type, with an example command",
		Long: `Show the record types from the ClouDNS add-record form.

Each example is a cloudns zone record add command. The host @ is the zone apex.
This command does not call the API.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantsJSON(cmd) {
				raw, err := json.Marshal(recordTypeExamples)
				if err != nil {
					return err
				}
				return printRaw(raw)
			}
			rows := make([][]string, len(recordTypeExamples))
			for i, rec := range recordTypeExamples {
				rows[i] = []string{rec.Type, rec.Name, rec.Example}
			}
			return printTable([]string{"TYPE", "RECORD", "EXAMPLE"}, rows)
		},
	}
}

func recordTypeNames() []string {
	names := make([]string, len(recordTypeExamples))
	for i, rec := range recordTypeExamples {
		names[i] = rec.Type
	}
	return names
}
