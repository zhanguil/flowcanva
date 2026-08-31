package main

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestInitDBUsesSingleSQLiteWriter(t *testing.T) {
	db, err := initDB(filepath.Join(t.TempDir(), "canvas.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
}
