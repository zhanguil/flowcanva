package main

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) CreateDevTestCanvas(c *gin.Context) {
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建测试画布失败"})
		return
	}
	defer tx.Rollback()

	suffix := uuid.New().String()[:8]
	canvasID := "cv_test_" + suffix
	if _, err := tx.Exec(`INSERT INTO canvases (id, name, project_type) VALUES (?, ?, 'canvas')`, canvasID, "V0.1 数据流回归测试"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建测试画布失败"})
		return
	}

	placeholder := mockPlaceholderImage()
	dataURL := "data:" + placeholder.MIMEType + ";base64," + placeholder.Data
	productContent, _ := json.Marshal(map[string]any{
		"asset_id": "dev_product_a", "name": "Product A", "url": dataURL,
	})
	referenceContent, _ := json.Marshal(map[string]any{
		"asset_id": "dev_reference_d", "name": "Reference D", "url": dataURL,
	})
	generationB, _ := json.Marshal(map[string]any{
		"prompt": "保持 Product A 产品结构不变，生成第一轮测试图", "images": []any{}, "generated_images": []any{},
	})
	generationC, _ := json.Marshal(map[string]any{
		"prompt": "使用 Generation B 最新输出与 Reference D 生成第二轮测试图", "images": []any{}, "generated_images": []any{},
	})

	nodes := []struct {
		id, nodeType, content string
		x, y, width, height   float64
	}{
		{"node_product_a_" + suffix, "asset", string(productContent), 120, 160, 260, 240},
		{"node_generation_b_" + suffix, "image", string(generationB), 520, 150, 400, 300},
		{"node_generation_c_" + suffix, "image", string(generationC), 1040, 280, 400, 300},
		{"node_reference_d_" + suffix, "asset", string(referenceContent), 560, 560, 260, 240},
	}
	for _, node := range nodes {
		if _, err := tx.Exec(`INSERT INTO nodes (id, canvas_id, node_type, x, y, width, height, content, config) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}')`,
			node.id, canvasID, node.nodeType, node.x, node.y, node.width, node.height, node.content); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建测试节点失败"})
			return
		}
	}

	edges := []struct{ source, target string }{
		{nodes[0].id, nodes[1].id},
		{nodes[1].id, nodes[2].id},
		{nodes[3].id, nodes[2].id},
	}
	for _, edge := range edges {
		if _, err := tx.Exec(`INSERT INTO edges (id, canvas_id, source_node_id, target_node_id) VALUES (?, ?, ?, ?)`,
			"ed_test_"+uuid.New().String()[:8], canvasID, edge.source, edge.target); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建测试连线失败"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存测试画布失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"canvas_id": canvasID,
		"nodes":     gin.H{"product_a": nodes[0].id, "generation_b": nodes[1].id, "generation_c": nodes[2].id, "reference_d": nodes[3].id},
	})
}
