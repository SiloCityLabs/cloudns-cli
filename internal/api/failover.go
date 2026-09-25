package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// FailoverUsage is how many DNS failover checks the account is using.
type FailoverUsage struct {
	Count string
	Limit string
}

// FailoverNode is one checker from GET /dns/get-failover-servers.json.
type FailoverNode struct {
	ID       string
	IP       string
	IP6      string
	Location string
}

func (n *FailoverNode) UnmarshalJSON(b []byte) error {
	var raw struct {
		ID       flexString `json:"id"`
		IP       string     `json:"ip"`
		IP6      string     `json:"ip6"`
		Location string     `json:"location"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*n = FailoverNode{ID: raw.ID.String(), IP: raw.IP, IP6: raw.IP6, Location: raw.Location}
	return nil
}

// FailoverCheck is a DNS failover configuration on one record.
// Type, Down, and Up accept the CLI names or the API integers.
type FailoverCheck struct {
	Domain             string
	RecordID           string
	Type               string
	Down               string
	Up                 string
	MainIP             string
	Backups            []string
	Region             string
	Host               string
	PingThreshold      string
	Protocol           string
	Port               string
	Path               string
	OpenPort           string
	Content            string
	QueryType          string
	QueryResponse      string
	Period             string
	Mail               string
	DeactivateRecord   string
	LatencyLimit       string
	Timeout            string
	RequestType        string
	Header             string
	HeaderValue        string
	ConnectionSecurity string
	// State is sent only on modify, and only when set. 1 is active, 0 is paused.
	State string
}

// FailoverUsage calls GET /dns/get-failover-stats.json.
func (c *Client) FailoverUsage(ctx context.Context) (FailoverUsage, json.RawMessage, error) {
	raw, err := c.Do(ctx, "/dns/get-failover-stats.json", nil)
	if err != nil {
		return FailoverUsage{}, nil, err
	}
	var body struct {
		Count flexString `json:"count"`
		Limit flexString `json:"limit"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return FailoverUsage{}, nil, fmt.Errorf("parse failover usage: %w", err)
	}
	return FailoverUsage{Count: body.Count.String(), Limit: body.Limit.String()}, raw, nil
}

// FailoverNodes calls GET /dns/get-failover-servers.json.
func (c *Client) FailoverNodes(ctx context.Context) ([]FailoverNode, json.RawMessage, error) {
	raw, err := c.Do(ctx, "/dns/get-failover-servers.json", nil)
	if err != nil {
		return nil, nil, err
	}
	var nodes []FailoverNode
	if err := json.Unmarshal(trimBody(raw), &nodes); err != nil {
		return nil, nil, fmt.Errorf("parse failover nodes: %w", err)
	}
	return nodes, raw, nil
}

// FailoverSettings calls GET /dns/failover-settings.json.
func (c *Client) FailoverSettings(ctx context.Context, domain, recordID string) (json.RawMessage, error) {
	q, err := failoverIdentity(domain, recordID)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, "/dns/failover-settings.json", q)
}

// ActivateFailover calls GET /dns/failover-activate.json.
func (c *Client) ActivateFailover(ctx context.Context, check FailoverCheck) (json.RawMessage, error) {
	q, err := check.query(false)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, "/dns/failover-activate.json", q)
}

// ModifyFailover calls GET /dns/failover-modify.json.
func (c *Client) ModifyFailover(ctx context.Context, check FailoverCheck) (json.RawMessage, error) {
	q, err := check.query(true)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, "/dns/failover-modify.json", q)
}

// DeactivateFailover calls GET /dns/failover-deactivate.json.
func (c *Client) DeactivateFailover(ctx context.Context, domain, recordID string) (json.RawMessage, error) {
	q, err := failoverIdentity(domain, recordID)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, "/dns/failover-deactivate.json", q)
}

func failoverIdentity(domain, recordID string) (url.Values, error) {
	domain = strings.TrimSpace(domain)
	recordID = strings.TrimSpace(recordID)
	if domain == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	if recordID == "" {
		return nil, fmt.Errorf("record id is required")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	q.Set("record-id", recordID)
	return q, nil
}

func (check FailoverCheck) query(modify bool) (url.Values, error) {
	q, err := failoverIdentity(check.Domain, check.RecordID)
	if err != nil {
		return nil, err
	}
	typ, err := failoverType(check.Type)
	if err != nil {
		return nil, err
	}
	down, err := failoverDown(check.Down)
	if err != nil {
		return nil, err
	}
	up, err := failoverUp(check.Up)
	if err != nil {
		return nil, err
	}
	mainIP := strings.TrimSpace(check.MainIP)
	if net.ParseIP(mainIP) == nil {
		return nil, fmt.Errorf("--main-ip must be an IP address")
	}
	backups, err := failoverBackups(check.Backups)
	if err != nil {
		return nil, err
	}
	region, err := failoverRegion(check.Region)
	if err != nil {
		return nil, err
	}
	protocol, err := oneOf(check.Protocol, "--protocol", "http", "https")
	if err != nil {
		return nil, err
	}
	period, err := oneOf(check.Period, "--period", "60", "120", "300", "600", "900", "1200", "1800", "3600")
	if err != nil {
		return nil, err
	}
	threshold, err := oneOf(check.PingThreshold, "--ping-threshold", "15", "25", "50", "100")
	if err != nil {
		return nil, err
	}
	openPort, err := oneOf(check.OpenPort, "--open-port", "0", "1")
	if err != nil {
		return nil, err
	}
	requestType, err := oneOf(strings.ToUpper(strings.TrimSpace(check.RequestType)), "--request-type", "GET", "HEAD", "POST", "PUT", "DELETE")
	if err != nil {
		return nil, err
	}
	security, err := oneOf(check.ConnectionSecurity, "--security", "1", "2", "3")
	if err != nil {
		return nil, err
	}
	deactivate, err := oneOf(check.DeactivateRecord, "--deactivate-record", "0", "1")
	if err != nil {
		return nil, err
	}
	port, err := failoverPort(check.Port)
	if err != nil {
		return nil, err
	}
	timeout, err := failoverTimeout(typ, check.Timeout)
	if err != nil {
		return nil, err
	}
	if err := failoverLatency(check.LatencyLimit); err != nil {
		return nil, err
	}
	header := strings.TrimSpace(check.Header)
	if header != "" && !failoverHeaderName(header) {
		return nil, fmt.Errorf("--header may contain only letters, dash, and underscore")
	}
	if err := failoverTypeFields(typ, check, port, openPort); err != nil {
		return nil, err
	}

	q.Set("check_type", typ)
	q.Set("down_event_handler", down)
	q.Set("up_event_handler", up)
	q.Set("main_ip", mainIP)
	q.Set("backup_ip_1", backups[0])
	for i := 1; i < len(backups); i++ {
		q.Set(fmt.Sprintf("backup_ip_%d", i+1), backups[i])
	}
	setOpt(q, "monitoring_region", region)
	setOpt(q, "host", check.Host)
	setOpt(q, "ping_threshold", threshold)
	setOpt(q, "http_protocol", protocol)
	setOpt(q, "port", port)
	setOpt(q, "path", check.Path)
	setOpt(q, "open_port", openPort)
	setOpt(q, "content", check.Content)
	setOpt(q, "query_type", check.QueryType)
	setOpt(q, "query_response", check.QueryResponse)
	setOpt(q, "check_period", period)
	setOpt(q, "notification_mail", check.Mail)
	setOpt(q, "deactivate_record", deactivate)
	setOpt(q, "latency_limit", strings.TrimSpace(check.LatencyLimit))
	setOpt(q, "timeout", timeout)
	setOpt(q, "http_request_type", requestType)
	setOpt(q, "custom_header", header)
	setOpt(q, "custom_header_value", check.HeaderValue)
	setOpt(q, "connection_security", security)
	if modify {
		state, err := oneOf(check.State, "--state", "0", "1")
		if err != nil {
			return nil, err
		}
		setOpt(q, "state", state)
	} else if strings.TrimSpace(check.State) != "" {
		return nil, fmt.Errorf("--state is only used when editing a failover check")
	}
	return q, nil
}

func setOpt(q url.Values, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		q.Set(key, value)
	}
}

func failoverType(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ping", "17":
		return "17", nil
	case "web", "18":
		return "18", nil
	case "tcp", "8":
		return "8", nil
	case "udp", "9":
		return "9", nil
	case "dns", "10":
		return "10", nil
	case "smtp", "15":
		return "15", nil
	default:
		return "", fmt.Errorf("check type must be ping, web, tcp, udp, dns, or smtp")
	}
}

func failoverDown(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "monitor", "0":
		return "0", nil
	case "deactivate", "1":
		return "1", nil
	case "replace", "2":
		return "2", nil
	default:
		return "", fmt.Errorf("--down must be monitor, deactivate, or replace")
	}
}

func failoverUp(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "monitor", "0":
		return "0", nil
	case "activate", "1":
		return "1", nil
	case "ignore", "2":
		return "2", nil
	default:
		return "", fmt.Errorf("--up must be monitor, activate, or ignore")
	}
}

func failoverBackups(ips []string) ([]string, error) {
	var out []string
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return nil, fmt.Errorf("--backup-ip is empty")
		}
		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("--backup-ip must be an IP address")
		}
		out = append(out, ip)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--backup-ip is required")
	}
	if len(out) > 5 {
		return nil, fmt.Errorf("at most 5 --backup-ip values")
	}
	return out, nil
}

func failoverRegion(region string) (string, error) {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		return "", nil
	}
	if _, err := strconv.Atoi(region); err == nil {
		return region, nil
	}
	switch region {
	case "global", "eur", "nam", "asi", "at", "bg", "br", "ca", "de", "es", "fi", "hk", "hu", "il", "in", "it", "jp", "kr", "mx", "nl", "pl", "ro", "ru", "sg", "tr", "tw", "uk", "us", "za":
		return region, nil
	default:
		return "", fmt.Errorf("--region must be a region code or a node id from zone failover nodes")
	}
}

func oneOf(value, flag string, allowed ...string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	for _, item := range allowed {
		if strings.EqualFold(value, item) {
			return item, nil
		}
	}
	return "", fmt.Errorf("%s must be %s", flag, strings.Join(allowed, ", "))
}

func failoverPort(port string) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		return "", nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("--port must be from 1 to 65535")
	}
	return strconv.Itoa(n), nil
}

func failoverTimeout(checkType, timeout string) (string, error) {
	timeout = strings.TrimSpace(timeout)
	if timeout == "" {
		return "", nil
	}
	n, err := strconv.Atoi(timeout)
	if err != nil {
		return "", fmt.Errorf("--timeout must be a number of seconds")
	}
	switch checkType {
	case "17":
		if n < 1 || n > 5 {
			return "", fmt.Errorf("--timeout for ping must be from 1 to 5")
		}
	case "18":
		if n < 4 || n > 10 {
			return "", fmt.Errorf("--timeout for web must be from 4 to 10")
		}
	}
	return strconv.Itoa(n), nil
}

func failoverLatency(limit string) error {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return nil
	}
	if _, err := strconv.ParseFloat(limit, 64); err != nil {
		return fmt.Errorf("--latency-limit must be a number")
	}
	return nil
}

func failoverHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func failoverTypeFields(checkType string, check FailoverCheck, port, openPort string) error {
	host := strings.TrimSpace(check.Host)
	switch checkType {
	case "18":
		if host == "" {
			return fmt.Errorf("web checks require --host")
		}
		if port == "" {
			return fmt.Errorf("web checks require --port")
		}
	case "10":
		if host == "" {
			return fmt.Errorf("dns checks require --host")
		}
		if strings.TrimSpace(check.QueryType) == "" || strings.TrimSpace(check.QueryResponse) == "" {
			return fmt.Errorf("dns checks require --query-type and --query-response")
		}
	case "8", "9":
		if port == "" {
			return fmt.Errorf("tcp and udp checks require --port")
		}
		if openPort == "" {
			return fmt.Errorf("tcp and udp checks require --open-port")
		}
	}
	return nil
}
