package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Glue TLDs accept an optional IPv4 after the hostname. ClouDNS sends that
// element as "hostname<space>ip" and replaces the whole delegation.
var glueTLDs = map[string]struct{}{
	"de": {},
	"be": {},
	"ch": {},
	"fr": {},
	"re": {},
	"tf": {},
	"wf": {},
	"yt": {},
	"sh": {},
	"eu": {},
}

const glueTLDText = ".de, .be, .ch, .fr, .re, .tf, .wf, .yt, .sh, .eu"

// GetNameservers calls GET /domains/get-nameservers.json.
// The payload may be a JSON array or an object with keys "1", "2", and so on.
// Empty strings are dropped.
func (c *Client) GetNameservers(ctx context.Context, domain string) ([]string, json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, nil, fmt.Errorf("domain name is required")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	body, err := c.Do(ctx, "/domains/get-nameservers.json", q)
	if err != nil {
		return nil, nil, err
	}
	servers, err := ParseNameservers(body)
	if err != nil {
		return nil, nil, err
	}
	return servers, body, nil
}

// SetNameservers replaces the registrar delegation with nameservers.
// Each value is one nameservers[] parameter. Glue addresses are already
// formatted as "hostname ip".
func (c *Client) SetNameservers(ctx context.Context, domain string, nameservers []string) (json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("domain name is required")
	}
	if len(nameservers) == 0 {
		return nil, fmt.Errorf("at least one nameserver is required")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	for _, ns := range nameservers {
		q.Add("nameservers[]", ns)
	}
	return c.Do(ctx, "/domains/set-nameservers.json", q)
}

// ParseNameserverArgs turns CLI arguments into nameservers[] values.
// On a glue TLD, an IPv4 may follow a hostname as the next argument or inside
// one argument separated by a space.
func ParseNameserverArgs(domain string, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("at least one nameserver is required")
	}
	glue := glueAllowed(domain)
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			return nil, fmt.Errorf("nameserver hostname is empty")
		}
		if strings.Contains(arg, " ") {
			host, ip, ok := splitGlueToken(arg)
			if !ok {
				return nil, fmt.Errorf("invalid nameserver %q", arg)
			}
			if !glue {
				return nil, fmt.Errorf("glue address is only valid for %s", glueTLDText)
			}
			if !isIPv4(ip) {
				return nil, fmt.Errorf("invalid glue IPv4 %q", ip)
			}
			out = append(out, host+" "+ip)
			continue
		}
		if isIPv4(arg) {
			return nil, fmt.Errorf("unexpected IPv4 %q without a nameserver hostname", arg)
		}
		if i+1 < len(args) && isIPv4(strings.TrimSpace(args[i+1])) {
			if !glue {
				return nil, fmt.Errorf("glue address is only valid for %s", glueTLDText)
			}
			out = append(out, arg+" "+strings.TrimSpace(args[i+1]))
			i++
			continue
		}
		out = append(out, arg)
	}
	return out, nil
}

// ParseNameservers accepts a JSON array or an object with numeric keys.
func ParseNameservers(raw json.RawMessage) ([]string, error) {
	raw = trimBody(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	switch raw[0] {
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("parse nameservers: %w", err)
		}
		out := make([]string, 0, len(arr))
		for _, el := range arr {
			s, err := jsonString(el)
			if err != nil {
				return nil, err
			}
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("parse nameservers: %w", err)
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			ni, ei := strconv.Atoi(keys[i])
			nj, ej := strconv.Atoi(keys[j])
			if ei == nil && ej == nil {
				return ni < nj
			}
			if ei == nil {
				return true
			}
			if ej == nil {
				return false
			}
			return keys[i] < keys[j]
		})
		out := make([]string, 0, len(keys))
		for _, k := range keys {
			s, err := jsonString(obj[k])
			if err != nil {
				return nil, err
			}
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected nameserver payload")
	}
}

func jsonString(raw json.RawMessage) (string, error) {
	raw = trimBody(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	if raw[0] == '{' || raw[0] == '[' {
		return "", fmt.Errorf("unexpected nameserver value")
	}
	return string(raw), nil
}

func glueAllowed(domain string) bool {
	_, ok := glueTLDs[publicSuffixLabel(domain)]
	return ok
}

func publicSuffixLabel(domain string) string {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if i := strings.LastIndex(domain, "."); i >= 0 && i < len(domain)-1 {
		return domain[i+1:]
	}
	return domain
}

func splitGlueToken(arg string) (host, ip string, ok bool) {
	host, ip, found := strings.Cut(arg, " ")
	if !found {
		return "", "", false
	}
	host = strings.TrimSpace(host)
	ip = strings.TrimSpace(ip)
	if host == "" || ip == "" || strings.Contains(ip, " ") {
		return "", "", false
	}
	return host, ip, true
}

func isIPv4(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	return ip != nil && ip.To4() != nil
}
