package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"one-mcp/backend/common"
	"one-mcp/backend/library/proxy"
	"one-mcp/backend/model"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"
)

// ---------- Wrapped mode server ----------

func buildWrappedMCPServer(server *mcpserver.MCPServer, group *model.MCPServiceGroup) (*mcpserver.MCPServer, error) {
	if err := addGroupTools(server, group); err != nil {
		return nil, err
	}
	if err := addGroupResources(server, group); err != nil {
		return nil, err
	}
	return server, nil
}

// ---------- Tools (search_tools + execute_tool) ----------

type groupSearchArgs struct {
	MCPName string
}

type executeArgs struct {
	MCPName   string
	ToolName  string
	Arguments map[string]any
}

func addGroupTools(server *mcpserver.MCPServer, group *model.MCPServiceGroup) error {
	if server == nil {
		return errors.New("mcp server is nil")
	}

	serviceNames := getGroupServiceNames(group)

	searchTool := mcp.Tool{
		Name:        "search_tools",
		Description: "STEP 1: Discover available tools in a service. You MUST call this first before execute_tool.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"mcp_name": map[string]any{
					"type":        "string",
					"enum":        serviceNames,
					"description": "MCP service name",
				},
			},
			Required: []string{"mcp_name"},
		},
	}

	executeTool := mcp.Tool{
		Name:        "execute_tool",
		Description: "STEP 2: Execute a tool found via search_tools. Pass arguments directly, do NOT nest.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"mcp_name": map[string]any{
					"type":        "string",
					"enum":        serviceNames,
					"description": "MCP service name",
				},
				"tool_name": map[string]any{
					"type":        "string",
					"description": "Tool name from search_tools",
				},
				"arguments": map[string]any{
					"type":        "object",
					"description": "Tool arguments. Example: {\"message\": \"hello\"} for a tool with message param",
				},
			},
			Required: []string{"mcp_name", "tool_name", "arguments"},
		},
	}

	server.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := common.ParseAnyToMap(request.Params.Arguments)
		if args == nil {
			args = map[string]any{}
		}
		parsed, err := parseGroupSearchArgs(args)
		if err != nil {
			return toolErrorResult(err), nil
		}
		result, err := searchGroupTools(ctx, group, parsed)
		if err != nil {
			return toolErrorResult(err), nil
		}
		return toolResultFromStructured(result), nil
	})

	server.AddTool(executeTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := common.ParseAnyToMap(request.Params.Arguments)
		if args == nil {
			args = map[string]any{}
		}
		parsed, err := parseExecuteArgs(args)
		if err != nil {
			return toolErrorResult(err), nil
		}
		result, err := executeGroupTool(ctx, group, parsed)
		if err != nil {
			return toolErrorResult(err), nil
		}
		return toolResultFromStructured(result), nil
	})

	return nil
}

// ---------- Resources ----------

func addGroupResources(server *mcpserver.MCPServer, group *model.MCPServiceGroup) error {
	if server == nil {
		return errors.New("mcp server is nil")
	}

	ids := group.GetServiceIDs()
	for _, id := range ids {
		svc, err := model.GetServiceByID(id)
		if err != nil {
			continue
		}

		resourceURI := fmt.Sprintf("mcp://%s/%s", group.Name, svc.Name)

		resource := mcp.Resource{
			URI:         resourceURI,
			Name:        svc.Name,
			Description: svc.Description,
			MIMEType:    "application/yaml",
		}

		currentSvc := svc

		server.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			args := &groupSearchArgs{
				MCPName: currentSvc.Name,
			}

			result, err := searchGroupTools(ctx, group, args)
			if err != nil {
				return nil, err
			}

			resultMap, ok := result.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("internal error: unexpected result type")
			}

			var contentStr string
			if rawContent, ok := resultMap["content"].([]map[string]any); ok && len(rawContent) > 0 {
				contentStr, _ = rawContent[0]["text"].(string)
			}

			if contentStr == "" {
				contentStr = "# No tools available or failed to fetch"
			}

			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "application/yaml",
					Text:     contentStr,
				},
			}, nil
		})
	}

	return nil
}

// ---------- Argument parsing ----------

func parseGroupSearchArgs(args map[string]any) (*groupSearchArgs, error) {
	mcpName, _ := args["mcp_name"].(string)
	if strings.TrimSpace(mcpName) == "" {
		return nil, fmt.Errorf("mcp_name is required")
	}
	return &groupSearchArgs{
		MCPName: strings.TrimSpace(mcpName),
	}, nil
}

func parseExecuteArgs(args map[string]any) (*executeArgs, error) {
	mcpName, _ := args["mcp_name"].(string)
	toolName, _ := args["tool_name"].(string)
	if strings.TrimSpace(mcpName) == "" || strings.TrimSpace(toolName) == "" {
		return nil, fmt.Errorf("mcp_name and tool_name are required")
	}

	arguments, fieldFound := parseArgumentsValue(args)
	if !fieldFound {
		arguments = extractRemainingAsArguments(args)
	}
	if arguments == nil {
		arguments = map[string]any{}
	}

	return &executeArgs{
		MCPName:   strings.TrimSpace(mcpName),
		ToolName:  strings.TrimSpace(toolName),
		Arguments: arguments,
	}, nil
}

// extractRemainingAsArguments collects all fields except mcp_name/tool_name as arguments.
func extractRemainingAsArguments(args map[string]any) map[string]any {
	reserved := map[string]bool{"mcp_name": true, "tool_name": true, "arguments": true, "parameters": true}
	result := make(map[string]any)
	for k, v := range args {
		if !reserved[k] {
			result[k] = v
		}
	}
	return result
}

// parseArgumentsValue parses arguments that could be either a map or a JSON string.
func parseArgumentsValue(args map[string]any) (map[string]any, bool) {
	for _, fieldName := range []string{"arguments", "parameters"} {
		if v, ok := args[fieldName]; ok && v != nil {
			return common.ParseAnyToMap(v), true
		}
	}
	return nil, false
}

// ---------- Search / Execute ----------

func getGroupServiceNames(group *model.MCPServiceGroup) []string {
	ids := group.GetServiceIDs()
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		svc, err := model.GetServiceByID(id)
		if err == nil {
			names = append(names, svc.Name)
		}
	}
	return names
}

func searchGroupTools(ctx context.Context, group *model.MCPServiceGroup, args *groupSearchArgs) (any, error) {
	svc, err := group.GetServiceByName(args.MCPName)
	if err != nil {
		available := getGroupServiceNames(group)
		return nil, fmt.Errorf("mcp_name '%s' not in group, available: %v", args.MCPName, available)
	}

	currentTime := time.Now().Format("2006-01-02 15:04")

	toolsCacheMgr := proxy.GetToolsCacheManager()
	entry, ok := toolsCacheMgr.GetServiceTools(svc.ID)

	var tools []mcp.Tool
	if !ok || len(entry.Tools) == 0 {
		fetchedTools, fetchErr := fetchToolsFromService(ctx, svc)
		if fetchErr != nil {
			return nil, fmt.Errorf("failed to fetch tools from %s: %v", svc.Name, fetchErr)
		}
		tools = fetchedTools
	} else {
		tools = entry.Tools
	}

	yamlTools := convertToolsToYAML(tools, svc.Name)
	yamlBytes, err := yaml.Marshal(yamlTools)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize tools: %v", err)
	}

	toolsSummary := string(yamlBytes)
	toolsSummaryWithTime := fmt.Sprintf("# current_time: %s\n%s", currentTime, toolsSummary)
	return map[string]any{
		"content": []map[string]any{
			{
				"type": mcp.ContentTypeText,
				"text": toolsSummaryWithTime,
			},
		},
	}, nil
}

func fetchToolsFromService(ctx context.Context, svc *model.MCPService) ([]mcp.Tool, error) {
	sharedInst, err := proxy.GetOrCreateSharedMcpInstanceWithKey(ctx, svc, proxy.SharedServiceCacheKey(svc.ID), proxy.SharedServiceInstanceName(svc.ID), svc.DefaultEnvsJSON)
	if err != nil {
		return nil, err
	}

	toolsReq := mcp.ListToolsRequest{}
	result, err := sharedInst.Client.ListTools(ctx, toolsReq)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []mcp.Tool{}, nil
	}
	return result.Tools, nil
}

// yamlTool is a compact YAML-friendly tool representation.
type yamlTool struct {
	Name   string         `yaml:"name"`
	Desc   string         `yaml:"desc,omitempty"`
	Params map[string]any `yaml:"params,omitempty"`
}

func convertToolsToYAML(tools []mcp.Tool, mcpName string) []yamlTool {
	result := make([]yamlTool, 0, len(tools))
	for _, tool := range tools {
		yt := yamlTool{
			Name: tool.Name,
			Desc: tool.Description,
		}
		if len(tool.InputSchema.Properties) > 0 {
			yt.Params = tool.InputSchema.Properties
		}
		result = append(result, yt)
	}
	return result
}

func executeGroupTool(ctx context.Context, group *model.MCPServiceGroup, args *executeArgs) (any, error) {
	svc, err := group.GetServiceByName(args.MCPName)
	if err != nil {
		available := getGroupServiceNames(group)
		return nil, fmt.Errorf("mcp_name '%s' not in group, available: %v", args.MCPName, available)
	}

	return callServiceTool(ctx, svc, args.ToolName, args.Arguments, group.Name)
}
