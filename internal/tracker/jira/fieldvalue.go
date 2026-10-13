package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Generic field values: any custom field, by schema type, so
// teams can add fields without code changes.

// Kind is how Maestro displays and edits a field.
type Kind string

const (
	KindText     Kind = "text"     // string (textfield)
	KindURL      Kind = "url"      // string (url)
	KindMarkdown Kind = "markdown" // string (textarea): ADF on Cloud, wiki on DC
	KindNumber   Kind = "number"
	KindDate     Kind = "date"     // YYYY-MM-DD
	KindDateTime Kind = "datetime" // RFC 3339
	KindOption   Kind = "option"   // single select / radio
	KindOptions  Kind = "options"  // multi select / checkboxes
	KindCascade  Kind = "cascade"  // "Parent › Child"
	KindUser     Kind = "user"
	KindUsers    Kind = "users"
	KindVersion  Kind = "version"
	KindVersions Kind = "versions"
	KindLabels   Kind = "labels"
	KindGroup    Kind = "group"  // read only in v1
	KindGroups   Kind = "groups" // read only in v1
	KindTeam     Kind = "team"   // read only in v1
	KindUnknown  Kind = "unknown"
)

// Editable reports whether Maestro can write the kind in v1.
func (k Kind) Editable() bool {
	switch k {
	case KindGroup, KindGroups, KindTeam, KindUnknown:
		return false
	}
	return true
}

// KindOf maps a field schema to a Kind.
func KindOf(s FieldSchema) Kind {
	switch s.Type {
	case "string":
		switch s.Custom {
		case schemaTextarea:
			return KindMarkdown
		case schemaURL:
			return KindURL
		}
		if s.System == "description" || s.System == "environment" {
			return KindMarkdown
		}
		return KindText
	case "number":
		return KindNumber
	case "date":
		return KindDate
	case "datetime":
		return KindDateTime
	case "option":
		return KindOption
	case "option-with-child":
		return KindCascade
	case "user":
		return KindUser
	case "group":
		return KindGroup
	case "version":
		return KindVersion
	case "team":
		return KindTeam
	case "array":
		switch s.Items {
		case "option":
			return KindOptions
		case "user":
			return KindUsers
		case "version":
			return KindVersions
		case "string":
			return KindLabels
		case "group":
			return KindGroups
		case "component":
			return KindOptions
		}
	}
	if s.Custom == SchemaTeam {
		return KindTeam
	}
	return KindUnknown
}

// Value is a decoded field value, ready to display.
type Value struct {
	Kind  Kind     `json:"kind"`
	Text  string   `json:"text,omitempty"`  // single value, or markdown, or a date
	Items []string `json:"items,omitempty"` // multi values (options, labels, versions, users' names)
	Users []User   `json:"users,omitempty"` // for user kinds
	Empty bool     `json:"empty"`
}

// DecodeValue reads a raw Jira field value. It never fails: unknown shapes
// become text, so no value is ever lost from the view.
func DecodeValue(s FieldSchema, raw json.RawMessage) Value {
	k := KindOf(s)
	v := Value{Kind: k}
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" || string(raw) == `""` {
		v.Empty = true
		return v
	}
	switch k {
	case KindText, KindURL:
		v.Text = asString(raw)
	case KindMarkdown:
		v.Text = RichTextToMarkdown(raw)
	case KindNumber:
		var f float64
		if json.Unmarshal(raw, &f) == nil {
			v.Text = strconv.FormatFloat(f, 'f', -1, 64)
		}
	case KindDate:
		v.Text = asString(raw)
	case KindDateTime:
		v.Text = normalizeTime(asString(raw))
	case KindOption, KindVersion, KindGroup, KindTeam:
		v.Text = nameOf(raw)
	case KindCascade:
		var c struct {
			Value string `json:"value"`
			Child *struct {
				Value string `json:"value"`
			} `json:"child"`
		}
		if json.Unmarshal(raw, &c) == nil {
			v.Text = c.Value
			if c.Child != nil && c.Child.Value != "" {
				v.Text += " › " + c.Child.Value
			}
		}
	case KindOptions, KindVersions, KindGroups:
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil {
			for _, a := range arr {
				if n := nameOf(a); n != "" {
					v.Items = append(v.Items, n)
				}
			}
		}
	case KindLabels:
		_ = json.Unmarshal(raw, &v.Items)
	case KindUser:
		var u User
		if json.Unmarshal(raw, &u) == nil {
			v.Users, v.Text = []User{u}, u.DisplayName
		}
	case KindUsers:
		if json.Unmarshal(raw, &v.Users) == nil {
			for _, u := range v.Users {
				v.Items = append(v.Items, u.DisplayName)
			}
		}
	default:
		v.Text = fallbackText(raw)
	}
	v.Empty = v.Text == "" && len(v.Items) == 0
	return v
}

// EncodeValue turns user input (one string, or several for multi kinds) into
// the JSON Jira expects for create and edit. Empty input clears the field.
func EncodeValue(s FieldSchema, input []string, dep Deployment) (any, error) {
	k := KindOf(s)
	if !k.Editable() {
		return nil, fmt.Errorf("fields of type %q cannot be edited in Maestro yet", k)
	}
	var vals []string
	for _, in := range input {
		if t := strings.TrimSpace(in); t != "" {
			vals = append(vals, t)
		}
	}
	one := ""
	if len(vals) > 0 {
		one = vals[0]
	}
	if len(vals) == 0 {
		switch k {
		case KindOptions, KindUsers, KindVersions, KindLabels:
			return []any{}, nil
		}
		return nil, nil
	}

	switch k {
	case KindText, KindURL:
		return one, nil
	case KindMarkdown:
		if dep == DataCenter {
			return one, nil
		}
		return FromMarkdown(one), nil
	case KindNumber:
		f, err := strconv.ParseFloat(one, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", one)
		}
		return f, nil
	case KindDate:
		if _, err := time.Parse(time.DateOnly, one); err != nil {
			return nil, fmt.Errorf("%q is not a date (YYYY-MM-DD)", one)
		}
		return one, nil
	case KindDateTime:
		t, err := time.Parse(time.RFC3339, one)
		if err != nil {
			return nil, fmt.Errorf("%q is not a date and time", one)
		}
		return t.Format("2006-01-02T15:04:05.000-0700"), nil
	case KindOption:
		return map[string]string{"value": one}, nil
	case KindOptions:
		out := make([]map[string]string, len(vals))
		for i, v := range vals {
			out[i] = map[string]string{"value": v}
		}
		if s.Items == "component" {
			for i, v := range vals {
				out[i] = map[string]string{"name": v}
			}
		}
		return out, nil
	case KindCascade:
		parent, child, _ := strings.Cut(one, " › ")
		if len(vals) == 2 {
			parent, child = vals[0], vals[1]
		}
		c := map[string]any{"value": strings.TrimSpace(parent)}
		if child = strings.TrimSpace(child); child != "" {
			c["child"] = map[string]string{"value": child}
		}
		return c, nil
	case KindUser:
		return userRefFor(one, dep), nil
	case KindUsers:
		out := make([]map[string]any, len(vals))
		for i, v := range vals {
			out[i] = userRefFor(v, dep)
		}
		return out, nil
	case KindVersion:
		return map[string]string{"name": one}, nil
	case KindVersions:
		out := make([]map[string]string, len(vals))
		for i, v := range vals {
			out[i] = map[string]string{"name": v}
		}
		return out, nil
	case KindLabels:
		for _, v := range vals {
			if strings.ContainsAny(v, " \t") {
				return nil, errors.New("labels cannot contain spaces")
			}
		}
		return vals, nil
	}
	return nil, fmt.Errorf("unsupported field type %q", k)
}

// AllowedValues lists the option names from createmeta/editmeta allowedValues.
func AllowedValues(raw json.RawMessage) []string {
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	var out []string
	for _, a := range arr {
		var o struct {
			Value    string `json:"value"`
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
			Archived bool   `json:"archived"`
			Released bool   `json:"released"`
			Children []struct {
				Value string `json:"value"`
			} `json:"children"`
		}
		if json.Unmarshal(a, &o) != nil || o.Disabled || o.Archived {
			continue
		}
		n := o.Value
		if n == "" {
			n = o.Name
		}
		if len(o.Children) > 0 {
			for _, c := range o.Children {
				out = append(out, n+" › "+c.Value)
			}
			continue
		}
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func userRefFor(id string, dep Deployment) map[string]any {
	if dep == DataCenter {
		return map[string]any{"name": id}
	}
	return map[string]any{"accountId": id}
}

func asString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return fallbackText(raw)
}

// nameOf reads the display name of an object value (option, version, group, team).
func nameOf(raw json.RawMessage) string {
	var o map[string]any
	if json.Unmarshal(raw, &o) != nil {
		return asString(raw)
	}
	for _, k := range []string{"value", "name", "displayName", "title"} {
		if s, ok := o[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func fallbackText(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case map[string]any:
		return nameOf(raw)
	case []any:
		var parts []string
		for _, e := range t {
			b, _ := json.Marshal(e)
			if s := fallbackText(b); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func normalizeTime(s string) string {
	for _, layout := range []string{"2006-01-02T15:04:05.000-0700", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}
