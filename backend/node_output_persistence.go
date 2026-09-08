package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (h *Handler) persistNodeGeneratedOutputs(canvasID, nodeID string, assets []GeneratedAsset) error {
	if strings.TrimSpace(canvasID) == "" || strings.TrimSpace(nodeID) == "" {
		return nil
	}
	if h.db == nil {
		return errors.New("数据库未初始化")
	}

	var content string
	if err := h.db.QueryRow(`SELECT content FROM nodes WHERE id = ? AND canvas_id = ? AND node_type = 'image'`, nodeID, canvasID).Scan(&content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("生图节点不存在或不属于当前画布")
		}
		return fmt.Errorf("读取生图节点输出: %w", err)
	}

	data := map[string]any{}
	if strings.TrimSpace(content) != "" {
		_ = json.Unmarshal([]byte(content), &data)
	}
	outputs := make([]map[string]any, 0, len(assets))
	outputIDs := make([]string, 0, len(assets))
	for _, asset := range assets {
		outputIDs = append(outputIDs, asset.ID)
		outputs = append(outputs, map[string]any{
			"id": asset.ID, "asset_id": asset.ID, "name": asset.Filename, "url": asset.URL,
			"size": asset.Size, "mime_type": asset.MimeType, "width": asset.Width, "height": asset.Height,
			"generation": asset.Generation,
		})
	}
	data["generated_images"] = outputs
	data["output"] = map[string]any{"generated_asset_ids": outputIDs}
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("编码生图节点输出: %w", err)
	}
	if _, err := h.db.Exec(`UPDATE nodes SET content = ?, updated_at = datetime('now','localtime') WHERE id = ? AND canvas_id = ?`, string(encoded), nodeID, canvasID); err != nil {
		return fmt.Errorf("保存生图节点输出: %w", err)
	}
	return nil
}
