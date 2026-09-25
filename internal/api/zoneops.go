package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// RegisterZone calls GET /dns/register.json.
// zoneType is master, slave, parked, or geodns. Slave zones require masterIP.
// nameservers, when set, replace the default NS records.
func (c *Client) RegisterZone(ctx context.Context, domain, zoneType, masterIP string, nameservers []string) (json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	zoneType = strings.ToLower(strings.TrimSpace(zoneType))
	masterIP = strings.TrimSpace(masterIP)
	if domain == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	switch zoneType {
	case "master", "slave", "parked", "geodns":
	case "":
		return nil, fmt.Errorf("zone type is required")
	default:
		return nil, fmt.Errorf("zone type must be master, slave, parked, or geodns")
	}
	if zoneType == "slave" && masterIP == "" {
		return nil, fmt.Errorf("slave zones require --master-ip")
	}
	if zoneType != "slave" && masterIP != "" {
		return nil, fmt.Errorf("--master-ip is only used for slave zones")
	}
	if masterIP != "" && net.ParseIP(masterIP) == nil {
		return nil, fmt.Errorf("--master-ip must be an IP address")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	q.Set("zone-type", zoneType)
	if masterIP != "" {
		q.Set("master-ip", masterIP)
	}
	for _, ns := range nameservers {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			return nil, fmt.Errorf("nameserver hostname is empty")
		}
		q.Add("ns[]", ns)
	}
	return c.Do(ctx, "/dns/register.json", q)
}

// DeleteZone calls GET /dns/delete.json.
func (c *Client) DeleteZone(ctx context.Context, domain string) (json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	return c.Do(ctx, "/dns/delete.json", q)
}

// GetZoneInfo calls GET /dns/get-zone-info.json.
func (c *Client) GetZoneInfo(ctx context.Context, domain string) (map[string]string, json.RawMessage, error) {
	body, err := c.zoneCall(ctx, "/dns/get-zone-info.json", domain)
	if err != nil {
		return nil, nil, err
	}
	info, err := jsonObjectStrings(body)
	if err != nil {
		return nil, nil, fmt.Errorf("parse zone info: %w", err)
	}
	return info, body, nil
}

// SOA is the start of authority for a master zone.
type SOA struct {
	PrimaryNS  string
	AdminMail  string
	Refresh    string
	Retry      string
	Expire     string
	DefaultTTL string
}

// SOAPatch replaces the SOA fields that are non-nil. Modify SOA requires every field,
// so the caller loads the current SOA and overlays this patch.
type SOAPatch struct {
	PrimaryNS  *string
	AdminMail  *string
	Refresh    *string
	Retry      *string
	Expire     *string
	DefaultTTL *string
}

func (p SOAPatch) empty() bool {
	return p.PrimaryNS == nil && p.AdminMail == nil && p.Refresh == nil && p.Retry == nil && p.Expire == nil && p.DefaultTTL == nil
}

// GetSOA calls GET /dns/soa-details.json.
func (c *Client) GetSOA(ctx context.Context, domain string) (SOA, json.RawMessage, error) {
	body, err := c.zoneCall(ctx, "/dns/soa-details.json", domain)
	if err != nil {
		return SOA{}, nil, err
	}
	soa, err := parseSOA(body)
	if err != nil {
		return SOA{}, nil, err
	}
	return soa, body, nil
}

// SetSOA loads the current SOA, overlays patch, and calls GET /dns/modify-soa.json.
func (c *Client) SetSOA(ctx context.Context, domain string, patch SOAPatch) (json.RawMessage, error) {
	if patch.empty() {
		return nil, fmt.Errorf("set at least one of --primary-ns, --admin-mail, --refresh, --retry, --expire, or --default-ttl")
	}
	current, _, err := c.GetSOA(ctx, domain)
	if err != nil {
		return nil, err
	}
	merged := current
	if patch.PrimaryNS != nil {
		merged.PrimaryNS = strings.TrimSpace(*patch.PrimaryNS)
	}
	if patch.AdminMail != nil {
		merged.AdminMail = strings.TrimSpace(*patch.AdminMail)
	}
	if patch.Refresh != nil {
		merged.Refresh = strings.TrimSpace(*patch.Refresh)
	}
	if patch.Retry != nil {
		merged.Retry = strings.TrimSpace(*patch.Retry)
	}
	if patch.Expire != nil {
		merged.Expire = strings.TrimSpace(*patch.Expire)
	}
	if patch.DefaultTTL != nil {
		merged.DefaultTTL = strings.TrimSpace(*patch.DefaultTTL)
	}
	if err := merged.validate(); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("domain-name", strings.TrimSpace(domain))
	q.Set("primary-ns", merged.PrimaryNS)
	q.Set("admin-mail", merged.AdminMail)
	q.Set("refresh", merged.Refresh)
	q.Set("retry", merged.Retry)
	q.Set("expire", merged.Expire)
	q.Set("default-ttl", merged.DefaultTTL)
	return c.Do(ctx, "/dns/modify-soa.json", q)
}

func (s SOA) validate() error {
	if s.PrimaryNS == "" {
		return fmt.Errorf("SOA response is missing the primary nameserver")
	}
	if s.AdminMail == "" {
		return fmt.Errorf("SOA response is missing the admin mail")
	}
	checks := []struct {
		name  string
		value string
		min   int
		max   int
	}{
		{"--refresh", s.Refresh, 1200, 43200},
		{"--retry", s.Retry, 180, 2419200},
		{"--expire", s.Expire, 1209600, 2419200},
		{"--default-ttl", s.DefaultTTL, 60, 2419200},
	}
	for _, check := range checks {
		n, err := strconv.Atoi(check.value)
		if err != nil {
			return fmt.Errorf("%s must be an integer", check.name)
		}
		if n < check.min || n > check.max {
			return fmt.Errorf("%s must be from %d to %d seconds", check.name, check.min, check.max)
		}
	}
	return nil
}

func parseSOA(body []byte) (SOA, error) {
	fields, err := jsonObjectStrings(body)
	if err != nil {
		return SOA{}, fmt.Errorf("parse SOA: %w", err)
	}
	return SOA{
		PrimaryNS:  soaField(fields, "primaryns", "primary-ns"),
		AdminMail:  soaField(fields, "adminmail", "admin-mail"),
		Refresh:    soaField(fields, "refresh"),
		Retry:      soaField(fields, "retry"),
		Expire:     soaField(fields, "expire"),
		DefaultTTL: soaField(fields, "defaultttl", "default-ttl"),
	}, nil
}

func soaField(fields map[string]string, names ...string) string {
	for _, name := range names {
		if v := fields[normalizeKey(name)]; v != "" {
			return v
		}
	}
	return ""
}

// ExportZone calls GET /dns/records-export.json and returns the BIND zone file.
func (c *Client) ExportZone(ctx context.Context, domain string) (string, json.RawMessage, error) {
	body, err := c.zoneCall(ctx, "/dns/records-export.json", domain)
	if err != nil {
		return "", nil, err
	}
	var payload struct {
		Zone string `json:"zone"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil, fmt.Errorf("parse zone export: %w", err)
	}
	if strings.TrimSpace(payload.Zone) == "" {
		return "", nil, fmt.Errorf("zone export was empty")
	}
	return payload.Zone, body, nil
}

// ZoneUpdated calls GET /dns/is-updated.json.
// It is true when the zone is updated on every nameserver.
func (c *Client) ZoneUpdated(ctx context.Context, domain string) (bool, json.RawMessage, error) {
	body, err := c.zoneCall(ctx, "/dns/is-updated.json", domain)
	if err != nil {
		return false, nil, err
	}
	updated, err := parseUpdated(body)
	if err != nil {
		return false, nil, err
	}
	return updated, body, nil
}

func parseUpdated(body []byte) (bool, error) {
	var flag bool
	if err := json.Unmarshal(bytes.TrimSpace(body), &flag); err == nil {
		return flag, nil
	}
	var text string
	if err := json.Unmarshal(bytes.TrimSpace(body), &text); err == nil {
		body = []byte(text)
	}
	switch strings.ToLower(strings.Trim(strings.TrimSpace(string(body)), `"`)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected update status")
	}
}

// NameserverStatus is one server from GET /dns/update-status.json.
type NameserverStatus struct {
	Server  string
	IPv4    string
	IPv6    string
	Updated bool
}

// UpdateStatus calls GET /dns/update-status.json.
func (c *Client) UpdateStatus(ctx context.Context, domain string) ([]NameserverStatus, json.RawMessage, error) {
	body, err := c.zoneCall(ctx, "/dns/update-status.json", domain)
	if err != nil {
		return nil, nil, err
	}
	rows, err := parseUpdateStatus(body)
	if err != nil {
		return nil, nil, err
	}
	return rows, body, nil
}

func parseUpdateStatus(body []byte) ([]NameserverStatus, error) {
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 || string(trim) == "null" || string(trim) == "[]" || string(trim) == "{}" {
		return nil, nil
	}
	var items []json.RawMessage
	switch trim[0] {
	case '[':
		if err := json.Unmarshal(trim, &items); err != nil {
			return nil, fmt.Errorf("parse update status: %w", err)
		}
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trim, &obj); err != nil {
			return nil, fmt.Errorf("parse update status: %w", err)
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			items = append(items, obj[k])
		}
	default:
		return nil, fmt.Errorf("parse update status: unexpected response")
	}
	out := make([]NameserverStatus, 0, len(items))
	for _, item := range items {
		row, err := parseNameserverStatus(item)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Server < out[j].Server })
	return out, nil
}

func parseNameserverStatus(item json.RawMessage) (NameserverStatus, error) {
	var raw struct {
		Server  string     `json:"server"`
		Name    string     `json:"name"`
		IP4     flexString `json:"ip4"`
		IPv4    flexString `json:"ipv4"`
		IP6     flexString `json:"ip6"`
		IPv6    flexString `json:"ipv6"`
		Updated flexString `json:"updated"`
	}
	if err := json.Unmarshal(item, &raw); err != nil {
		return NameserverStatus{}, fmt.Errorf("parse update status: %w", err)
	}
	server := raw.Server
	if server == "" {
		server = raw.Name
	}
	updated, _ := parseUpdated([]byte(raw.Updated.String()))
	if strings.EqualFold(raw.Updated.String(), "true") || raw.Updated.String() == "1" {
		updated = true
	}
	ipv4 := raw.IP4.String()
	if ipv4 == "" {
		ipv4 = raw.IPv4.String()
	}
	ipv6 := raw.IP6.String()
	if ipv6 == "" {
		ipv6 = raw.IPv6.String()
	}
	return NameserverStatus{Server: server, IPv4: ipv4, IPv6: ipv6, Updated: updated}, nil
}

// MasterServer is one slave-zone master address.
type MasterServer struct {
	ID string
	IP string
}

// ListMasters calls GET /dns/master-servers.json.
func (c *Client) ListMasters(ctx context.Context, domain string) ([]MasterServer, json.RawMessage, error) {
	body, err := c.zoneCall(ctx, "/dns/master-servers.json", domain)
	if err != nil {
		return nil, nil, err
	}
	masters, err := parseMasters(body)
	if err != nil {
		return nil, nil, err
	}
	return masters, body, nil
}

// AddMaster calls GET /dns/add-master-server.json.
func (c *Client) AddMaster(ctx context.Context, domain, ip string) (json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	ip = strings.TrimSpace(ip)
	if domain == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	if net.ParseIP(ip) == nil {
		return nil, fmt.Errorf("master IP must be an IP address")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	q.Set("master-ip", ip)
	return c.Do(ctx, "/dns/add-master-server.json", q)
}

// DeleteMaster calls GET /dns/delete-master-server.json.
func (c *Client) DeleteMaster(ctx context.Context, domain, id string) (json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	id = strings.TrimSpace(id)
	if domain == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	if id == "" {
		return nil, fmt.Errorf("master id is required")
	}
	if _, err := strconv.Atoi(id); err != nil {
		return nil, fmt.Errorf("master id must be an integer")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	q.Set("master-id", id)
	return c.Do(ctx, "/dns/delete-master-server.json", q)
}

func parseMasters(body []byte) ([]MasterServer, error) {
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 || string(trim) == "null" || string(trim) == "[]" || string(trim) == "{}" {
		return nil, nil
	}
	if trim[0] == '[' {
		var ips []flexString
		if err := json.Unmarshal(trim, &ips); err != nil {
			return nil, fmt.Errorf("parse master servers: %w", err)
		}
		out := make([]MasterServer, 0, len(ips))
		for _, ip := range ips {
			if ip.String() == "" {
				continue
			}
			out = append(out, MasterServer{IP: ip.String()})
		}
		return out, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trim, &obj); err != nil {
		return nil, fmt.Errorf("parse master servers: %w", err)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, aerr := strconv.Atoi(keys[i])
		bi, berr := strconv.Atoi(keys[j])
		if aerr == nil && berr == nil && ai != bi {
			return ai < bi
		}
		return keys[i] < keys[j]
	})
	out := make([]MasterServer, 0, len(keys))
	for _, id := range keys {
		var ip flexString
		if err := json.Unmarshal(obj[id], &ip); err != nil {
			return nil, fmt.Errorf("parse master servers: %w", err)
		}
		if ip.String() == "" {
			continue
		}
		out = append(out, MasterServer{ID: id, IP: ip.String()})
	}
	return out, nil
}

func (c *Client) zoneCall(ctx context.Context, path, domain string) (json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	return c.Do(ctx, path, q)
}

func jsonObjectStrings(body []byte) (map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(body)))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		out[normalizeKey(key)] = jsonScalar(value)
	}
	return out, nil
}

func jsonScalar(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, "_", "")
	return key
}
