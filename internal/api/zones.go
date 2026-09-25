package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
)

// ListZones pages through GET /dns/list-zones.json until a short page or an empty page.
// The raw payload is the combined JSON array of zone objects.
func (c *Client) ListZones(ctx context.Context) ([]Zone, json.RawMessage, error) {
	size := c.pageSize()
	var zones []Zone
	var raws []json.RawMessage
	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("rows-per-page", strconv.Itoa(size))
		body, err := c.Do(ctx, "/dns/list-zones.json", q)
		if err != nil {
			return nil, nil, err
		}
		pageZones, pageRaw, paginate, err := decodeZones(body)
		if err != nil {
			return nil, nil, err
		}
		if len(pageZones) == 0 {
			break
		}
		zones = append(zones, pageZones...)
		raws = append(raws, pageRaw...)
		if !paginate || len(pageZones) < size {
			break
		}
		if page == maxPages {
			return nil, nil, fmt.Errorf("zone list did not end after %d pages", maxPages)
		}
	}
	if raws == nil {
		raws = []json.RawMessage{}
	}
	combined, err := json.Marshal(raws)
	if err != nil {
		return nil, nil, err
	}
	return zones, combined, nil
}

func decodeZones(body json.RawMessage) (zones []Zone, raws []json.RawMessage, paginate bool, err error) {
	trim := trimBody(body)
	if len(trim) == 0 || string(trim) == "null" || string(trim) == "[]" || string(trim) == "{}" {
		return nil, nil, true, nil
	}
	switch trim[0] {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trim, &items); err != nil {
			return nil, nil, false, fmt.Errorf("parse zone list: %w", err)
		}
		zones = make([]Zone, 0, len(items))
		for _, item := range items {
			var z Zone
			if err := json.Unmarshal(item, &z); err != nil {
				return nil, nil, false, fmt.Errorf("parse zone: %w", err)
			}
			zones = append(zones, z)
		}
		return zones, items, true, nil
	case '{':
		// Some responses are objects keyed by zone name. Do not paginate that shape:
		// a full page would otherwise be requested again forever.
		var items map[string]json.RawMessage
		if err := json.Unmarshal(trim, &items); err != nil {
			return nil, nil, false, fmt.Errorf("parse zone list: %w", err)
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
			val := trimBody(items[k])
			if len(val) == 0 || val[0] != '{' {
				return nil, nil, false, fmt.Errorf("unexpected zone list payload")
			}
			var z Zone
			if err := json.Unmarshal(val, &z); err != nil {
				return nil, nil, false, fmt.Errorf("parse zone: %w", err)
			}
			if z.Name == "" {
				z.Name = k
			}
			zones = append(zones, z)
			raws = append(raws, items[k])
		}
		return zones, raws, false, nil
	default:
		return nil, nil, false, fmt.Errorf("unexpected zone list payload")
	}
}
