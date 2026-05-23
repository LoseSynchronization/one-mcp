package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"one-mcp/backend/common"
	"one-mcp/backend/library/proxy"
	"one-mcp/backend/model"

	"log"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// ---------- Handler caching ----------

type groupMCPHandlerEntry struct {
	handler     http.Handler
	fingerprint string
}

var (
	groupMCPHandlers   = map[string]*groupMCPHandlerEntry{}
	groupMCPHandlersMu sync.RWMutex
)

func getOrCreateGroupMCPHandler(group *model.MCPServiceGroup, userID int64) (http.Handler, error) {
	cacheKey := groupHandlerCacheKey(group.ID, userID)
	fingerprint := groupHandlerFingerprint(group)

	groupMCPHandlersMu.RLock()
	if entry, ok := groupMCPHandlers[cacheKey]; ok && entry.fingerprint == fingerprint {
		groupMCPHandlersMu.RUnlock()
		return entry.handler, nil
	}
	groupMCPHandlersMu.RUnlock()

	handler, err := buildGroupMCPHandler(group)
	if err != nil {
		return nil, err
	}

	groupMCPHandlersMu.Lock()
	groupMCPHandlers[cacheKey] = &groupMCPHandlerEntry{
		handler:     handler,
		fingerprint: fingerprint,
	}
	groupMCPHandlersMu.Unlock()

	return handler, nil
}

func groupHandlerCacheKey(groupID int64, userID int64) string {
	return fmt.Sprintf("group-%d-user-%d", groupID, userID)
}

func groupHandlerFingerprint(group *model.MCPServiceGroup) string {
	return fmt.Sprintf("%s|%s|%s|%s", group.Name, group.Description, group.ServiceIDsJSON, group.Mode)
}

// ---------- Builder dispatch ----------

func buildGroupMCPHandler(group *model.MCPServiceGroup) (http.Handler, error) {
	server, err := buildGroupMCPServer(group)
	if err != nil {
		return nil, err
	}

	streamable := mcpserver.NewStreamableHTTPServer(server,
		mcpserver.WithHeartbeatInterval(30*time.Second),
	)

	return streamable, nil
}

func buildGroupMCPServer(group *model.MCPServiceGroup) (*mcpserver.MCPServer, error) {
	serverName := fmt.Sprintf("one-mcp-group-%s", group.Name)
	serverOptions := []mcpserver.ServerOption{}
	if strings.TrimSpace(group.Description) != "" {
		serverOptions = append(serverOptions, mcpserver.WithInstructions(group.Description))
	}

	server := mcpserver.NewMCPServer(serverName, "1.0.0", serverOptions...)

	if group.Mode == common.GroupModeNative {
		return buildNativeMCPServer(server, group)
	}

	// Default: wrapped mode
	return buildWrappedMCPServer(server, group)
}

// ---------- Shared tool execution ----------

// contextKey for storing per-request values in the MCP context.
type contextKey string

const (
	clientNameKey contextKey = "client_name"
	userIDKey     contextKey = "user_id"
)

// callServiceTool handles the common execution flow: RPD check → get instance → CallTool → record stats → log.
// Returns the upstream response as a map ready for toolResultFromStructured.
func callServiceTool(ctx context.Context, svc *model.MCPService, toolName string, arguments map[string]any, groupName string) (any, error) {
	start := time.Now()

	// Get userID from context for RPD check and stats
	var userID int64
	if uid, ok := ctx.Value(userIDKey).(int64); ok {
		userID = uid
	}

	// Check daily request limit (RPD) if limit is set
	if userID > 0 && svc.RPDLimit > 0 {
		if rpdErr := checkDailyRequestLimit(svc.ID, userID, svc.RPDLimit); rpdErr != nil {
			return nil, rpdErr
		}
	}

	sharedInst, err := proxy.GetOrCreateSharedMcpInstanceWithKey(ctx, svc, proxy.SharedServiceCacheKey(svc.ID), proxy.SharedServiceInstanceName(svc.ID), svc.DefaultEnvsJSON)
	if err != nil {
		return nil, err
	}

	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = toolName
	callReq.Params.Arguments = arguments

	toolCallCtx, cancel := context.WithTimeout(ctx, proxy.McpToolCallTimeout())
	defer cancel()

	result, err := sharedInst.Client.CallTool(toolCallCtx, callReq)
	duration := time.Since(start)

	// Get client name from context
	clientName := ""
	if cn, ok := ctx.Value(clientNameKey).(string); ok {
		clientName = cn
	}

	// Determine success: no error AND result.IsError is false
	success := err == nil && (result == nil || !result.IsError)

	// Only record stats for successful calls (not errors or isError responses)
	if success {
		go model.RecordRequestStat(
			svc.ID,
			svc.Name,
			userID,
			model.ProxyRequestTypeHTTP,
			"tools/call",
			fmt.Sprintf("/group/%s/mcp", groupName),
			duration.Milliseconds(),
			200,
			true,
		)
	}

	// Log the execution
	logLevel := model.MCPLogLevelInfo
	logMsg := fmt.Sprintf("Group tool call OK | group=%s | mcp=%s | tool=%s | duration=%dms | client=%s",
		groupName, svc.Name, toolName, duration.Milliseconds(), clientName)
	if err != nil {
		logLevel = model.MCPLogLevelError
		logMsg = fmt.Sprintf("Group tool call FAILED | group=%s | mcp=%s | tool=%s | duration=%dms | client=%s | error=%v",
			groupName, svc.Name, toolName, duration.Milliseconds(), clientName, err)
	} else if result != nil && result.IsError {
		logLevel = model.MCPLogLevelError
		logMsg = fmt.Sprintf("Group tool call ERROR | group=%s | mcp=%s | tool=%s | duration=%dms | client=%s | isError=true",
			groupName, svc.Name, toolName, duration.Milliseconds(), clientName)
	}
	if saveErr := model.SaveMCPLog(ctx, svc.ID, svc.Name, model.MCPLogPhaseRun, logLevel, logMsg); saveErr != nil {
		common.SysError(fmt.Sprintf("Failed to save MCP log for %s: %v", svc.Name, saveErr))
	}

	if err != nil {
		return nil, err
	}

	resp := map[string]any{}
	if result != nil && len(result.Content) > 0 {
		resp["content"] = result.Content
	}
	if result != nil && result.StructuredContent != nil {
		resp["structuredContent"] = result.StructuredContent
	}
	if result != nil && result.IsError {
		resp["isError"] = true
	}

	return resp, nil
}

// ClearGroupHandlerCaches clears all group MCP handler cache entries.
// Handlers are rebuilt on the next request. Call this after tool caches are refreshed
// so native-mode groups pick up the updated tool lists.
func ClearGroupHandlerCaches() {
	groupMCPHandlersMu.Lock()
	defer groupMCPHandlersMu.Unlock()
	cleared := len(groupMCPHandlers)
	groupMCPHandlers = make(map[string]*groupMCPHandlerEntry)
	if cleared > 0 {
		log.Printf("[GroupCache] Cleared %d group handler cache entries", cleared)
	}
}

// ---------- Result helpers ----------

func toolErrorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.TextContent{
				Type: mcp.ContentTypeText,
				Text: err.Error(),
			},
		},
	}
}

func toolResultFromStructured(result any) *mcp.CallToolResult {
	resultMap, _ := result.(map[string]any)

	contents := extractContent(resultMap)

	callResult := &mcp.CallToolResult{
		Content: contents,
	}
	if resultMap != nil {
		if sc, exists := resultMap["structuredContent"]; exists {
			if scMap, ok := sc.(map[string]any); ok {
				callResult.StructuredContent = scMap
			} else {
				callResult.StructuredContent = map[string]any{}
			}
		}
	}
	return callResult
}

func extractContent(result map[string]any) []mcp.Content {
	if result == nil {
		return nil
	}
	rawContent, ok := result["content"]
	if !ok || rawContent == nil {
		return nil
	}
	switch content := rawContent.(type) {
	case []mcp.Content:
		return content
	case []map[string]any:
		contents := make([]mcp.Content, 0, len(content))
		for _, item := range content {
			itemBytes, err := json.Marshal(item)
			if err != nil {
				continue
			}
			parsed, err := mcp.UnmarshalContent(itemBytes)
			if err != nil {
				continue
			}
			contents = append(contents, parsed)
		}
		return contents
	case []any:
		contents := make([]mcp.Content, 0, len(content))
		for _, item := range content {
			itemBytes, err := json.Marshal(item)
			if err != nil {
				continue
			}
			parsed, err := mcp.UnmarshalContent(itemBytes)
			if err != nil {
				continue
			}
			contents = append(contents, parsed)
		}
		return contents
	default:
		return nil
	}
}

