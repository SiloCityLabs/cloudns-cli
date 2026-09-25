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

// RegisteredDomain is one domain registered at ClouDNS, not a DNS zone.
type RegisteredDomain struct {
	Name       string
	Status     string
	Expires    string
	Registered string
}

func (d *RegisteredDomain) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	d.Name = firstString(raw, "name", "domain", "domain-name")
	d.Status = firstString(raw, "status")
	d.Expires = firstString(raw, "expires_on", "expire", "expiration", "expire_on", "valid_till", "expiration_date")
	d.Registered = firstString(raw, "registered_on", "registered", "registration")
	return nil
}

// SortDomains sorts a domain list. expires and registered put the earliest
// date first, and domains with no date last. name is alphabetical.
func SortDomains(domains []RegisteredDomain, by string) error {
	less, err := domainLess(by)
	if err != nil || less == nil {
		return err
	}
	sort.SliceStable(domains, func(i, j int) bool {
		return less(domains[i], domains[j])
	})
	return nil
}

// SortDomainPayload sorts domains and the matching JSON array together.
func SortDomainPayload(domains []RegisteredDomain, raw json.RawMessage, by string) ([]RegisteredDomain, json.RawMessage, error) {
	less, err := domainLess(by)
	if err != nil || less == nil {
		return domains, raw, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) != len(domains) {
		if err := SortDomains(domains, by); err != nil {
			return nil, nil, err
		}
		out, err := json.Marshal(domains)
		return domains, out, err
	}
	type row struct {
		domain RegisteredDomain
		raw    json.RawMessage
	}
	rows := make([]row, len(domains))
	for i := range domains {
		rows[i] = row{domain: domains[i], raw: items[i]}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return less(rows[i].domain, rows[j].domain)
	})
	sorted := make([]RegisteredDomain, len(rows))
	raws := make([]json.RawMessage, len(rows))
	for i := range rows {
		sorted[i] = rows[i].domain
		raws[i] = rows[i].raw
	}
	out, err := json.Marshal(raws)
	return sorted, out, err
}

func domainLess(by string) (func(a, b RegisteredDomain) bool, error) {
	by = strings.ToLower(strings.TrimSpace(by))
	if by == "" {
		return nil, nil
	}
	var key func(RegisteredDomain) string
	dates := false
	switch by {
	case "name":
		key = func(d RegisteredDomain) string { return strings.ToLower(d.Name) }
	case "expires", "expire", "expiry":
		dates = true
		key = func(d RegisteredDomain) string { return d.Expires }
	case "registered":
		dates = true
		key = func(d RegisteredDomain) string { return d.Registered }
	default:
		return nil, fmt.Errorf("invalid sort %q; use name, expires, or registered", by)
	}
	return func(a, b RegisteredDomain) bool {
		left, right := key(a), key(b)
		if dates {
			if left == "" {
				return false
			}
			if right == "" {
				return true
			}
		}
		return left < right
	}, nil
}

// ListDomains pages through GET /domains/list-domains.json.
// The raw payload is the combined JSON array of domain objects.
func (c *Client) ListDomains(ctx context.Context) ([]RegisteredDomain, json.RawMessage, error) {
	size := c.pageSize()
	var domains []RegisteredDomain
	var raws []json.RawMessage
	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("rows-per-page", strconv.Itoa(size))
		body, err := c.Do(ctx, "/domains/list-domains.json", q)
		if err != nil {
			return nil, nil, err
		}
		pageDomains, pageRaw, paginate, err := decodeDomains(body)
		if err != nil {
			return nil, nil, err
		}
		if len(pageDomains) == 0 {
			break
		}
		domains = append(domains, pageDomains...)
		raws = append(raws, pageRaw...)
		if !paginate || len(pageDomains) < size {
			break
		}
		if page == maxPages {
			return nil, nil, fmt.Errorf("domain list did not end after %d pages", maxPages)
		}
	}
	if raws == nil {
		raws = []json.RawMessage{}
	}
	combined, err := json.Marshal(raws)
	if err != nil {
		return nil, nil, err
	}
	return domains, combined, nil
}

func decodeDomains(body json.RawMessage) (domains []RegisteredDomain, raws []json.RawMessage, paginate bool, err error) {
	trim := trimBody(body)
	if len(trim) == 0 || string(trim) == "null" || string(trim) == "[]" || string(trim) == "{}" {
		return nil, nil, true, nil
	}
	switch trim[0] {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trim, &items); err != nil {
			return nil, nil, false, fmt.Errorf("parse domain list: %w", err)
		}
		domains = make([]RegisteredDomain, 0, len(items))
		for _, item := range items {
			domain, raw, err := oneDomain(item, "")
			if err != nil {
				return nil, nil, false, err
			}
			domains = append(domains, domain)
			raws = append(raws, raw)
		}
		return domains, raws, true, nil
	case '{':
		var items map[string]json.RawMessage
		if err := json.Unmarshal(trim, &items); err != nil {
			return nil, nil, false, fmt.Errorf("parse domain list: %w", err)
		}
		keys := make([]string, 0, len(items))
		for k := range items {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "status" || k == "statusDescription" {
				continue
			}
			domain, raw, err := oneDomain(items[k], k)
			if err != nil {
				return nil, nil, false, err
			}
			domains = append(domains, domain)
			raws = append(raws, raw)
		}
		return domains, raws, false, nil
	default:
		return nil, nil, false, fmt.Errorf("unexpected domain list payload")
	}
}

func oneDomain(item json.RawMessage, fallbackName string) (RegisteredDomain, json.RawMessage, error) {
	val := trimBody(item)
	if len(val) > 0 && val[0] == '"' {
		name, err := jsonString(val)
		if err != nil {
			return RegisteredDomain{}, nil, fmt.Errorf("parse domain: %w", err)
		}
		return RegisteredDomain{Name: name}, item, nil
	}
	var domain RegisteredDomain
	if err := json.Unmarshal(val, &domain); err != nil {
		return RegisteredDomain{}, nil, fmt.Errorf("parse domain: %w", err)
	}
	if domain.Name == "" {
		domain.Name = fallbackName
	}
	return domain, item, nil
}

func firstString(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		text, err := jsonString(value)
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" && text != "null" {
			return text
		}
	}
	return ""
}
