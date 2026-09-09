package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) ListAssets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "24"))
	filters := []struct{ column, value string }{
		{"category", c.Query("category")}, {"project_id", c.Query("project_id")},
		{"sku_id", c.Query("sku_id")}, {"type", c.Query("type")},
		{"created_by", c.Query("created_by")}, {"source_type", c.Query("source_type")},
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 24
	}
	offset := (page - 1) * pageSize

	whereClause := " WHERE deleted_at = ''"
	args := []any{}
	for _, filter := range filters {
		if filter.value != "" {
			whereClause += " AND " + filter.column + " = ?"
			args = append(args, filter.value)
		}
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM assets" + whereClause
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		h.log.Error("list assets count", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	queryArgs := append(args, pageSize, offset)
	rows, err := h.db.Query(`SELECT id, filename, url, size, mime_type, width, height, category, tags,
		project_id, type, thumbnail_url, aspect_ratio, source_type, role, sku_id, parent_asset_id,
		created_by, generation_metadata, created_at FROM assets`+whereClause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		h.log.Error("list assets", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type Asset struct {
		ID                 string `json:"id"`
		Filename           string `json:"filename"`
		URL                string `json:"url"`
		Size               int64  `json:"size"`
		MimeType           string `json:"mime_type"`
		Width              int    `json:"width"`
		Height             int    `json:"height"`
		Category           string `json:"category"`
		Tags               string `json:"tags"`
		ProjectID          string `json:"project_id"`
		Type               string `json:"type"`
		ThumbnailURL       string `json:"thumbnail_url"`
		AspectRatio        string `json:"aspect_ratio"`
		SourceType         string `json:"source_type"`
		Role               string `json:"role"`
		SKUID              string `json:"sku_id"`
		ParentAssetID      string `json:"parent_asset_id"`
		CreatedBy          string `json:"created_by"`
		GenerationMetadata string `json:"generation_metadata"`
		CreatedAt          string `json:"created_at"`
	}

	items := []Asset{}
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.Filename, &a.URL, &a.Size, &a.MimeType, &a.Width, &a.Height, &a.Category, &a.Tags,
			&a.ProjectID, &a.Type, &a.ThumbnailURL, &a.AspectRatio, &a.SourceType, &a.Role, &a.SKUID,
			&a.ParentAssetID, &a.CreatedBy, &a.GenerationMetadata, &a.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items = append(items, a)
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) UploadAsset(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}

	id := "ast_" + uuid.New().String()[:8]
	ext := filepath.Ext(file.Filename)
	savedName := id + ext
	uploadDir := h.assetUploadDir()
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	savePath := filepath.Join(uploadDir, savedName)

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()

	dst, err := os.Create(savePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	url := fmt.Sprintf("/uploads/%s", savedName)
	width, _ := strconv.Atoi(c.PostForm("width"))
	height, _ := strconv.Atoi(c.PostForm("height"))
	mimeType := c.PostForm("mime_type")
	if mimeType == "" {
		mimeType = file.Header.Get("Content-Type")
	}
	projectID := strings.TrimSpace(c.PostForm("project_id"))
	assetType := strings.TrimSpace(c.PostForm("type"))
	if assetType == "" {
		assetType = "image"
	}
	sourceType := strings.TrimSpace(c.PostForm("source_type"))
	if sourceType == "" {
		sourceType = "upload"
	}
	createdBy := strings.TrimSpace(c.PostForm("created_by"))
	if createdBy == "" {
		createdBy = "member"
	}
	role, skuID, parentAssetID := c.PostForm("role"), c.PostForm("sku_id"), c.PostForm("parent_asset_id")
	generationMetadata := c.PostForm("generation_metadata")
	if generationMetadata == "" {
		generationMetadata = "{}"
	}
	aspectRatio := imageAspectRatio(width, height)
	_, err = h.db.Exec(`INSERT INTO assets (id, filename, url, size, mime_type, width, height, project_id,
		type, thumbnail_url, aspect_ratio, source_type, role, sku_id, parent_asset_id, created_by, generation_metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, file.Filename, url, file.Size, mimeType, width, height, projectID, assetType, url, aspectRatio,
		sourceType, role, skuID, parentAssetID, createdBy, generationMetadata)
	if err != nil {
		h.log.Error("upload asset", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.log.Info("asset uploaded", "id", id, "filename", file.Filename)
	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"filename":   file.Filename,
		"url":        url,
		"size":       file.Size,
		"mime_type":  mimeType,
		"width":      width,
		"height":     height,
		"category":   "其他",
		"tags":       "[]",
		"project_id": projectID, "type": assetType, "thumbnail_url": url, "aspect_ratio": aspectRatio,
		"source_type": sourceType, "role": role, "sku_id": skuID, "parent_asset_id": parentAssetID,
		"created_by": createdBy, "generation_metadata": generationMetadata,
	})
}

func imageAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	a, b := width, height
	for b != 0 {
		a, b = b, a%b
	}
	return strconv.Itoa(width/a) + ":" + strconv.Itoa(height/a)
}

func (h *Handler) assetUploadDir() string {
	if h != nil && h.uploadDir != "" {
		return h.uploadDir
	}
	return "uploads"
}

func (h *Handler) DeleteAsset(c *gin.Context) {
	id := c.Param("id")

	result, err := h.db.Exec(`UPDATE assets SET deleted_at=datetime('now','localtime') WHERE id = ? AND deleted_at = ''`, id)
	if err != nil {
		h.log.Error("delete asset", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	count, _ := result.RowsAffected()
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// UpdateAsset modifies category/tags of an existing asset
func (h *Handler) UpdateAsset(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Category *string `json:"category"`
		Tags     *string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Category != nil {
		if _, err := h.db.Exec(`UPDATE assets SET category = ? WHERE id = ?`, *body.Category, id); err != nil {
			h.log.Error("update asset category", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if body.Tags != nil {
		if _, err := h.db.Exec(`UPDATE assets SET tags = ? WHERE id = ?`, *body.Tags, id); err != nil {
			h.log.Error("update asset tags", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.Status(http.StatusNoContent)
}
