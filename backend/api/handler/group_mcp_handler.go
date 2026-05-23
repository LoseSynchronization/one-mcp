package handler

import (
	"context"
	"net/http"

	"one-mcp/backend/common"
	"one-mcp/backend/model"

	"github.com/gin-gonic/gin"
)

// GroupMCPHandler is the HTTP entry point for all MCP group requests (/group/:name/mcp).
// It resolves the group, checks auth/enabled, then delegates to the cached MCP handler.
func GroupMCPHandler(c *gin.Context) {
	groupName := c.Param("name")
	userID := c.GetInt64("user_id")

	if userID == 0 {
		common.RespJSONRPCError(c, http.StatusUnauthorized, common.JSONRPCErrorCodeInvalidRequest,
			"Authentication failed: Invalid or expired API key. Please check your API key in Profile settings or refresh it if recently changed.")
		return
	}

	group, err := model.GetMCPServiceGroupByName(groupName, userID)
	if err != nil {
		common.RespJSONRPCError(c, http.StatusNotFound, common.JSONRPCErrorCodeInvalidRequest,
			"Group not found: "+err.Error())
		return
	}

	if !group.Enabled {
		common.RespJSONRPCError(c, http.StatusServiceUnavailable, common.JSONRPCErrorCodeInvalidRequest,
			"Group is disabled")
		return
	}

	handler, err := getOrCreateGroupMCPHandler(group, userID)
	if err != nil {
		common.RespJSONRPCError(c, http.StatusInternalServerError, common.JSONRPCErrorCodeInvalidRequest,
			"Failed to create MCP handler: "+err.Error())
		return
	}

	// Store client name and userID in request context for logging and RPD check
	clientName := c.Request.Header.Get("User-Agent")
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, clientNameKey, clientName)
	ctx = context.WithValue(ctx, userIDKey, userID)
	c.Request = c.Request.WithContext(ctx)

	handler.ServeHTTP(c.Writer, c.Request)
}
