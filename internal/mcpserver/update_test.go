package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/ps2build/ps2build/internal/versions"
)

func TestExitOnUpdateCancelsWhenPS2BuildTagChanges(t *testing.T) {
	root := t.TempDir()
	if err := versions.SaveComponent(root, "ps2build", "v1"); err != nil {
		t.Fatal(err)
	}
	ctx := exitOnUpdate(context.Background(), root)
	if err := versions.SaveComponent(root, "ps2build", "v2"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(updateCheckInterval + 3*time.Second):
		t.Fatal("context not cancelled after the installed tag changed")
	}
}
