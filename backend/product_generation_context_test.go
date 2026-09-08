package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductContextAdapters(t *testing.T) {
	for _, profile := range []string{"fast", "edit"} {
		t.Run(profile, func(t *testing.T) {
			f := newDataFlowFixture(t)
			f.node("B", "image", `{}`)
			encoded := base64.StdEncoding.EncodeToString(testPNG(t))
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var prompt string
				if profile == "fast" {
					var payload geminiGenerateRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
						return
					}
					if len(payload.Contents[0].Parts) < 2 {
						t.Error("missing reference image")
					}
					for _, part := range payload.Contents[0].Parts {
						prompt += part.Text
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"inlineData": map[string]string{"mimeType": "image/png", "data": encoded}}}}}}})
				} else {
					if err := r.ParseMultipartForm(2 << 20); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					prompt = r.FormValue("prompt")
					if len(r.MultipartForm.File["image"]) != 1 {
						t.Error("missing reference file")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": encoded}}})
				}
				if !strings.Contains(prompt, "角色=hardware") || !strings.Contains(prompt, "hardwareLock") {
					t.Error("adapter lost role or lock")
				}
			}))
			defer upstream.Close()
			f.h.imageProvider = nil
			f.h.vectorEngine = NewVectorEngineProvider(upstream.URL, "test-only", upstream.Client())
			constraints := ProductConstraint{HardwareLock: true}
			ctx := &GenerationContext{SchemaVersion: 1, Prompt: "保持五金位置", Constraints: constraints,
				Product:       &ProductAssetContext{ID: "p", Name: "电视柜", Constraints: constraints},
				References:    []ProductReference{{ID: "r", URL: "data:image/png;base64," + encoded, Role: "hardware"}},
				OutputOptions: ProductOutputOptions{Model: profile, Count: 1, AspectRatio: "3:4", ImageSize: "1K", GenerationType: "detail"},
			}
			body, _ := json.Marshal(VectorImageRequest{TaskID: "g", CanvasID: f.cid, NodeID: "B", GenerationContext: ctx, Lineage: &GenerationLineage{ID: "g"}})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/json")
			f.router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"rootProductAssetId":"p"`) {
				t.Fatal("adapter output missing product")
			}
		})
	}
}

func TestProductContextReachesProviderAndPersistedOutputs(t *testing.T) {
	f := newDataFlowFixture(t)
	f.node("B", "image", `{}`)
	locks := ProductConstraint{StructureLock: true, MaterialLock: true, TextureLock: true, ProportionLock: true, HardwareLock: true,
		LockedFields: map[string]any{"drawerCount": 4, "width": 2000, "height": 250, "depth": 400, "panelThickness": 20, "materials": []string{"中古胡桃", "罗马洞石", "亮光黑"}},
		CustomRules:  []string{"禁止改变木纹方向、五金位置、插座结构"},
	}
	product := &ProductAssetContext{ID: "product_yanyu", Name: "洞石砚屿电视柜", Constraints: locks, ReferenceIDs: []string{"white"}}
	ctx := &GenerationContext{SchemaVersion: 1, Product: product, Constraints: locks, Prompt: "产品细节",
		References:    []ProductReference{{ID: "white", URL: "/uploads/white.png", Role: "product"}, {ID: "angle", URL: "/uploads/angle.png", Role: "composition", GenerationID: "g2"}},
		OutputOptions: ProductOutputOptions{Model: "fast", AspectRatio: "3:4", ImageSize: "1K", Count: 1, GenerationType: "detail"},
	}
	parent := "g2"
	req := VectorImageRequest{TaskID: "g3", CanvasID: f.cid, NodeID: "B", GenerationContext: ctx, Lineage: &GenerationLineage{ID: "g3", ParentGenerationID: &parent, ParentGenerationIDs: []string{parent}}}
	body, _ := json.Marshal(req)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/images/generate", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	provider := f.mock.Requests()[0]
	if provider.GenerationContext.Product.ID != product.ID || provider.GenerationContext.References[1].Role != "composition" {
		t.Fatal("provider lost product or reference role")
	}
	for _, expected := range []string{"洞石砚屿电视柜", "drawerCount", "2000", "materialLock", "composition", "五金位置"} {
		if !strings.Contains(provider.Prompt, expected) {
			t.Fatalf("provider prompt missing %s", expected)
		}
	}
	var output struct {
		Data []GeneratedAsset `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	gen := output.Data[0].Generation
	if gen == nil || *gen.RootProductAssetID != product.ID || *gen.ParentGenerationID != parent {
		t.Fatal("output lost lineage")
	}
	var stored struct {
		Outputs []struct {
			Generation *ProductGenerationRecord `json:"generation"`
		} `json:"generated_images"`
	}
	if err := json.Unmarshal([]byte(nodeContent(t, f.db, "B")), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Outputs[0].Generation.Context.Constraints.LockedFields["drawerCount"] != float64(4) {
		t.Fatal("persisted output lost drawer lock")
	}
}

func TestRejectInvalidProductContext(t *testing.T) {
	ctx := &GenerationContext{SchemaVersion: 1, Product: &ProductAssetContext{ID: "p", Name: "电视柜", Constraints: ProductConstraint{StructureLock: true}}}
	req := VectorImageRequest{TaskID: "g1", GenerationContext: ctx, Lineage: &GenerationLineage{ID: "g1"}}
	if err := applyGenerationContext(&req); err == nil {
		t.Fatal("mismatched product locks accepted")
	}
	ctx.Product = nil
	ctx.Prompt = "test"
	ctx.OutputOptions = ProductOutputOptions{Count: 1, GenerationType: "scene", AspectRatio: "3:4"}
	ctx.References = []ProductReference{{ID: "r", URL: "/a.png", Role: "invalid"}}
	if err := applyGenerationContext(&req); err == nil {
		t.Fatal("invalid reference role accepted")
	}
}
