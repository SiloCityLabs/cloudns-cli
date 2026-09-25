package api

import (
	"bytes"
	"encoding/json"
)

// flexString accepts a JSON string or number.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(b)
	return nil
}

func (f flexString) String() string { return string(f) }

// Zone is one DNS zone from list-zones.
type Zone struct {
	Name   string
	Type   string
	Status string
}

func (z *Zone) UnmarshalJSON(b []byte) error {
	var raw struct {
		Name   string     `json:"name"`
		Type   string     `json:"type"`
		Status flexString `json:"status"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	z.Name = raw.Name
	z.Type = raw.Type
	z.Status = raw.Status.String()
	return nil
}

// Record is one DNS record.
type Record struct {
	ID    string
	Type  string
	Host  string
	Value string
	TTL   string
}

func (r *Record) UnmarshalJSON(b []byte) error {
	var raw struct {
		ID     flexString `json:"id"`
		Type   string     `json:"type"`
		Host   string     `json:"host"`
		Record string     `json:"record"`
		TTL    flexString `json:"ttl"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*r = Record{
		ID:    raw.ID.String(),
		Type:  raw.Type,
		Host:  raw.Host,
		Value: raw.Record,
		TTL:   raw.TTL.String(),
	}
	return nil
}
