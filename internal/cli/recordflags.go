package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ldrrp/cloudns-cli/internal/api"
)

type recordFieldFlag struct {
	flag  string
	param string
	help  string
}

var recordFieldFlags = []recordFieldFlag{
	{"priority", "priority", "Priority for MX, SRV, HTTPS, or SVCB"},
	{"weight", "weight", "Weight for SRV"},
	{"port", "port", "Port for SRV"},
	{"frame", "frame", "WR frame: 0 off, 1 on"},
	{"frame-title", "frame_title", "WR frame title"},
	{"frame-keywords", "frame_keywords", "WR frame keywords"},
	{"frame-description", "frame_description", "WR frame description"},
	{"frame-favicon", "frame_favicon", "WR frame favicon URL"},
	{"mobile-meta", "mobile_meta", "WR frame mobile meta tags: 0 or 1"},
	{"save-path", "save_path", "WR save path: 0 or 1"},
	{"redirect-type", "redirect_type", "WR redirect when the frame is off: 301 or 302"},
	{"mail", "mail", "RP email address"},
	{"txt", "txt", "RP TXT hostname"},
	{"algorithm", "algorithm", "SSHFP or DS algorithm"},
	{"fp-type", "fp_type", "SSHFP fingerprint type"},
	{"status", "status", "1 active, 0 inactive (add only)"},
	{"geodns-location", "geodns-location", "GeoDNS location ID for A, AAAA, CNAME, NAPTR, or SRV"},
	{"geodns-code", "geodns-code", "GeoDNS location code for A, AAAA, CNAME, NAPTR, or SRV"},
	{"caa-flag", "caa_flag", "CAA flag: 0 or 128"},
	{"caa-type", "caa_type", "CAA tag: issue, issuewild, iodef, issuemail, issuevmc, contactemail, or contactphone"},
	{"caa-value", "caa_value", "CAA value, if you do not pass it as the record argument"},
	{"tlsa-usage", "tlsa_usage", "TLSA usage (0-3)"},
	{"tlsa-selector", "tlsa_selector", "TLSA selector (0-1)"},
	{"tlsa-matching-type", "tlsa_matching_type", "TLSA matching type (0-2)"},
	{"key-tag", "key_tag", "DS key tag"},
	{"digest-type", "digest_type", "DS digest type"},
	{"order", "order", "NAPTR order"},
	{"pref", "pref", "NAPTR preference"},
	{"flag", "flag", "NAPTR flag: U, S, A, or P"},
	{"params", "params", "NAPTR service parameters"},
	{"regexp", "regexp", "NAPTR regular expression"},
	{"replace", "replace", "NAPTR replacement name"},
	{"cert-type", "cert_type", "CERT type"},
	{"cert-key-tag", "cert_key_tag", "CERT key tag"},
	{"cert-algorithm", "cert_algorithm", "CERT algorithm"},
	{"lat-deg", "lat_deg", "LOC latitude degrees (0-90)"},
	{"lat-min", "lat_min", "LOC latitude minutes"},
	{"lat-sec", "lat_sec", "LOC latitude seconds"},
	{"lat-dir", "lat_dir", "LOC latitude direction: N or S"},
	{"long-deg", "long_deg", "LOC longitude degrees (0-180)"},
	{"long-min", "long_min", "LOC longitude minutes"},
	{"long-sec", "long_sec", "LOC longitude seconds"},
	{"long-dir", "long_dir", "LOC longitude direction: E or W"},
	{"altitude", "altitude", "LOC altitude in meters (required)"},
	{"size", "size", "LOC size in meters"},
	{"h-precision", "h_precision", "LOC horizontal precision in meters"},
	{"v-precision", "v_precision", "LOC vertical precision in meters"},
	{"cpu", "cpu", "HINFO CPU"},
	{"os", "os", "HINFO operating system"},
	{"smimea-usage", "smimea_usage", "SMIMEA usage (0-3)"},
	{"smimea-selector", "smimea_selector", "SMIMEA selector (0-1)"},
	{"smimea-matching-type", "smimea_matching_type", "SMIMEA matching type (0-2)"},
	{"parameters", "parameters", "HTTPS or SVCB parameters, such as alpn=h2"},
}

func addRecordFieldFlags(cmd *cobra.Command, withStatus bool) {
	cmd.Flags().Int("ttl", 3600, "TTL in seconds (60, 300, 900, 1800, 3600, 21600, 43200, 86400, 172800, 259200, 604800, 1209600, 2592000)")
	for _, f := range recordFieldFlags {
		if f.param == "status" && !withStatus {
			continue
		}
		cmd.Flags().String(f.flag, "", f.help)
	}
}

func specFromRecordArgs(cmd *cobra.Command, domain, typ, id, host, value string) (api.RecordSpec, error) {
	ttl, err := cmd.Flags().GetInt("ttl")
	if err != nil {
		return api.RecordSpec{}, err
	}
	spec := api.RecordSpec{
		Domain: domain,
		Type:   typ,
		ID:     id,
		Host:   host,
		TTL:    ttl,
		Params: map[string]string{},
	}
	for _, f := range recordFieldFlags {
		flag := cmd.Flags().Lookup(f.flag)
		if flag == nil || !cmd.Flags().Changed(f.flag) {
			continue
		}
		v, err := cmd.Flags().GetString(f.flag)
		if err != nil {
			return api.RecordSpec{}, err
		}
		spec.Params[f.param] = v
	}
	if err := applyRecordValue(&spec, value); err != nil {
		return api.RecordSpec{}, err
	}
	if err := spec.Validate(); err != nil {
		return api.RecordSpec{}, err
	}
	return spec, nil
}

func applyRecordValue(spec *api.RecordSpec, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	switch strings.ToUpper(strings.TrimSpace(spec.Type)) {
	case "CAA":
		if strings.TrimSpace(spec.Params["caa_value"]) != "" {
			return errors.New("pass the CAA value as the argument or as --caa-value, not both")
		}
		spec.Params["caa_value"] = value
	case "HINFO", "LOC", "NAPTR", "RP":
		return fmt.Errorf("%s records do not take a value argument", strings.ToUpper(strings.TrimSpace(spec.Type)))
	default:
		spec.Record = value
	}
	return nil
}
