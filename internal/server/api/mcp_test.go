package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMCPInitialize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.MCP(c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize response = %d %s", w.Code, w.Body.String())
	}
}

func TestMCPRejectsCrossOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "http://localhost:8080/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	c.Request.Header.Set("Origin", "https://evil.example")
	h.MCP(c)
	if w.Code != 403 {
		t.Fatalf("cross-origin status = %d, want 403", w.Code)
	}
}

func TestMCPToolsListIsReadOnly(t *testing.T) {
	tools := mcpTools()
	if len(tools) == 0 {
		t.Fatal("expected MCP tools")
	}
	for _, tool := range tools {
		annotations, ok := tool["annotations"].(map[string]any)
		if !ok || annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false {
			t.Fatalf("tool %q is not marked read-only: %#v", tool["name"], tool["annotations"])
		}
	}
}
