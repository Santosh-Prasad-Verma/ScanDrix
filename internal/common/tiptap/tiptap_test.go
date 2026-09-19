package tiptap

import (
	"strings"
	"testing"
)

func TestConvertTiptapJSONToText(t *testing.T) {
	jsonDoc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "text", "text": "Hello world!"}
				]
			},
			{
				"type": "paragraph",
				"content": [
					{"type": "text", "text": "Second paragraph."}
				]
			}
		]
	}`

	text := ConvertTiptapJSONToText(jsonDoc)
	if !strings.Contains(text, "Hello world!") || !strings.Contains(text, "Second paragraph.") {
		t.Fatalf("unexpected text output: %s", text)
	}

	// Plain string returns itself
	plain := "plain text string"
	if ConvertTiptapJSONToText(plain) != plain {
		t.Fatalf("expected plain string to be returned as-is")
	}
}

func TestConvertTiptapJSONToMarkdown(t *testing.T) {
	jsonDoc := `{
		"type": "doc",
		"content": [
			{
				"type": "heading",
				"attrs": {"level": 2},
				"content": [
					{"type": "text", "text": "Rule Heading"}
				]
			},
			{
				"type": "paragraph",
				"content": [
					{
						"type": "text",
						"text": "Check out this link",
						"marks": [
							{"type": "bold"},
							{"type": "link", "attrs": {"href": "https://scandrix.dev"}}
						]
					}
				]
			},
			{
				"type": "bulletList",
				"content": [
					{
						"type": "listItem",
						"content": [
							{"type": "text", "text": "Bullet item 1"}
						]
					},
					{
						"type": "listItem",
						"content": [
							{"type": "text", "text": "Bullet item 2"}
						]
					}
				]
			},
			{
				"type": "codeBlock",
				"attrs": {"language": "go"},
				"content": [
					{"type": "text", "text": "package main\nfunc main() {}"}
				]
			}
		]
	}`

	md := ConvertTiptapJSONToMarkdown(jsonDoc)
	if !strings.Contains(md, "## Rule Heading") {
		t.Fatalf("expected heading level 2, got:\n%s", md)
	}
	if !strings.Contains(md, "**[Check out this link](https://scandrix.dev)**") {
		t.Fatalf("expected formatted link with bold, got:\n%s", md)
	}
	if !strings.Contains(md, "- Bullet item 1") || !strings.Contains(md, "- Bullet item 2") {
		t.Fatalf("expected bullet list items, got:\n%s", md)
	}
	if !strings.Contains(md, "```go\npackage main\nfunc main() {}\n```") {
		t.Fatalf("expected code block, got:\n%s", md)
	}
}

func TestConvertTiptapJSONToMarkdownMcpMention(t *testing.T) {
	jsonDoc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{
						"type": "mcpMention",
						"attrs": {
							"app": "github",
							"tool": "get_file",
							"resolvedOutput": "Fetched repository tree"
						}
					}
				]
			}
		]
	}`

	// Without keepMcpMentions
	mdOmitted := ConvertTiptapJSONToMarkdown(jsonDoc, MarkdownOptions{KeepMcpMentions: false})
	if strings.Contains(mdOmitted, "Fetched repository tree") {
		t.Fatalf("expected mcp mention to be omitted when KeepMcpMentions is false")
	}

	// With keepMcpMentions
	mdKept := ConvertTiptapJSONToMarkdown(jsonDoc, MarkdownOptions{KeepMcpMentions: true})
	if !strings.Contains(mdKept, "`Fetched repository tree`") {
		t.Fatalf("expected resolved output in backticks, got: %s", mdKept)
	}
}
