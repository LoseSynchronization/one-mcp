package handler

import (
	"context"

	"one-mcp/backend/common"
	"one-mcp/backend/library/proxy"
	"one-mcp/backend/model"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// buildNativeMCPServer registers each cached tool from all services directly
// on the mcp-go server with a prefixed name (e.g. "serviceName.toolName").
// Clients see all tools via the standard tools/list endpoint and can call them
// directly via tools/call with the prefixed name.
func buildNativeMCPServer(server *mcpserver.MCPServer, group *model.MCPServiceGroup) (*mcpserver.MCPServer, error) {
	delimiter := getNativeDelimiter()
	registerDirectTools(server, group, delimiter)
	return server, nil
}

func registerDirectTools(server *mcpserver.MCPServer, group *model.MCPServiceGroup, delimiter string) {
	serviceIDs := group.GetServiceIDs()
	toolsCacheMgr := proxy.GetToolsCacheManager()

	for _, svcID := range serviceIDs {
		svc, err := model.GetServiceByID(svcID)
		if err != nil || !svc.Enabled {
			continue
		}

		entry, ok := toolsCacheMgr.GetServiceTools(svcID)
		if !ok || len(entry.Tools) == 0 {
			continue
		}

		for _, tool := range entry.Tools {
			prefixedName := svc.Name + delimiter + tool.Name
			directTool := mcp.Tool{
				Name:        prefixedName,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
			}

			currentSvc := svc
			currentToolName := tool.Name
			server.AddTool(directTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				arguments := common.ParseAnyToMap(request.Params.Arguments)
				if arguments == nil {
					arguments = map[string]any{}
				}
				result, err := callServiceTool(ctx, currentSvc, currentToolName, arguments, group.Name)
				if err != nil {
					return toolErrorResult(err), nil
				}
				return toolResultFromStructured(result), nil
			})
		}
	}
}

func getNativeDelimiter() string {
	delimiter := common.OptionMap[common.OptionGroupNativeToolDelimiter]
	if delimiter == "" {
		delimiter = "."
	}
	return delimiter
}
