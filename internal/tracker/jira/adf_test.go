package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarkdownRoundTrip(t *testing.T) {
	md := strings.TrimSpace(`
## Context
Traffic flips **100%** at once. Use ` + "`healthz`" + ` and see [the doc](https://example.com/doc).

## Acceptance criteria
- [x] Shift weights configurable
- [ ] Abort if error rate > 2%

Steps:

1. Deploy v2
2. Roll back

- plain *bullet*
- ~~old~~ item

> a quote

---

` + "```go\nfmt.Println(\"hi\")\n```")

	doc := FromMarkdown(md)
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var back ADFNode
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	got := ToMarkdown(&back)
	for _, want := range []string{
		"## Context", "**100%**", "`healthz`", "[the doc](https://example.com/doc)",
		"- [x] Shift weights configurable", "- [ ] Abort if error rate > 2%",
		"1. Deploy v2", "2. Roll back", "- plain *bullet*", "~~old~~", "> a quote", "---",
		"```go\nfmt.Println(\"hi\")\n```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("round trip lost %q\n--- got ---\n%s", want, got)
		}
	}
	// ADF requires localId on task lists and items.
	if !strings.Contains(string(b), `"type":"taskList","attrs":{"localId"`) {
		t.Errorf("taskList without localId: %s", b)
	}
}

func TestRichTextFromJira(t *testing.T) {
	if got := RichTextToMarkdown(json.RawMessage(`"h1. Data Center text"`)); got != "h1. Data Center text" {
		t.Errorf("DC string = %q", got)
	}
	cloud := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[
	  {"type":"text","text":"Ping "},{"type":"mention","attrs":{"id":"x","text":"@Ana"}},
	  {"type":"text","text":" "},{"type":"emoji","attrs":{"shortName":":tada:","text":"🎉"}},
	  {"type":"unknownInline","content":[{"type":"text","text":" kept"}]}]}]}`
	if got := RichTextToMarkdown(json.RawMessage(cloud)); got != "Ping @Ana 🎉 kept" {
		t.Errorf("cloud = %q", got)
	}
	if RichTextToMarkdown(nil) != "" || RichTextToMarkdown(json.RawMessage("null")) != "" {
		t.Error("empty description must be empty")
	}
}
