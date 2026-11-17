package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_ListsServe(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "serve") {
		t.Fatalf("help should list serve, got:\n%s", out.String())
	}
}

func TestServeCmd_RejectsArgs(t *testing.T) {
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"serve", "extra"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unexpected arg")
	}
}
