package tiptap

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type TiptapMark struct {
	Type  string                 `json:"type"`
	Attrs map[string]interface{} `json:"attrs,omitempty"`
}

type TiptapNode struct {
	Type    string                 `json:"type"`
	Text    string                 `json:"text,omitempty"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
	Content []TiptapNode           `json:"content,omitempty"`
	Marks   []TiptapMark           `json:"marks,omitempty"`
}

type MarkdownOptions struct {
	KeepMcpMentions bool `json:"keepMcpMentions"`
}

func tryParseJSON(value string) (*TiptapNode, bool) {
	var node TiptapNode
	if err := json.Unmarshal([]byte(value), &node); err == nil && node.Type != "" {
		return &node, true
	}
	return nil, false
}

func ConvertTiptapJSONToText(content interface{}) string {
	if content == nil {
		return ""
	}

	var rootNode *TiptapNode
	switch v := content.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "{") {
			if node, ok := tryParseJSON(trimmed); ok {
				rootNode = node
			} else {
				return v
			}
		} else {
			return v
		}
	case TiptapNode:
		rootNode = &v
	case *TiptapNode:
		rootNode = v
	default:
		// Try marshaling then unmarshaling into TiptapNode
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		if node, ok := tryParseJSON(string(b)); ok {
			rootNode = node
		} else {
			return ""
		}
	}

	var result strings.Builder
	var traverse func(node *TiptapNode)
	traverse = func(node *TiptapNode) {
		if node == nil {
			return
		}
		switch node.Type {
		case "text":
			result.WriteString(node.Text)
		case "hardBreak":
			result.WriteString("\n")
		case "paragraph", "heading":
			for i := range node.Content {
				traverse(&node.Content[i])
			}
			result.WriteString("\n")
		case "mcpMention":
			resolved, ok := node.Attrs["resolvedOutput"].(string)
			if ok && resolved != "" {
				result.WriteString(resolved)
			} else {
				app, _ := node.Attrs["app"].(string)
				tool, _ := node.Attrs["tool"].(string)
				result.WriteString(fmt.Sprintf("@mcp<%s|%s>", app, tool))
			}
		default:
			for i := range node.Content {
				traverse(&node.Content[i])
			}
		}
	}

	traverse(rootNode)
	res := result.String()
	re := regexp.MustCompile(`\n{3,}`)
	res = re.ReplaceAllString(res, "\n\n")
	return strings.TrimRight(res, " \t\r\n")
}

type listContext struct {
	inList    bool
	listType  string
	listIndex int
}

func applyMarks(text string, marks []TiptapMark) string {
	if len(marks) == 0 {
		return text
	}
	res := text
	for i := len(marks) - 1; i >= 0; i-- {
		m := marks[i]
		switch m.Type {
		case "bold":
			res = fmt.Sprintf("**%s**", res)
		case "italic":
			res = fmt.Sprintf("*%s*", res)
		case "code":
			res = fmt.Sprintf("`%s`", res)
		case "strike":
			res = fmt.Sprintf("~~%s~~", res)
		case "link":
			if href, ok := m.Attrs["href"].(string); ok && href != "" {
				res = fmt.Sprintf("[%s](%s)", res, href)
			}
		}
	}
	return res
}

func ConvertTiptapJSONToMarkdown(content interface{}, opts ...MarkdownOptions) string {
	if content == nil {
		return ""
	}

	var opt MarkdownOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	var rootNode *TiptapNode
	switch v := content.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "{") {
			if node, ok := tryParseJSON(trimmed); ok {
				rootNode = node
			} else {
				return v
			}
		} else {
			return v
		}
	case TiptapNode:
		rootNode = &v
	case *TiptapNode:
		rootNode = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		if node, ok := tryParseJSON(string(b)); ok {
			rootNode = node
		} else {
			return ""
		}
	}

	var markdown strings.Builder
	var traverse func(node *TiptapNode, ctx *listContext)

	traverse = func(node *TiptapNode, ctx *listContext) {
		if node == nil {
			return
		}

		switch node.Type {
		case "text":
			markdown.WriteString(applyMarks(node.Text, node.Marks))
		case "hardBreak":
			markdown.WriteString("  \n")
		case "mcpMention":
			if !opt.KeepMcpMentions {
				return
			}
			resolved, ok := node.Attrs["resolvedOutput"].(string)
			if ok && resolved != "" {
				markdown.WriteString(fmt.Sprintf("`%s`", resolved))
			} else {
				app, _ := node.Attrs["app"].(string)
				tool, _ := node.Attrs["tool"].(string)
				markdown.WriteString(fmt.Sprintf("`@mcp<%s|%s>`", app, tool))
			}
		case "paragraph":
			before := markdown.Len()
			for i := range node.Content {
				traverse(&node.Content[i], ctx)
			}
			if (ctx == nil || !ctx.inList) && markdown.Len() > before {
				markdown.WriteString("\n\n")
			}
		case "heading":
			level := 1
			if l, ok := node.Attrs["level"].(float64); ok && l >= 1 {
				level = int(l)
			}
			markdown.WriteString(strings.Repeat("#", level) + " ")
			for i := range node.Content {
				traverse(&node.Content[i], ctx)
			}
			markdown.WriteString("\n\n")
		case "bulletList":
			for i := range node.Content {
				traverse(&node.Content[i], &listContext{
					inList:   true,
					listType: "bullet",
				})
			}
			if ctx == nil || !ctx.inList {
				markdown.WriteString("\n")
			}
		case "orderedList":
			start := 1
			if s, ok := node.Attrs["start"].(float64); ok && s >= 1 {
				start = int(s)
			}
			for i := range node.Content {
				traverse(&node.Content[i], &listContext{
					inList:    true,
					listType:  "ordered",
					listIndex: start,
				})
				start++
			}
			if ctx == nil || !ctx.inList {
				markdown.WriteString("\n")
			}
		case "listItem":
			prefix := "- "
			if ctx != nil && ctx.listType == "ordered" {
				prefix = fmt.Sprintf("%d. ", ctx.listIndex)
			}
			markdown.WriteString(prefix)
			for i := range node.Content {
				traverse(&node.Content[i], ctx)
			}
			markdown.WriteString("\n")
		case "blockquote":
			before := markdown.Len()
			for i := range node.Content {
				traverse(&node.Content[i], ctx)
			}
			if markdown.Len() > before {
				contentStr := markdown.String()[before:]
				lines := strings.Split(contentStr, "\n")
				var quoted []string
				for _, line := range lines {
					if strings.TrimSpace(line) != "" {
						quoted = append(quoted, "> "+line)
					}
				}
				orig := markdown.String()[:before]
				markdown.Reset()
				markdown.WriteString(orig + strings.Join(quoted, "\n") + "\n\n")
			}
		case "codeBlock":
			lang, _ := node.Attrs["language"].(string)
			markdown.WriteString(fmt.Sprintf("```%s\n", lang))
			for _, child := range node.Content {
				if child.Type == "text" {
					markdown.WriteString(child.Text)
				}
			}
			markdown.WriteString("\n```\n\n")
		case "horizontalRule":
			markdown.WriteString("---\n\n")
		default:
			for i := range node.Content {
				traverse(&node.Content[i], ctx)
			}
		}
	}

	traverse(rootNode, nil)
	res := markdown.String()
	re := regexp.MustCompile(`\n{3,}`)
	res = re.ReplaceAllString(res, "\n\n")
	return strings.TrimSpace(res)
}
