package main

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
	"testing"
)

func TestStressContractErrorsBeforeBrowserAdmission(t *testing.T) {
	service := NewXiaohongshuService()
	_, err := service.SearchFeeds(context.Background(), " \t\n")
	require.ErrorContains(t, err, "[invalid_argument]")
	_, err = service.GetFeedDetailWithConfig(context.Background(), "fixture", "token", true, xiaohongshu.CommentLoadConfig{ScrollSpeed: "warp"})
	require.ErrorContains(t, err, "[invalid_argument]")
	require.Empty(t, browserSlots().slots)
}
func TestStressContractErrorMetadataKeepsExecutionFailureDistinct(t *testing.T) {
	handler := withPanicRecovery("fixture", func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		panic(context.DeadlineExceeded)
	})
	result, _, err := handler(context.Background(), nil, struct{}{})
	require.NoError(t, err, "execution failure must not be a JSON-RPC invalid argument")
	require.True(t, result.IsError)
	require.Equal(t, "deadline", result.Meta["error_kind"])
	for _, kind := range []string{"login_required", "profile_access_denied", "profile_tab_unavailable", "invalid_argument"} {
		require.Equal(t, kind, toolFailureKind("["+kind+"] fixture"))
	}
}
