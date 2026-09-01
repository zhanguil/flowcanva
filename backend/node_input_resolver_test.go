package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type dataFlowFixture struct {
	t      *testing.T
	db     *sql.DB
	h      *Handler
	mock   *MockImageProvider
	router *gin.Engine
	cid    string
}

func newDataFlowFixture(t *testing.T) *dataFlowFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tempDir := t.TempDir()
	db, err := initDB(filepath.Join(tempDir, "data.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO canvases (id, name) VALUES ('cv_flow', 'flow test')`); err != nil {
		t.Fatal(err)
	}
	mock := NewMockImageProvider()
	h := &Handler{
		db: db, log: log, imageProvider: mock, devMode: true,
		generationDebug: &GenerationDebugStore{}, imageModelFast: "server-fast-model",
		uploadDir: filepath.Join(tempDir, "uploads"),
	}
	router := gin.New()
	router.POST("/api/images/generate", h.GenerateImage)
	return &dataFlowFixture{t: t, db: db, h: h, mock: mock, router: router, cid: "cv_flow"}
}

func (f *dataFlowFixture) node(id, nodeType, content string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO nodes (id, canvas_id, node_type, content) VALUES (?, ?, ?, ?)`, id, f.cid, nodeType, content); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dataFlowFixture) edge(id, source, target string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO edges (id, canvas_id, source_node_id, target_node_id) VALUES (?, ?, ?, ?)`, id, f.cid, source, target); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dataFlowFixture) generate(nodeID string) ImageProviderRequest {
	f.t.Helper()
	body, _ := json.Marshal(VectorImageRequest{CanvasID: f.cid, NodeID: nodeID, Prompt: "regression", Profile: "fast", N: 1})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		f.t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	requests := f.mock.Requests()
	if len(requests) == 0 {
		f.t.Fatal("mock provider did not record a request")
	}
	return requests[len(requests)-1]
}

func assetContent(id, url string) string {
	value, _ := json.Marshal(map[string]any{"asset_id": id, "name": id, "url": url})
	return string(value)
}

func imageOutputContent(urls ...string) string {
	outputs := make([]map[string]any, 0, len(urls))
	for index, url := range urls {
		outputs = append(outputs, map[string]any{"asset_id": "generated_" + string(rune('a'+index)), "url": url})
	}
	value, _ := json.Marshal(map[string]any{"prompt": "test", "generated_images": outputs})
	return string(value)
}

func assertReferences(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("references=%v want=%v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("references=%v want=%v", got, want)
		}
	}
}

func TestGenerationDataFlowRegression(t *testing.T) {
	t.Run("Test 1 product asset flows into generation", func(t *testing.T) {
		f := newDataFlowFixture(t)
		f.node("A", "asset", assetContent("asset_a", "/uploads/a.png"))
		f.node("B", "image", imageOutputContent())
		f.edge("A_B", "A", "B")
		assertReferences(t, f.generate("B").ReferenceImages, "/uploads/a.png")
	})

	t.Run("Test 2 generated output flows into next generation", func(t *testing.T) {
		f := newDataFlowFixture(t)
		f.node("A", "asset", assetContent("asset_a", "/uploads/a.png"))
		f.node("B", "image", imageOutputContent("/uploads/b1.png"))
		f.node("C", "image", imageOutputContent())
		f.edge("A_B", "A", "B")
		f.edge("B_C", "B", "C")
		assertReferences(t, f.generate("C").ReferenceImages, "/uploads/b1.png")
	})

	t.Run("Test 3 multiple incoming images are preserved", func(t *testing.T) {
		f := newDataFlowFixture(t)
		f.node("A", "asset", assetContent("asset_a", "/uploads/a.png"))
		f.node("B", "asset", assetContent("asset_b", "/uploads/b.png"))
		f.node("C", "image", imageOutputContent())
		f.edge("A_C", "A", "C")
		f.edge("B_C", "B", "C")
		assertReferences(t, f.generate("C").ReferenceImages, "/uploads/a.png", "/uploads/b.png")
	})

	t.Run("Test 4 deep chain uses immediate latest output", func(t *testing.T) {
		f := newDataFlowFixture(t)
		f.node("A", "asset", assetContent("asset_a", "/uploads/a.png"))
		f.node("B", "image", imageOutputContent("/uploads/b.png"))
		f.node("C", "image", imageOutputContent("/uploads/c.png"))
		f.node("D", "image", imageOutputContent())
		f.edge("A_B", "A", "B")
		f.edge("B_C", "B", "C")
		f.edge("C_D", "C", "D")
		assertReferences(t, f.generate("D").ReferenceImages, "/uploads/c.png")
	})

	t.Run("Test 5 deleted edge removes its reference", func(t *testing.T) {
		f := newDataFlowFixture(t)
		f.node("A", "asset", assetContent("asset_a", "/uploads/a.png"))
		f.node("B", "asset", assetContent("asset_b", "/uploads/b.png"))
		f.node("C", "image", imageOutputContent())
		f.edge("A_C", "A", "C")
		f.edge("B_C", "B", "C")
		if _, err := f.db.Exec(`DELETE FROM edges WHERE id = 'A_C'`); err != nil {
			t.Fatal(err)
		}
		assertReferences(t, f.generate("C").ReferenceImages, "/uploads/b.png")
	})

	t.Run("Test 6 regeneration replaces stale output", func(t *testing.T) {
		f := newDataFlowFixture(t)
		f.node("B", "image", imageOutputContent("/uploads/b1.png"))
		f.node("C", "image", imageOutputContent())
		f.edge("B_C", "B", "C")
		assertReferences(t, f.generate("C").ReferenceImages, "/uploads/b1.png")
		if _, err := f.db.Exec(`UPDATE nodes SET content = ? WHERE id = 'B'`, imageOutputContent("/uploads/b2.png")); err != nil {
			t.Fatal(err)
		}
		assertReferences(t, f.generate("C").ReferenceImages, "/uploads/b2.png")
	})
}
