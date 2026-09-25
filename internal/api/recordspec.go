package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// RecordSpec is one DNS record for add-record or mod-record.
// Type is sent only when adding. Params holds optional API fields such as priority.
type RecordSpec struct {
	Domain string
	Type   string
	ID     string
	Host   string
	Record string
	TTL    int
	Params map[string]string
}

type recordKind struct {
	needsRecord bool
	required    []string
	optional    []string
}

// Kinds match the ClouDNS add-record form. status is allowed on every type
// and is sent only when adding. GeoDNS fields are allowed only where the API lists them.
var recordKinds = map[string]recordKind{
	"A":          {needsRecord: true, optional: []string{"geodns-location", "geodns-code"}},
	"AAAA":       {needsRecord: true, optional: []string{"geodns-location", "geodns-code"}},
	"ALIAS":      {needsRecord: true},
	"CAA":        {required: []string{"caa_flag", "caa_type", "caa_value"}},
	"CERT":       {needsRecord: true, required: []string{"cert_type", "cert_key_tag", "cert_algorithm"}},
	"CNAME":      {needsRecord: true, optional: []string{"geodns-location", "geodns-code"}},
	"DNAME":      {needsRecord: true},
	"DS":         {needsRecord: true, required: []string{"key_tag", "algorithm", "digest_type"}},
	"HINFO":      {required: []string{"cpu", "os"}},
	"HTTPS":      {needsRecord: true, optional: []string{"priority", "parameters"}},
	"LOC":        {required: []string{"lat_deg", "lat_dir", "long_deg", "long_dir", "altitude"}, optional: []string{"lat_min", "lat_sec", "long_min", "long_sec", "size", "h_precision", "v_precision"}},
	"MX":         {needsRecord: true, required: []string{"priority"}},
	"NAPTR":      {required: []string{"order", "pref", "flag", "params"}, optional: []string{"regexp", "replace", "geodns-location", "geodns-code"}},
	"NS":         {needsRecord: true},
	"OPENPGPKEY": {needsRecord: true},
	"PTR":        {needsRecord: true},
	"RP":         {required: []string{"mail", "txt"}},
	"SMIMEA":     {needsRecord: true, required: []string{"smimea_usage", "smimea_selector", "smimea_matching_type"}},
	"SPF":        {needsRecord: true},
	"SRV":        {needsRecord: true, required: []string{"priority", "weight", "port"}, optional: []string{"geodns-location", "geodns-code"}},
	"SSHFP":      {needsRecord: true, required: []string{"algorithm", "fp_type"}},
	"SVCB":       {needsRecord: true, optional: []string{"priority", "parameters"}},
	"TLSA":       {needsRecord: true, required: []string{"tlsa_usage", "tlsa_selector", "tlsa_matching_type"}},
	"TXT":        {needsRecord: true},
	"WR":         {needsRecord: true, optional: []string{"frame", "frame_title", "frame_keywords", "frame_description", "frame_favicon", "mobile_meta", "save_path", "redirect_type"}},
}

var paramFlag = map[string]string{
	"priority":             "--priority",
	"weight":               "--weight",
	"port":                 "--port",
	"frame":                "--frame",
	"frame_title":          "--frame-title",
	"frame_keywords":       "--frame-keywords",
	"frame_description":    "--frame-description",
	"frame_favicon":        "--frame-favicon",
	"mobile_meta":          "--mobile-meta",
	"save_path":            "--save-path",
	"redirect_type":        "--redirect-type",
	"mail":                 "--mail",
	"txt":                  "--txt",
	"algorithm":            "--algorithm",
	"fp_type":              "--fp-type",
	"status":               "--status",
	"geodns-location":      "--geodns-location",
	"geodns-code":          "--geodns-code",
	"caa_flag":             "--caa-flag",
	"caa_type":             "--caa-type",
	"caa_value":            "--caa-value",
	"tlsa_usage":           "--tlsa-usage",
	"tlsa_selector":        "--tlsa-selector",
	"tlsa_matching_type":   "--tlsa-matching-type",
	"key_tag":              "--key-tag",
	"digest_type":          "--digest-type",
	"order":                "--order",
	"pref":                 "--pref",
	"flag":                 "--flag",
	"params":               "--params",
	"regexp":               "--regexp",
	"replace":              "--replace",
	"cert_type":            "--cert-type",
	"cert_key_tag":         "--cert-key-tag",
	"cert_algorithm":       "--cert-algorithm",
	"lat_deg":              "--lat-deg",
	"lat_min":              "--lat-min",
	"lat_sec":              "--lat-sec",
	"lat_dir":              "--lat-dir",
	"long_deg":             "--long-deg",
	"long_min":             "--long-min",
	"long_sec":             "--long-sec",
	"long_dir":             "--long-dir",
	"altitude":             "--altitude",
	"size":                 "--size",
	"h_precision":          "--h-precision",
	"v_precision":          "--v-precision",
	"cpu":                  "--cpu",
	"os":                   "--os",
	"smimea_usage":         "--smimea-usage",
	"smimea_selector":      "--smimea-selector",
	"smimea_matching_type": "--smimea-matching-type",
	"parameters":           "--parameters",
}

var intParams = map[string]struct{}{
	"priority": {}, "weight": {}, "port": {}, "frame": {}, "mobile_meta": {},
	"save_path": {}, "redirect_type": {}, "algorithm": {}, "fp_type": {},
	"status": {}, "geodns-location": {}, "caa_flag": {}, "key_tag": {},
	"digest_type": {}, "cert_type": {}, "cert_key_tag": {}, "cert_algorithm": {},
	"lat_deg": {}, "lat_min": {}, "long_deg": {}, "long_min": {},
	"tlsa_usage": {}, "tlsa_selector": {}, "tlsa_matching_type": {},
	"smimea_usage": {}, "smimea_selector": {}, "smimea_matching_type": {},
}

var decimalParams = map[string]struct{}{
	"lat_sec": {}, "long_sec": {}, "altitude": {}, "size": {},
	"h_precision": {}, "v_precision": {},
}

var caaTypes = map[string]struct{}{
	"issue": {}, "issuewild": {}, "iodef": {}, "issuemail": {},
	"issuevmc": {}, "contactemail": {}, "contactphone": {},
}

// Validate checks the fields ClouDNS requires for this record type.
func (s *RecordSpec) Validate() error {
	s.Domain = strings.TrimSpace(s.Domain)
	s.Type = strings.ToUpper(strings.TrimSpace(s.Type))
	s.ID = strings.TrimSpace(s.ID)
	s.Host = strings.TrimSpace(s.Host)
	s.Record = strings.TrimSpace(s.Record)
	if s.Domain == "" {
		return fmt.Errorf("domain name is required")
	}
	if s.Type == "" {
		return fmt.Errorf("record type is required")
	}
	if s.Host == "" {
		return fmt.Errorf("host is required; use @ for the zone apex")
	}
	if err := ValidateTTL(s.TTL); err != nil {
		return err
	}
	kind, ok := recordKinds[s.Type]
	if !ok {
		return fmt.Errorf("unsupported record type %s; run cloudns zone types", s.Type)
	}
	if s.Params == nil {
		s.Params = map[string]string{}
	}
	for name, value := range s.Params {
		value = strings.TrimSpace(value)
		if value == "" {
			delete(s.Params, name)
			continue
		}
		s.Params[name] = value
		if !kind.allows(name) {
			return fmt.Errorf("%s records do not use %s", s.Type, flagName(name))
		}
		if err := checkParam(name, value); err != nil {
			return err
		}
	}
	if err := normalizeEnums(s.Type, s.Params); err != nil {
		return err
	}
	if kind.needsRecord && s.Record == "" {
		return fmt.Errorf("%s records require a value", s.Type)
	}
	if !kind.needsRecord && s.Record != "" {
		return fmt.Errorf("%s records do not take a value argument", s.Type)
	}
	for _, name := range kind.required {
		if s.Params[name] == "" {
			return fmt.Errorf("%s records require %s", s.Type, flagName(name))
		}
	}
	if s.Type == "NAPTR" {
		hasRegexp := s.Params["regexp"] != ""
		hasReplace := s.Params["replace"] != ""
		if hasRegexp && hasReplace {
			return fmt.Errorf("NAPTR records take either --regexp or --replace")
		}
		if !hasRegexp && !hasReplace {
			return fmt.Errorf("NAPTR records require --regexp or --replace")
		}
	}
	return nil
}

func (k recordKind) allows(name string) bool {
	if name == "status" {
		return true
	}
	for _, p := range k.required {
		if p == name {
			return true
		}
	}
	for _, p := range k.optional {
		if p == name {
			return true
		}
	}
	return false
}

func checkParam(name, value string) error {
	if _, ok := intParams[name]; ok {
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("%s must be an integer", flagName(name))
		}
	}
	if _, ok := decimalParams[name]; ok {
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("%s must be a number", flagName(name))
		}
	}
	return nil
}

func normalizeEnums(typ string, params map[string]string) error {
	if v, ok := params["caa_flag"]; ok && v != "0" && v != "128" {
		return fmt.Errorf("--caa-flag must be 0 or 128")
	}
	if v, ok := params["caa_type"]; ok {
		low := strings.ToLower(v)
		if _, known := caaTypes[low]; !known {
			return fmt.Errorf("--caa-type must be issue, issuewild, iodef, issuemail, issuevmc, contactemail, or contactphone")
		}
		params["caa_type"] = low
	}
	if v, ok := params["frame"]; ok && v != "0" && v != "1" {
		return fmt.Errorf("--frame must be 0 or 1")
	}
	if v, ok := params["save_path"]; ok && v != "0" && v != "1" {
		return fmt.Errorf("--save-path must be 0 or 1")
	}
	if v, ok := params["mobile_meta"]; ok && v != "0" && v != "1" {
		return fmt.Errorf("--mobile-meta must be 0 or 1")
	}
	if v, ok := params["status"]; ok && v != "0" && v != "1" {
		return fmt.Errorf("--status must be 0 or 1")
	}
	if v, ok := params["redirect_type"]; ok && v != "301" && v != "302" {
		return fmt.Errorf("--redirect-type must be 301 or 302")
	}
	if v, ok := params["flag"]; ok && typ == "NAPTR" {
		flag := strings.ToUpper(v)
		switch flag {
		case "U", "S", "A", "P":
			params["flag"] = flag
		default:
			return fmt.Errorf("--flag must be U, S, A, or P")
		}
	}
	if v, ok := params["mail"]; ok && typ == "RP" && !strings.Contains(v, "@") {
		return fmt.Errorf("--mail must be an email address")
	}
	if v, ok := params["lat_dir"]; ok {
		dir := strings.ToUpper(v)
		if dir != "N" && dir != "S" {
			return fmt.Errorf("--lat-dir must be N or S")
		}
		params["lat_dir"] = dir
	}
	if v, ok := params["long_dir"]; ok {
		dir := strings.ToUpper(v)
		if dir != "E" && dir != "W" {
			return fmt.Errorf("--long-dir must be E or W")
		}
		params["long_dir"] = dir
	}
	return nil
}

func flagName(param string) string {
	if flag, ok := paramFlag[param]; ok {
		return flag
	}
	return param
}

// includeType selects add-record (true) or mod-record (false).
// mod-record does not send record-type or status.
func (s RecordSpec) query(includeType bool) (url.Values, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if !includeType && s.ID == "" {
		return nil, fmt.Errorf("record id is required")
	}
	q := url.Values{}
	q.Set("domain-name", s.Domain)
	q.Set("host", s.Host)
	q.Set("ttl", strconv.Itoa(s.TTL))
	if includeType {
		q.Set("record-type", s.Type)
	} else {
		q.Set("record-id", s.ID)
	}
	if s.Record != "" {
		q.Set("record", s.Record)
	}
	for name, value := range s.Params {
		if !includeType && name == "status" {
			continue
		}
		q.Set(name, value)
	}
	return q, nil
}
