package tools

import (
	"encoding/json"
	"errors"
	"testing"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

func TestRelPath(t *testing.T) {
	if got := RelPath("/foo/bar", "/foo/bar/baz/qux.go"); got != "baz/qux.go" {
		t.Fatalf("expected baz/qux.go, got %s", got)
	}
	if got := RelPath("/foo/bar", "/other/dir/file.go"); got != "../../other/dir/file.go" {
		t.Fatalf("expected ../../other/dir/file.go, got %q", got)
	}
	if got := RelPath("", "/abs/path.go"); got != "/abs/path.go" {
		t.Fatal("expected unchanged path without CWD")
	}
}

func TestWrapCodeBlock(t *testing.T) {
	tests := []struct {
		path, content, want string
	}{
		{"main.go", "x", "````go\nx\n````"},
		{"script.py", "y", "````py\ny\n````"},
		{"Makefile", "z", "````\nz\n````"},
		{".hidden", "a", "````hidden\na\n````"},
		{"noext", "b", "````\nb\n````"},
		{"", "c", "````\nc\n````"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := WrapCodeBlock(tt.path, tt.content)
			if got != tt.want {
				t.Fatalf("WrapCodeBlock(%q, %q) = %q, want %q", tt.path, tt.content, got, tt.want)
			}
		})
	}
}

func TestToolResponse(t *testing.T) {
	upd := acp.UpdateToolCallDelta("tc1", acp.WithRawOutput("ok"))
	resp := ToolResponse("done", upd)

	if resp.Type != "text" {
		t.Fatalf("expected text type, got %q", resp.Type)
	}
	if resp.Content != "done" {
		t.Fatalf("expected 'done', got %q", resp.Content)
	}
	if resp.IsError {
		t.Fatal("expected IsError=false")
	}

	var updates []acp.SessionUpdate
	if err := json.Unmarshal([]byte(resp.Metadata), &updates); err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
}

func TestToolErrorResponse(t *testing.T) {
	upd := acp.UpdateToolCallDelta("tc1", acp.WithRawOutput("fail"), acp.WithStatus(acp.ToolFailed))
	resp := ToolErrorResponse("oops", upd)

	if !resp.IsError {
		t.Fatal("expected IsError=true")
	}
	if resp.Content != "oops" {
		t.Fatalf("expected 'oops', got %q", resp.Content)
	}
}

func TestToolFailedResponse(t *testing.T) {
	tc := fantasy.ToolCall{ID: "tc1", Name: "read"}
	resp := ToolFailedResponse(tc, errors.New("file not found"))

	if !resp.IsError {
		t.Fatal("expected IsError=true")
	}
	if resp.Content != "file not found" {
		t.Fatalf("expected error message, got %q", resp.Content)
	}

	var updates []acp.SessionUpdate
	if err := json.Unmarshal([]byte(resp.Metadata), &updates); err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	tcu := updates[0].ToolCallUpdate
	if tcu == nil {
		t.Fatal("expected ToolCallUpdate")
	}
	if tcu.Status == nil || *tcu.Status != acp.ToolFailed {
		t.Fatal("expected ToolFailed status")
	}
}
