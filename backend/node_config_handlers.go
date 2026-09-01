package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListNodeConfigs(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id, node_type, model_name, api_channel, base_url, api_key, parameters, prompt_template, extra_config, enabled, created_at, updated_at FROM node_configs ORDER BY node_type`)
	if err != nil {
		h.log.Error("list node configs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	configs := []NodeConfig{}
	for rows.Next() {
		var nc NodeConfig
		if err := rows.Scan(&nc.ID, &nc.NodeType, &nc.ModelName, &nc.APIChannel, &nc.BaseURL, &nc.APIKey, &nc.Parameters, &nc.PromptTemplate, &nc.ExtraConfig, &nc.Enabled, &nc.CreatedAt, &nc.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		configs = append(configs, sanitizeNodeConfigForBrowser(nc))
	}
	c.JSON(http.StatusOK, configs)
}

func (h *Handler) GetNodeConfig(c *gin.Context) {
	nodeType := c.Param("type")
	var nc NodeConfig
	err := h.db.QueryRow(`SELECT id, node_type, model_name, api_channel, base_url, api_key, parameters, prompt_template, extra_config, enabled, created_at, updated_at FROM node_configs WHERE node_type = ?`, nodeType).
		Scan(&nc.ID, &nc.NodeType, &nc.ModelName, &nc.APIChannel, &nc.BaseURL, &nc.APIKey, &nc.Parameters, &nc.PromptTemplate, &nc.ExtraConfig, &nc.Enabled, &nc.CreatedAt, &nc.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "node config not found"})
		return
	}
	if err != nil {
		h.log.Error("get node config", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sanitizeNodeConfigForBrowser(nc))
}

func (h *Handler) UpdateNodeConfig(c *gin.Context) {
	nodeType := c.Param("type")
	var body struct {
		ModelName      *string `json:"model_name"`
		APIChannel     *string `json:"api_channel"`
		BaseURL        *string `json:"base_url"`
		Parameters     *string `json:"parameters"`
		PromptTemplate *string `json:"prompt_template"`
		ExtraConfig    *string `json:"extra_config"`
		Enabled        *int    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var nc NodeConfig
	err := h.db.QueryRow(`SELECT id, node_type, model_name, api_channel, base_url, api_key, parameters, prompt_template, extra_config, enabled, created_at, updated_at FROM node_configs WHERE node_type = ?`, nodeType).
		Scan(&nc.ID, &nc.NodeType, &nc.ModelName, &nc.APIChannel, &nc.BaseURL, &nc.APIKey, &nc.Parameters, &nc.PromptTemplate, &nc.ExtraConfig, &nc.Enabled, &nc.CreatedAt, &nc.UpdatedAt)
	if err != nil {
		h.log.Error("update node config", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if body.ModelName != nil {
		nc.ModelName = *body.ModelName
	}
	if body.APIChannel != nil {
		nc.APIChannel = *body.APIChannel
	}
	if body.BaseURL != nil {
		nc.BaseURL = *body.BaseURL
	}
	if body.Parameters != nil {
		nc.Parameters = *body.Parameters
	}
	if body.PromptTemplate != nil {
		nc.PromptTemplate = *body.PromptTemplate
	}
	if body.ExtraConfig != nil {
		nc.ExtraConfig = stripBrowserAPIKeys(*body.ExtraConfig)
	}
	if body.Enabled != nil {
		nc.Enabled = *body.Enabled
	}

	_, err = h.db.Exec(`UPDATE node_configs SET model_name=?, api_channel=?, base_url=?, api_key=?, parameters=?, prompt_template=?, extra_config=?, enabled=?, updated_at=datetime('now','localtime') WHERE node_type=?`,
		nc.ModelName, nc.APIChannel, nc.BaseURL, nc.APIKey, nc.Parameters, nc.PromptTemplate, nc.ExtraConfig, nc.Enabled, nodeType)
	if err != nil {
		h.log.Error("update node config", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.log.Info("node config updated", "type", nodeType)
	c.JSON(http.StatusOK, sanitizeNodeConfigForBrowser(nc))
}

func sanitizeNodeConfigForBrowser(config NodeConfig) NodeConfig {
	config.APIKey = ""
	config.ExtraConfig = stripBrowserAPIKeys(config.ExtraConfig)
	return config
}

func stripBrowserAPIKeys(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "{}"
	}
	removeBrowserAPIKeys(value)
	clean, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(clean)
}

func removeBrowserAPIKeys(value any) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			if normalized == "apikey" || normalized == "vectorengineapikey" {
				delete(item, key)
				continue
			}
			removeBrowserAPIKeys(child)
		}
	case []any:
		for _, child := range item {
			removeBrowserAPIKeys(child)
		}
	}
}
