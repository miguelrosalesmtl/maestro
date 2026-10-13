package jira

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Atlassian Document Format (ADF) ⇄ Markdown.
//
// Supported both ways: paragraphs, headings, bold, italic, strike, inline code,
// links, bullet/ordered/task lists, code blocks, quotes, rules, hard breaks.
// Read only: mentions, emoji, inline cards, panels, tables (as text), media.
// Unknown nodes are rendered through their children, so text is never lost.

type ADFNode struct {
	Type    string         `json:"type"`
	Version int            `json:"version,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []*ADFNode     `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []ADFMark      `json:"marks,omitempty"`
}

type ADFMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// ── Markdown → ADF ──────────────────────────────────────────────

var (
	reHeading = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reTask    = regexp.MustCompile(`^\s*[-*]\s+\[( |x|X)\]\s+(.*)$`)
	reBullet  = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	reOrdered = regexp.MustCompile(`^\s*\d+[.)]\s+(.*)$`)
	reRule    = regexp.MustCompile(`^\s*(---+|\*\*\*+)\s*$`)
	reInline  = regexp.MustCompile("`([^`]+)`|\\*\\*([^*]+)\\*\\*|~~([^~]+)~~|\\[([^\\]]+)\\]\\(([^)\\s]+)\\)|\\*([^*]+)\\*|_([^_]+)_")
)

// FromMarkdown converts Markdown to an ADF document.
func FromMarkdown(md string) *ADFNode {
	doc := &ADFNode{Type: "doc", Version: 1, Content: []*ADFNode{}}
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var para []string
	var list *ADFNode
	localID := 0
	nextID := func() string { localID++; return "t" + strconv.Itoa(localID) }

	flushPara := func() {
		if len(para) == 0 {
			return
		}
		p := &ADFNode{Type: "paragraph"}
		for i, l := range para {
			if i > 0 {
				p.Content = append(p.Content, &ADFNode{Type: "hardBreak"})
			}
			p.Content = append(p.Content, inlineADF(l)...)
		}
		doc.Content = append(doc.Content, p)
		para = nil
	}
	flushList := func() {
		if list != nil {
			doc.Content = append(doc.Content, list)
			list = nil
		}
	}
	addItem := func(kind string, item *ADFNode) {
		if list == nil || list.Type != kind {
			flushList()
			list = &ADFNode{Type: kind}
			if kind == "taskList" {
				list.Attrs = map[string]any{"localId": nextID()}
			}
		}
		list.Content = append(list.Content, item)
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			flushPara()
			flushList()
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			var code []string
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
				code = append(code, lines[i])
			}
			cb := &ADFNode{Type: "codeBlock", Content: []*ADFNode{{Type: "text", Text: strings.Join(code, "\n")}}}
			if lang != "" {
				cb.Attrs = map[string]any{"language": lang}
			}
			if len(code) == 0 {
				cb.Content = nil
			}
			doc.Content = append(doc.Content, cb)
			continue
		}
		if m := reTask.FindStringSubmatch(line); m != nil {
			flushPara()
			state := "TODO"
			if strings.EqualFold(m[1], "x") {
				state = "DONE"
			}
			addItem("taskList", &ADFNode{Type: "taskItem", Attrs: map[string]any{"localId": nextID(), "state": state}, Content: inlineADF(m[2])})
			continue
		}
		if m := reBullet.FindStringSubmatch(line); m != nil && !reRule.MatchString(line) {
			flushPara()
			addItem("bulletList", listItem(m[1]))
			continue
		}
		if m := reOrdered.FindStringSubmatch(line); m != nil {
			flushPara()
			addItem("orderedList", listItem(m[1]))
			continue
		}
		flushList()
		switch {
		case trimmed == "":
			flushPara()
		case reRule.MatchString(line):
			flushPara()
			doc.Content = append(doc.Content, &ADFNode{Type: "rule"})
		case reHeading.MatchString(trimmed):
			flushPara()
			m := reHeading.FindStringSubmatch(trimmed)
			doc.Content = append(doc.Content, &ADFNode{Type: "heading", Attrs: map[string]any{"level": len(m[1])}, Content: inlineADF(m[2])})
		case strings.HasPrefix(trimmed, ">"):
			flushPara()
			q := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			doc.Content = append(doc.Content, &ADFNode{Type: "blockquote", Content: []*ADFNode{{Type: "paragraph", Content: inlineADF(q)}}})
		default:
			para = append(para, trimmed)
		}
	}
	flushPara()
	flushList()
	return doc
}

func listItem(text string) *ADFNode {
	return &ADFNode{Type: "listItem", Content: []*ADFNode{{Type: "paragraph", Content: inlineADF(text)}}}
}

func inlineADF(s string) []*ADFNode {
	var out []*ADFNode
	text := func(t string, marks ...ADFMark) {
		if t != "" {
			out = append(out, &ADFNode{Type: "text", Text: t, Marks: marks})
		}
	}
	last := 0
	for _, m := range reInline.FindAllStringSubmatchIndex(s, -1) {
		text(s[last:m[0]])
		group := func(n int) string { return s[m[2*n]:m[2*n+1]] }
		switch {
		case m[2] >= 0:
			text(group(1), ADFMark{Type: "code"})
		case m[4] >= 0:
			text(group(2), ADFMark{Type: "strong"})
		case m[6] >= 0:
			text(group(3), ADFMark{Type: "strike"})
		case m[8] >= 0:
			text(group(4), ADFMark{Type: "link", Attrs: map[string]any{"href": group(5)}})
		case m[12] >= 0:
			text(group(6), ADFMark{Type: "em"})
		case m[14] >= 0:
			text(group(7), ADFMark{Type: "em"})
		}
		last = m[1]
	}
	text(s[last:])
	return out
}

// ── ADF → Markdown ──────────────────────────────────────────────

// RichTextToMarkdown accepts a description or comment body as Jira returns
// it: an ADF object (Cloud) or a plain string (Data Center).
func RichTextToMarkdown(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var doc ADFNode
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	return ToMarkdown(&doc)
}

// ToMarkdown converts an ADF tree to Markdown.
func ToMarkdown(n *ADFNode) string {
	var b strings.Builder
	writeBlocks(&b, n.Content, "")
	return strings.TrimRight(b.String(), "\n")
}

func writeBlocks(b *strings.Builder, nodes []*ADFNode, indent string) {
	for _, n := range nodes {
		writeBlock(b, n, indent)
	}
}

func writeBlock(b *strings.Builder, n *ADFNode, indent string) {
	switch n.Type {
	case "paragraph":
		b.WriteString(indent + inlineMD(n.Content) + "\n\n")
	case "heading":
		level := 1
		if l, ok := n.Attrs["level"].(float64); ok {
			level = int(l)
		} else if l, ok := n.Attrs["level"].(int); ok {
			level = l
		}
		b.WriteString(strings.Repeat("#", max(1, min(level, 6))) + " " + inlineMD(n.Content) + "\n\n")
	case "bulletList", "orderedList", "taskList":
		for i, item := range n.Content {
			marker := "- "
			switch {
			case n.Type == "orderedList":
				marker = strconv.Itoa(i+1) + ". "
			case item.Type == "taskItem":
				marker = "- [ ] "
				if item.Attrs["state"] == "DONE" {
					marker = "- [x] "
				}
			}
			writeListItem(b, item, indent, marker)
		}
		if indent == "" {
			b.WriteString("\n")
		}
	case "codeBlock":
		lang, _ := n.Attrs["language"].(string)
		b.WriteString("```" + lang + "\n" + plainText(n.Content) + "\n```\n\n")
	case "blockquote", "panel":
		var inner strings.Builder
		writeBlocks(&inner, n.Content, "")
		for _, l := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			b.WriteString("> " + l + "\n")
		}
		b.WriteString("\n")
	case "rule":
		b.WriteString("---\n\n")
	case "table":
		for _, row := range n.Content {
			var cells []string
			for _, cell := range row.Content {
				var cb strings.Builder
				writeBlocks(&cb, cell.Content, "")
				cells = append(cells, strings.ReplaceAll(strings.TrimSpace(cb.String()), "\n", " "))
			}
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
		b.WriteString("\n")
	case "mediaSingle", "mediaGroup", "media":
		b.WriteString("[attachment]\n\n")
	default:
		if len(n.Content) > 0 {
			writeBlocks(b, n.Content, indent)
		} else if n.Text != "" {
			b.WriteString(n.Text + "\n\n")
		}
	}
}

func writeListItem(b *strings.Builder, item *ADFNode, indent, marker string) {
	if item.Type == "taskItem" { // task items hold inline content directly
		b.WriteString(indent + marker + inlineMD(item.Content) + "\n")
		return
	}
	first := true
	for _, child := range item.Content {
		switch child.Type {
		case "paragraph":
			if first {
				b.WriteString(indent + marker + inlineMD(child.Content) + "\n")
			} else {
				b.WriteString(indent + "  " + inlineMD(child.Content) + "\n")
			}
		case "bulletList", "orderedList", "taskList":
			writeBlock(b, child, indent+"  ")
		default:
			writeBlock(b, child, indent+"  ")
		}
		first = false
	}
}

func inlineMD(nodes []*ADFNode) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case "text":
			t := n.Text
			for _, m := range n.Marks {
				switch m.Type {
				case "code":
					t = "`" + t + "`"
				case "strong":
					t = "**" + t + "**"
				case "em":
					t = "*" + t + "*"
				case "strike":
					t = "~~" + t + "~~"
				case "link":
					if href, _ := m.Attrs["href"].(string); href != "" {
						t = fmt.Sprintf("[%s](%s)", t, href)
					}
				}
			}
			b.WriteString(t)
		case "hardBreak":
			b.WriteString("\n")
		case "mention":
			text, _ := n.Attrs["text"].(string)
			if !strings.HasPrefix(text, "@") {
				text = "@" + text
			}
			b.WriteString(text)
		case "emoji":
			if t, _ := n.Attrs["text"].(string); t != "" {
				b.WriteString(t)
			} else if s, _ := n.Attrs["shortName"].(string); s != "" {
				b.WriteString(s)
			}
		case "inlineCard", "blockCard":
			if u, _ := n.Attrs["url"].(string); u != "" {
				b.WriteString(u)
			}
		case "date":
			if ts, _ := n.Attrs["timestamp"].(string); ts != "" {
				b.WriteString(ts)
			}
		case "status":
			if t, _ := n.Attrs["text"].(string); t != "" {
				b.WriteString("[" + t + "]")
			}
		default:
			b.WriteString(inlineMD(n.Content))
		}
	}
	return b.String()
}

func plainText(nodes []*ADFNode) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(n.Text)
		b.WriteString(plainText(n.Content))
	}
	return b.String()
}
