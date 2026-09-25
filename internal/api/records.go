package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// AllowedTTLs are the TTL values ClouDNS accepts, in seconds.
var AllowedTTLs = []int{
	60, 300, 900, 1800, 3600, 21600, 43200, 86400,
	172800, 259200, 604800, 1209600, 2592000,
}

// ValidateTTL reports whether ttl is one of AllowedTTLs.
func ValidateTTL(ttl int) error {
	for _, allowed := range AllowedTTLs {
		if ttl == allowed {
			return nil
		}
	}
	parts := make([]string, len(AllowedTTLs))
	for i, n := range AllowedTTLs {
		parts[i] = strconv.Itoa(n)
	}
	return fmt.Errorf("invalid ttl %d; allowed values: %s", ttl, strings.Join(parts, ", "))
}

// ListRecords calls GET /dns/records.json.
func (c *Client) ListRecords(ctx context.Context, domain, typ, host string) ([]Record, json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, nil, fmt.Errorf("domain name is required")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	if typ = strings.TrimSpace(typ); typ != "" {
		q.Set("type", typ)
	}
	if host = strings.TrimSpace(host); host != "" {
		q.Set("host", host)
	}
	body, err := c.Do(ctx, "/dns/records.json", q)
	if err != nil {
		return nil, nil, err
	}
	recs, err := decodeRecords(body)
	if err != nil {
		return nil, nil, err
	}
	return filterRecords(recs, typ, host), body, nil
}

// AddRecord calls GET /dns/add-record.json.
// The record type is sent as record-type. The apex host is @.
func (c *Client) AddRecord(ctx context.Context, spec RecordSpec) (json.RawMessage, error) {
	q, err := spec.query(true)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, "/dns/add-record.json", q)
}

// ModifyRecord calls GET /dns/mod-record.json for an existing record id.
// The type is validated locally and is not sent. ClouDNS cannot change a record's type.
func (c *Client) ModifyRecord(ctx context.Context, spec RecordSpec) (json.RawMessage, error) {
	q, err := spec.query(false)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, "/dns/mod-record.json", q)
}

// UpsertRecord updates the single record with the same type and host, or adds one.
func (c *Client) UpsertRecord(ctx context.Context, spec RecordSpec) (json.RawMessage, error) {
	if _, err := spec.query(true); err != nil {
		return nil, err
	}
	existing, _, err := c.ListRecords(ctx, spec.Domain, spec.Type, spec.Host)
	if err != nil {
		return nil, err
	}
	switch len(existing) {
	case 0:
		return c.AddRecord(ctx, spec)
	case 1:
		spec.ID = existing[0].ID
		return c.ModifyRecord(ctx, spec)
	default:
		return nil, fmt.Errorf("found %d %s records for host %s; delete the extras before --upsert", len(existing), strings.ToUpper(strings.TrimSpace(spec.Type)), spec.Host)
	}
}

// DeleteRecord calls GET /dns/delete-record.json.
func (c *Client) DeleteRecord(ctx context.Context, domain, id string) (json.RawMessage, error) {
	domain = strings.TrimSpace(domain)
	id = strings.TrimSpace(id)
	if domain == "" {
		return nil, fmt.Errorf("domain name is required")
	}
	if id == "" {
		return nil, fmt.Errorf("record id is required")
	}
	q := url.Values{}
	q.Set("domain-name", domain)
	q.Set("record-id", id)
	return c.Do(ctx, "/dns/delete-record.json", q)
}

func decodeRecords(body json.RawMessage) ([]Record, error) {
	trim := trimBody(body)
	if len(trim) == 0 || string(trim) == "null" || string(trim) == "[]" || string(trim) == "{}" {
		return nil, nil
	}
	if trim[0] == '[' {
		var recs []Record
		if err := json.Unmarshal(trim, &recs); err != nil {
			return nil, fmt.Errorf("parse records: %w", err)
		}
		sortRecords(recs)
		return recs, nil
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(trim, &asMap); err != nil {
		return nil, fmt.Errorf("parse records: %w", err)
	}
	recs := make([]Record, 0, len(asMap))
	for id, raw := range asMap {
		val := trimBody(raw)
		if len(val) == 0 || val[0] != '{' {
			return nil, fmt.Errorf("parse records: unexpected value for %s", id)
		}
		var rec Record
		if err := json.Unmarshal(val, &rec); err != nil {
			return nil, fmt.Errorf("parse records: %w", err)
		}
		if rec.ID == "" {
			rec.ID = id
		}
		recs = append(recs, rec)
	}
	sortRecords(recs)
	return recs, nil
}

func sortRecords(recs []Record) {
	sort.Slice(recs, func(i, j int) bool {
		ai, aerr := strconv.Atoi(recs[i].ID)
		bi, berr := strconv.Atoi(recs[j].ID)
		if aerr == nil && berr == nil && ai != bi {
			return ai < bi
		}
		return recs[i].ID < recs[j].ID
	})
}

func filterRecords(recs []Record, typ, host string) []Record {
	if typ == "" && host == "" {
		return recs
	}
	out := make([]Record, 0, len(recs))
	for _, rec := range recs {
		if typ != "" && !strings.EqualFold(rec.Type, typ) {
			continue
		}
		if host != "" && !sameHost(rec.Host, host) {
			continue
		}
		out = append(out, rec)
	}
	return out
}

func sameHost(a, b string) bool {
	return normHost(a) == normHost(b)
}

func normHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "@"
	}
	return s
}
