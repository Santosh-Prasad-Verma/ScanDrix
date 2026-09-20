// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// MCPToolToEngineTool converts an MCP tool definition into an EngineTool with schema enhancement.
func MCPToolToEngineTool(mcpTool MCPToolRawWithServer) (*EngineTool, error) {
	name := strings.TrimSpace(mcpTool.Name)
	if name == "" {
		return nil, errors.New("invalid MCP tool name: empty")
	}

	if strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid tool name (path traversal sequence detected): %s", name)
	}

	inputSchema := mcpTool.InputSchema
	if !ValidateMCPSchema(inputSchema) {
		inputSchema = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	outputSchema := mcpTool.OutputSchema
	if outputSchema != nil && !ValidateMCPSchema(outputSchema) {
		outputSchema = nil
	}

	enhancedInputSchema := enhanceMCPSchema(inputSchema)
	enhancedOutputSchema := enhanceMCPSchema(outputSchema)

	desc := mcpTool.Description
	if desc == "" {
		desc = mcpTool.Title
	}
	if desc == "" {
		desc = fmt.Sprintf("MCP Tool: %s", name)
	}

	return &EngineTool{
		Name:         name,
		Description:  desc,
		InputSchema:  enhancedInputSchema,
		OutputSchema: enhancedOutputSchema,
		Annotations:  mcpTool.Annotations,
		Title:        mcpTool.Title,
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			return nil, errors.New("tool execute function not connected to MCP client")
		},
	}, nil
}

// MCPToolsToEngineTools validates multiple MCP tools and checks for duplicate name conflicts across servers.
func MCPToolsToEngineTools(mcpTools []MCPToolRawWithServer) ([]*EngineTool, error) {
	toolNameServers := make(map[string][]string)

	for _, tool := range mcpTools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		server := tool.ServerName
		if server == "" {
			server = "default"
		}
		toolNameServers[name] = append(toolNameServers[name], server)
	}

	// Detect conflict across distinct servers
	for toolName, servers := range toolNameServers {
		distinct := make(map[string]bool)
		for _, s := range servers {
			distinct[s] = true
		}
		if len(distinct) > 1 {
			var serverList []string
			for s := range distinct {
				serverList = append(serverList, s)
			}
			return nil, fmt.Errorf("tool name conflict detected: '%s' exists in multiple servers: %s. Please ensure tool names are unique",
				toolName, strings.Join(serverList, ", "))
		}
	}

	validTools := make([]*EngineTool, 0, len(mcpTools))
	for _, tool := range mcpTools {
		engineTool, err := MCPToolToEngineTool(tool)
		if err == nil && engineTool != nil {
			validTools = append(validTools, engineTool)
		}
	}

	return validTools, nil
}

// ParseToolName extracts tool and server names.
func ParseToolName(fullName string) (serverName, toolName string) {
	trimmed := strings.TrimSpace(fullName)
	if trimmed == "" {
		return "", ""
	}

	// If formatted as "server:tool" or "server/tool"
	if idx := strings.IndexAny(trimmed, ":/"); idx != -1 {
		return trimmed[:idx], trimmed[idx+1:]
	}

	return "", trimmed
}

func enhanceMCPSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	enhanced := make(map[string]any, len(schema))
	for k, v := range schema {
		enhanced[k] = v
	}

	properties, ok := enhanced["properties"].(map[string]any)
	if !ok || properties == nil {
		return enhanced
	}

	// Infer required fields if not explicitly specified
	existingRequired, _ := enhanced["required"].([]string)
	if len(existingRequired) == 0 {
		var inferred []string
		for propName, propDef := range properties {
			if propObj, isMap := propDef.(map[string]any); isMap {
				if req, hasReq := propObj["required"].(bool); hasReq && req {
					inferred = append(inferred, propName)
				}
			}
		}
		if len(inferred) > 0 {
			enhanced["required"] = inferred
		}
	}

	enhancedProperties := make(map[string]any, len(properties))
	for propName, propDef := range properties {
		if propObj, isMap := propDef.(map[string]any); isMap {
			copyProp := make(map[string]any, len(propObj))
			for pk, pv := range propObj {
				copyProp[pk] = pv
			}

			// Recursive object enhancement
			if pType, _ := copyProp["type"].(string); pType == "object" {
				if nestedProps, hasNested := copyProp["properties"].(map[string]any); hasNested {
					copyProp["properties"] = enhanceMCPSchema(nestedProps)
				}
			}

			// Recursive array items enhancement
			if pType, _ := copyProp["type"].(string); pType == "array" {
				if itemDef, hasItems := copyProp["items"].(map[string]any); hasItems {
					copyProp["items"] = enhanceMCPSchema(itemDef)
				}
			}

			enhancedProperties[propName] = copyProp
		} else {
			enhancedProperties[propName] = propDef
		}
	}

	enhanced["properties"] = enhancedProperties
	return enhanced
}
