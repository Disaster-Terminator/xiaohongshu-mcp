package main

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus/hooks/test"
	"testing"
)

func TestPartialNoteIsNotReportedAsCompleteSuccess(t *testing.T) {
	hook := test.NewGlobal()
	defer hook.Reset()
	wire := convertToMCPResult(&MCPToolResult{Content: []MCPContent{{Type: "text", Text: `{"partial":true,"data":{"note":{"noteId":"fixture"},"comments":null,"comments_status":"unavailable"}}`}}, Meta: map[string]any{"partial": true, "error_kind": "detail_comments_not_ready"}})
	handler := withPanicRecovery("get_feed_detail", func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return wire, nil, nil
	})
	result, _, err := handler(context.Background(), nil, struct{}{})
	if err != nil || result.IsError {
		t.Fatal("available note must remain consumable")
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil {
		t.Fatal("invalid MCP result")
	}
	meta, ok := decoded["_meta"].(map[string]any)
	if !ok || meta["partial"] != true {
		t.Fatal("partial metadata lost in SDK serialization")
	}
	last := hook.LastEntry()
	if last.Data["outcome"] != "partial" || last.Data["error_kind"] != "detail_comments_not_ready" {
		t.Fatalf("partial read incorrectly logged: %v", last.Data)
	}
}
