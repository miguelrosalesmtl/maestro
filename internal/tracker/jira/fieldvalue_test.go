package jira

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func sch(typ, items, custom string) FieldSchema {
	return FieldSchema{Type: typ, Items: items, Custom: custom}
}

func TestDecodeValue(t *testing.T) {
	cases := []struct {
		name   string
		schema FieldSchema
		raw    string
		want   Value
	}{
		{"text", sch("string", "", ""), `"Customer A"`, Value{Kind: KindText, Text: "Customer A"}},
		{"textarea ADF", sch("string", "", schemaTextarea), `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Given X"}]}]}`, Value{Kind: KindMarkdown, Text: "Given X"}},
		{"number", sch("number", "", SchemaFloat), `3.5`, Value{Kind: KindNumber, Text: "3.5"}},
		{"date", sch("date", "", SchemaDatePicker), `"2026-11-01"`, Value{Kind: KindDate, Text: "2026-11-01"}},
		{"datetime", sch("datetime", "", ""), `"2026-11-01T10:00:00.000-0400"`, Value{Kind: KindDateTime, Text: "2026-11-01T14:00:00Z"}},
		{"select", sch("option", "", ""), `{"id":"1","value":"Critical"}`, Value{Kind: KindOption, Text: "Critical"}},
		{"multiselect", sch("array", "option", ""), `[{"value":"iOS"},{"value":"Web"}]`, Value{Kind: KindOptions, Items: []string{"iOS", "Web"}}},
		{"flagged", sch("array", "option", SchemaMultiCheckboxes), `[{"value":"Impediment"}]`, Value{Kind: KindOptions, Items: []string{"Impediment"}}},
		{"cascade", sch("option-with-child", "", ""), `{"value":"EU","child":{"value":"Germany"}}`, Value{Kind: KindCascade, Text: "EU › Germany"}},
		{"user", sch("user", "", ""), `{"accountId":"a","displayName":"Ana"}`, Value{Kind: KindUser, Text: "Ana", Users: []User{{AccountID: "a", DisplayName: "Ana"}}}},
		{"versions", sch("array", "version", ""), `[{"id":"1","name":"2.4.0"}]`, Value{Kind: KindVersions, Items: []string{"2.4.0"}}},
		{"labels", sch("array", "string", ""), `["a","b"]`, Value{Kind: KindLabels, Items: []string{"a", "b"}}},
		{"team", sch("team", "", SchemaTeam), `{"id":"t1","name":"Platform"}`, Value{Kind: KindTeam, Text: "Platform"}},
		{"unknown app field", sch("any", "", "com.vendor:thing"), `{"title":"Blue"}`, Value{Kind: KindUnknown, Text: "Blue"}},
		{"empty", sch("option", "", ""), `null`, Value{Kind: KindOption, Empty: true}},
	}
	for _, c := range cases {
		got := DecodeValue(c.schema, json.RawMessage(c.raw))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestEncodeValue(t *testing.T) {
	enc := func(s FieldSchema, dep Deployment, in ...string) string {
		v, err := EncodeValue(s, in, dep)
		if err != nil {
			return "ERR " + err.Error()
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	cases := map[string][2]string{
		"text":        {enc(sch("string", "", ""), Cloud, "hi"), `"hi"`},
		"number":      {enc(sch("number", "", ""), Cloud, "2.5"), `2.5`},
		"bad number":  {enc(sch("number", "", ""), Cloud, "x"), `ERR "x" is not a number`},
		"date":        {enc(sch("date", "", ""), Cloud, "2026-11-01"), `"2026-11-01"`},
		"option":      {enc(sch("option", "", ""), Cloud, "High"), `{"value":"High"}`},
		"options":     {enc(sch("array", "option", ""), Cloud, "a", "b"), `[{"value":"a"},{"value":"b"}]`},
		"cascade":     {enc(sch("option-with-child", "", ""), Cloud, "EU › Germany"), `{"child":{"value":"Germany"},"value":"EU"}`},
		"user cloud":  {enc(sch("user", "", ""), Cloud, "abc"), `{"accountId":"abc"}`},
		"user dc":     {enc(sch("user", "", ""), DataCenter, "jdoe"), `{"name":"jdoe"}`},
		"versions":    {enc(sch("array", "version", ""), Cloud, "2.4.0"), `[{"name":"2.4.0"}]`},
		"clear list":  {enc(sch("array", "version", ""), Cloud), `[]`},
		"clear one":   {enc(sch("option", "", ""), Cloud, ""), `null`},
		"textarea dc": {enc(sch("string", "", schemaTextarea), DataCenter, "plain"), `"plain"`},
		"team ro":     {enc(sch("team", "", SchemaTeam), Cloud, "x"), `ERR fields of type "team" cannot be edited in Maestro yet`},
		"label space": {enc(sch("array", "string", ""), Cloud, "a b"), `ERR labels cannot contain spaces`},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s: got %s, want %s", name, c[0], c[1])
		}
	}
	if s := enc(sch("string", "", schemaTextarea), Cloud, "**x**"); !strings.Contains(s, `"type":"doc"`) {
		t.Errorf("textarea on Cloud must be ADF: %s", s)
	}
}

func TestAllowedValues(t *testing.T) {
	raw := `[{"value":"Low"},{"value":"Old","disabled":true},{"name":"2.4.0"},{"value":"EU","children":[{"value":"DE"},{"value":"FR"}]}]`
	got := AllowedValues(json.RawMessage(raw))
	if strings.Join(got, ",") != "Low,2.4.0,EU › DE,EU › FR" {
		t.Fatalf("got %v", got)
	}
}

func TestStandardFieldsBySchema(t *testing.T) {
	d := &Discovery{Fields: []Field{
		{ID: "customfield_1", Name: "Sprint", Schema: FieldSchema{Custom: SchemaSprint}},
		{ID: "customfield_2", Name: "Story Points", Schema: FieldSchema{Type: "number", Custom: SchemaFloat}},
		{ID: "customfield_3", Name: "Story point estimate", Schema: FieldSchema{Type: "number", Custom: SchemaStoryPointEst}},
		{ID: "customfield_4", Name: "Flagged", Schema: FieldSchema{Custom: SchemaMultiCheckboxes}},
		{ID: "customfield_5", Name: "Platforms", Schema: FieldSchema{Custom: SchemaMultiCheckboxes}},
		{ID: "customfield_6", Name: "Due by (team)", Schema: FieldSchema{Custom: SchemaDatePicker}},
		{ID: "customfield_7", Name: "Start date", Schema: FieldSchema{Custom: SchemaDatePicker}},
	}}
	s := d.Standard()
	if s.Sprint != "customfield_1" || s.StoryPoints != "customfield_3" || s.StoryPointsAlt != "customfield_2" ||
		s.Flagged != "customfield_4" || s.StartDate != "customfield_7" {
		t.Fatalf("catalog: %+v", s)
	}
	// Rule 1: the board's estimation field wins.
	d.Projects = []ProjectDiscovery{{Boards: []BoardDiscovery{{Config: &BoardConfig{
		Estimation: &BoardEstimation{Field: EstimationField{FieldID: "customfield_2"}}}}}}}
	if s := d.Standard(); s.StoryPoints != "customfield_2" || s.StoryPointsAlt != "customfield_3" {
		t.Fatalf("board estimation must win: %+v", s)
	}
}
