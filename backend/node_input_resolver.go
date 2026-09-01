package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ResolvedImageInput struct {
	SourceNodeID string `json:"source_node_id"`
	SourceType   string `json:"source_type"`
	AssetID      string `json:"asset_id,omitempty"`
	Name         string `json:"name,omitempty"`
	URL          string `json:"url"`
}

type ResolvedTextInput struct {
	SourceNodeID string `json:"source_node_id"`
	Text         string `json:"text"`
}

type ResolvedUpstreamNode struct {
	ID       string `json:"id"`
	NodeType string `json:"node_type"`
}

type ResolvedNodeInputs struct {
	Images        []ResolvedImageInput   `json:"images"`
	Texts         []ResolvedTextInput    `json:"texts"`
	Assets        []string               `json:"assets"`
	UpstreamNodes []ResolvedUpstreamNode `json:"upstream_nodes"`
	IncomingEdges []Edge                 `json:"incoming_edges"`
}

func (h *Handler) resolveNodeInputs(canvasID, nodeID string) (ResolvedNodeInputs, error) {
	result := ResolvedNodeInputs{
		Images: []ResolvedImageInput{}, Texts: []ResolvedTextInput{}, Assets: []string{},
		UpstreamNodes: []ResolvedUpstreamNode{}, IncomingEdges: []Edge{},
	}
	if strings.TrimSpace(canvasID) == "" || strings.TrimSpace(nodeID) == "" {
		return result, nil
	}
	if h.db == nil {
		return result, errors.New("数据库未初始化")
	}

	var targetExists string
	if err := h.db.QueryRow(`SELECT id FROM nodes WHERE id = ? AND canvas_id = ?`, nodeID, canvasID).Scan(&targetExists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, errors.New("目标节点不存在或不属于当前画布")
		}
		return result, fmt.Errorf("查询目标节点: %w", err)
	}

	rows, err := h.db.Query(`
		SELECT e.id, e.canvas_id, e.source_node_id, e.target_node_id, e.created_at,
		       n.node_type, n.content
		FROM edges e
		JOIN nodes n ON n.id = e.source_node_id AND n.canvas_id = e.canvas_id
		WHERE e.canvas_id = ? AND e.target_node_id = ?
		ORDER BY e.created_at, e.id`, canvasID, nodeID)
	if err != nil {
		return result, fmt.Errorf("查询节点输入: %w", err)
	}
	defer rows.Close()

	seenImages := map[string]bool{}
	seenAssets := map[string]bool{}
	for rows.Next() {
		var edge Edge
		var nodeType, content string
		if err := rows.Scan(&edge.ID, &edge.CanvasID, &edge.SourceNodeID, &edge.TargetNodeID, &edge.CreatedAt, &nodeType, &content); err != nil {
			return result, fmt.Errorf("读取节点输入: %w", err)
		}
		result.IncomingEdges = append(result.IncomingEdges, edge)
		result.UpstreamNodes = append(result.UpstreamNodes, ResolvedUpstreamNode{ID: edge.SourceNodeID, NodeType: nodeType})

		images, text := extractNodeOutput(edge.SourceNodeID, nodeType, content)
		if text != "" {
			result.Texts = append(result.Texts, ResolvedTextInput{SourceNodeID: edge.SourceNodeID, Text: text})
		}
		for _, image := range images {
			if image.URL == "" || seenImages[image.URL] {
				continue
			}
			seenImages[image.URL] = true
			result.Images = append(result.Images, image)
			if image.AssetID != "" && !seenAssets[image.AssetID] {
				seenAssets[image.AssetID] = true
				result.Assets = append(result.Assets, image.AssetID)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("遍历节点输入: %w", err)
	}
	return result, nil
}

func (h *Handler) resolveSelectedNodeImages(canvasID string, nodeIDs []string) ([]ResolvedImageInput, error) {
	if strings.TrimSpace(canvasID) == "" || len(nodeIDs) == 0 {
		return []ResolvedImageInput{}, nil
	}
	if len(nodeIDs) > maxReferenceImages {
		return nil, fmt.Errorf("选中图片节点最多支持 %d 个", maxReferenceImages)
	}
	images := []ResolvedImageInput{}
	seen := map[string]bool{}
	for _, nodeID := range nodeIDs {
		var nodeType, content string
		if err := h.db.QueryRow(`SELECT node_type, content FROM nodes WHERE id = ? AND canvas_id = ?`, nodeID, canvasID).Scan(&nodeType, &content); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("选中节点 %s 不存在或不属于当前画布", nodeID)
			}
			return nil, fmt.Errorf("查询选中节点: %w", err)
		}
		outputs, _ := extractNodeOutput(nodeID, nodeType, content)
		for _, output := range outputs {
			if output.URL != "" && !seen[output.URL] {
				seen[output.URL] = true
				images = append(images, output)
			}
		}
	}
	return images, nil
}

func extractNodeOutput(nodeID, nodeType, content string) ([]ResolvedImageInput, string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ""
	}
	if nodeType == "text" {
		return nil, content
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil, ""
	}
	if nodeType == "asset" {
		urlValue, _ := data["url"].(string)
		assetID, _ := data["asset_id"].(string)
		name, _ := data["name"].(string)
		if strings.TrimSpace(urlValue) == "" {
			return nil, ""
		}
		return []ResolvedImageInput{{SourceNodeID: nodeID, SourceType: nodeType, AssetID: assetID, Name: name, URL: urlValue}}, ""
	}
	if nodeType != "image" {
		return nil, ""
	}

	outputs, _ := data["generated_images"].([]any)
	images := make([]ResolvedImageInput, 0, len(outputs))
	for _, output := range outputs {
		item, ok := output.(map[string]any)
		if !ok {
			continue
		}
		urlValue, _ := item["url"].(string)
		if strings.TrimSpace(urlValue) == "" {
			continue
		}
		assetID, _ := item["asset_id"].(string)
		name, _ := item["name"].(string)
		images = append(images, ResolvedImageInput{
			SourceNodeID: nodeID, SourceType: nodeType, AssetID: assetID, Name: name, URL: urlValue,
		})
	}
	return images, ""
}

func mergeReferenceImages(resolved []ResolvedImageInput, explicit []string) []string {
	result := make([]string, 0, len(resolved)+len(explicit))
	seen := map[string]bool{}
	for _, image := range resolved {
		value := strings.TrimSpace(image.URL)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	for _, reference := range explicit {
		value := strings.TrimSpace(reference)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
