package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplicitReferencesDoNotRestoreRemovedImages(t *testing.T) {
	for _, references := range [][]string{{"/uploads/kept.png", "/uploads/kept.png"}, {}} {
		f := newDataFlowFixture(t)
		f.node("A", "image", imageOutputContent("/uploads/removed.png", "/uploads/kept.png"))
		f.node("B", "image", imageOutputContent())
		f.edge("A_B", "A", "B")
		body, _ := json.Marshal(VectorImageRequest{
			CanvasID: f.cid, NodeID: "B", Prompt: "furniture", AspectRatio: "3:4",
			ReferenceMode: "explicit", ReferenceImages: references,
		})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		f.router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		got := f.mock.Requests()[0]
		if len(references) == 0 {
			assertReferences(t, got.ReferenceImages)
		} else {
			assertReferences(t, got.ReferenceImages, "/uploads/kept.png")
		}
		if got.AspectRatio != "3:4" {
			t.Fatalf("ratio=%s", got.AspectRatio)
		}
	}
}
