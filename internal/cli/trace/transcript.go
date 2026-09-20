// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

var (
	writeTools   = map[string]bool{"Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true, "write": true, "edit": true, "replace_file_content": true, "multi_replace_file_content": true, "write_to_file": true}
	readTools    = map[string]bool{"Read": true, "Glob": true, "Grep": true, "read": true, "glob": true, "grep": true, "view_file": true, "read_file": true}
	commandTools = map[string]bool{"Bash": true, "bash": true, "terminal": true, "sh": true, "run_command": true}
)

// TranscriptParseResult holds structured insights extracted from a JSONL agent session transcript.
type TranscriptParseResult struct {
	Prompts           []string              `json:"prompts"`
	AssistantMessages []string              `json:"assistant_messages"`
	ModifiedFiles     []string              `json:"modified_files"`
	FilesRead         []string              `json:"files_read"`
	Commands          []string              `json:"commands"`
	ToolCalls         []TraceToolCallRecord `json:"tool_calls"`
	TokenUsage        TokenUsage            `json:"token_usage"`
	Summary           string                `json:"summary"`
	SubagentIDs       []string              `json:"subagent_ids"`
	EntryCount        int                   `json:"entry_count"`
	NextOffset        int64                 `json:"next_offset"`
}

// ParseTranscript reads a transcript JSONL file starting from fromOffset and extracts turns and tools.
func ParseTranscript(transcriptPath string, fromOffset int64) (*TranscriptParseResult, error) {
	res := &TranscriptParseResult{
		Prompts:           make([]string, 0),
		AssistantMessages: make([]string, 0),
		ModifiedFiles:     make([]string, 0),
		FilesRead:         make([]string, 0),
		Commands:          make([]string, 0),
		ToolCalls:         make([]TraceToolCallRecord, 0),
		SubagentIDs:       make([]string, 0),
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return nil, err
	}
	defer f.Close()

	if fromOffset > 0 {
		if _, err := f.Seek(fromOffset, io.SeekStart); err != nil {
			return nil, err
		}
	}

	reader := bufio.NewReader(f)
	currentOffset := fromOffset
	modifiedSet := make(map[string]bool)
	readSet := make(map[string]bool)
	subagentSet := make(map[string]bool)
	var lastAssistantText string

	for {
		lineBytes, isPrefix, err := reader.ReadLine()
		if err != nil {
			break
		}
		currentOffset += int64(len(lineBytes)) + 1 // +1 for newline delimiter
		if isPrefix {
			// Line too long for single buffer, consume rest
			for isPrefix && err == nil {
				var extra []byte
				extra, isPrefix, err = reader.ReadLine()
				currentOffset += int64(len(extra))
			}
		}

		line := strings.TrimSpace(string(lineBytes))
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		res.EntryCount++

		// Track subagent
		if subID, ok := entry["subagent_id"].(string); ok && subID != "" {
			if !subagentSet[subID] {
				subagentSet[subID] = true
				res.SubagentIDs = append(res.SubagentIDs, subID)
			}
		}

		// Message payload: either nested in "message", top-level role, or type-based
		msgObj, hasMsg := entry["message"].(map[string]any)
		if !hasMsg {
			if _, hasRole := entry["role"]; hasRole {
				msgObj = entry
			} else if t, hasType := entry["type"].(string); hasType {
				low := strings.ToLower(t)
				if low == "user" || low == "user_input" || low == "human" {
					msgObj = map[string]any{"role": "user", "content": entry["content"]}
				} else if low == "assistant" || low == "planner_response" || low == "model" {
					msgObj = map[string]any{"role": "assistant", "content": entry["content"]}
				} else if low == "tool_use" {
					msgObj = map[string]any{
						"role": "assistant",
						"content": []any{
							map[string]any{
								"type":  "tool_use",
								"name":  entry["name"],
								"input": entry["input"],
							},
						},
					}
				}
			}
		}

		hasExtractedUsage := false
		if msgObj != nil {
			if _, ok := msgObj["usage"].(map[string]any); ok {
				hasExtractedUsage = true
			}
			processTranscriptMessage(msgObj, res, modifiedSet, readSet, &lastAssistantText)
		}

		if !hasExtractedUsage {
			if u, hasUsage := entry["usage"].(map[string]any); hasUsage {
				if in, ok := u["input_tokens"].(float64); ok {
					res.TokenUsage.InputTokens += int64(in)
				}
				if out, ok := u["output_tokens"].(float64); ok {
					res.TokenUsage.OutputTokens += int64(out)
				}
				res.TokenUsage.TotalTokens = res.TokenUsage.InputTokens + res.TokenUsage.OutputTokens
			}
		}
	}

	res.NextOffset = currentOffset
	if len(lastAssistantText) > 500 {
		res.Summary = lastAssistantText[:500]
	} else {
		res.Summary = lastAssistantText
	}

	return res, nil
}

func processTranscriptMessage(msg map[string]any, res *TranscriptParseResult, modSet, readSet map[string]bool, lastAssistant *string) {
	role, _ := msg["role"].(string)

	// Extract token usage if present
	if usage, ok := msg["usage"].(map[string]any); ok {
		if in, ok := usage["input_tokens"].(float64); ok {
			res.TokenUsage.InputTokens += int64(in)
		}
		if out, ok := usage["output_tokens"].(float64); ok {
			res.TokenUsage.OutputTokens += int64(out)
		}
		if cw, ok := usage["cache_creation_input_tokens"].(float64); ok {
			res.TokenUsage.CacheWriteTokens += int64(cw)
		}
		if cr, ok := usage["cache_read_input_tokens"].(float64); ok {
			res.TokenUsage.CacheReadTokens += int64(cr)
		}
		res.TokenUsage.TotalTokens = res.TokenUsage.InputTokens + res.TokenUsage.OutputTokens
	}

	// Content blocks or text
	content := msg["content"]
	switch c := content.(type) {
	case string:
		clean := strings.TrimSpace(c)
		if role == "user" && clean != "" {
			res.Prompts = append(res.Prompts, clean)
		} else if role == "assistant" && clean != "" {
			res.AssistantMessages = append(res.AssistantMessages, clean)
			*lastAssistant = clean
		}
	case []any:
		var assistantBuf strings.Builder
		for _, blockItem := range c {
			block, ok := blockItem.(map[string]any)
			if !ok {
				continue
			}
			bType, _ := block["type"].(string)
			switch bType {
			case "text":
				if txt, ok := block["text"].(string); ok && strings.TrimSpace(txt) != "" {
					if role == "user" {
						res.Prompts = append(res.Prompts, strings.TrimSpace(txt))
					} else if role == "assistant" {
						assistantBuf.WriteString(txt)
						assistantBuf.WriteString("\n")
					}
				}
			case "tool_use":
				toolName, _ := block["name"].(string)
				inputMap, _ := block["input"].(map[string]any)
				record := TraceToolCallRecord{
					ToolName: toolName,
				}

				if inputMap != nil {
					// Extract file path or command
					var targetFile string
					for _, k := range []string{"path", "file_path", "target_file", "filePath", "TargetFile"} {
						if v, ok := inputMap[k].(string); ok && v != "" {
							targetFile = v
							break
						}
					}
					record.FileAffected = targetFile

					if cmd, ok := inputMap["command"].(string); ok && cmd != "" {
						record.Summary = cmd
						res.Commands = append(res.Commands, cmd)
					} else if desc, ok := inputMap["description"].(string); ok && desc != "" {
						record.Summary = desc
					}

					if targetFile != "" {
						if writeTools[toolName] && !modSet[targetFile] {
							modSet[targetFile] = true
							res.ModifiedFiles = append(res.ModifiedFiles, targetFile)
						} else if readTools[toolName] && !readSet[targetFile] {
							readSet[targetFile] = true
							res.FilesRead = append(res.FilesRead, targetFile)
						}
					}
				}
				res.ToolCalls = append(res.ToolCalls, record)
			}
		}

		if role == "assistant" && assistantBuf.Len() > 0 {
			txt := strings.TrimSpace(assistantBuf.String())
			res.AssistantMessages = append(res.AssistantMessages, txt)
			*lastAssistant = txt
		}
	}
}

// ExtractPrompts extracts only prompt strings from a transcript.
func ExtractPrompts(transcriptPath string) ([]string, error) {
	res, err := ParseTranscript(transcriptPath, 0)
	if err != nil {
		return nil, err
	}
	return res.Prompts, nil
}

// ExtractSummary returns the latest assistant summary text from a transcript.
func ExtractSummary(transcriptPath string) (string, error) {
	res, err := ParseTranscript(transcriptPath, 0)
	if err != nil {
		return "", err
	}
	return res.Summary, nil
}

// ExtractModifiedFiles returns unique modified files from tool calls.
func ExtractModifiedFiles(transcriptPath string) ([]string, error) {
	res, err := ParseTranscript(transcriptPath, 0)
	if err != nil {
		return nil, err
	}
	return res.ModifiedFiles, nil
}

// CalculateTokenUsage sums up token consumption from assistant messages.
func CalculateTokenUsage(transcriptPath string) (*TokenUsage, error) {
	res, err := ParseTranscript(transcriptPath, 0)
	if err != nil {
		return nil, err
	}
	return &res.TokenUsage, nil
}

// WaitForTranscriptFlush waits for a transcript file to stabilize (no writes for 150ms).
func WaitForTranscriptFlush(transcriptPath string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	pollInterval := 50 * time.Millisecond
	if timeout < 200*time.Millisecond {
		pollInterval = timeout / 5
		if pollInterval < 5*time.Millisecond {
			pollInterval = 5 * time.Millisecond
		}
	}
	startTime := time.Now()
	var lastSize int64 = -1
	stableCount := 0
	requiredStable := 3

	for time.Since(startTime) < timeout {
		info, err := os.Stat(transcriptPath)
		if err == nil {
			if info.Size() == lastSize {
				stableCount++
				if stableCount >= requiredStable {
					return true
				}
			} else {
				stableCount = 0
				lastSize = info.Size()
			}
		} else {
			stableCount = 0
		}
		time.Sleep(pollInterval)
	}
	return stableCount >= requiredStable
}
