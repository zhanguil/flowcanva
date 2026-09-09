package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var projectStatuses = map[string]bool{
	"draft": true, "generating": true, "reviewing": true, "completed": true, "archived": true,
}

func (h *Handler) ListStudioProjects(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	status := strings.TrimSpace(c.Query("status"))
	where := ` WHERE deleted_at = ''`
	args := []any{}
	if status != "" {
		where += ` AND status = ?`
		args = append(args, status)
	}
	var total int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM studio_projects`+where, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取项目失败"})
		return
	}
	rows, err := h.db.Query(`SELECT id, workspace_id, canvas_id, name, product_name, created_by,
		status, cover_asset_id, tags, created_at, updated_at FROM studio_projects`+where+`
		ORDER BY updated_at DESC LIMIT ? OFFSET ?`, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取项目失败"})
		return
	}
	defer rows.Close()
	items := []StudioProject{}
	for rows.Next() {
		project, err := scanStudioProject(rows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取项目失败"})
			return
		}
		items = append(items, project)
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) CreateStudioProject(c *gin.Context) {
	var body struct {
		Name        string   `json:"name"`
		ProductName string   `json:"product_name"`
		CreatedBy   string   `json:"created_by"`
		Tags        []string `json:"tags"`
		SKUs        []string `json:"skus"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "项目资料格式不正确"})
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.ProductName = strings.TrimSpace(body.ProductName)
	if body.Name == "" || body.ProductName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "项目名称和产品名称不能为空"})
		return
	}
	if strings.TrimSpace(body.CreatedBy) == "" {
		body.CreatedBy = "member"
	}
	canvasID := "cv_" + uuid.New().String()[:8]
	projectID := "prj_" + uuid.New().String()[:8]
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建项目失败"})
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO canvases (id, name, project_type) VALUES (?, ?, 'canvas')`, canvasID, body.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建项目画布失败"})
		return
	}
	_, err = tx.Exec(`INSERT INTO studio_projects (id, canvas_id, name, product_name, created_by, tags)
		VALUES (?, ?, ?, ?, ?, ?)`, projectID, canvasID, body.Name, body.ProductName, body.CreatedBy, encodeJSON(body.Tags, "[]"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建项目失败"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存项目失败"})
		return
	}
	project, _ := h.studioProjectByID(projectID)
	packInput := ProductPack{ProductName: body.ProductName, CreatedBy: body.CreatedBy, ProductDNA: emptyProductDNA()}
	for _, name := range body.SKUs {
		name = strings.TrimSpace(name)
		if name != "" {
			packInput.SKUs = append(packInput.SKUs, ProductSKU{Name: name, Label: name})
		}
	}
	packInput.References.References = []ReferenceAsset{}
	pack, err := h.replaceProductPack(project, packInput)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "项目已创建，但产品资料保存失败"})
		return
	}
	c.JSON(http.StatusCreated, StudioProjectDetail{StudioProject: project, ProductPack: pack})
}

func (h *Handler) GetStudioProject(c *gin.Context) {
	project, err := h.studioProjectByID(c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取项目失败"})
		return
	}
	pack, err := h.loadProductPack(project.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取产品资料失败"})
		return
	}
	c.JSON(http.StatusOK, StudioProjectDetail{StudioProject: project, ProductPack: pack})
}

func (h *Handler) UpdateStudioProject(c *gin.Context) {
	project, err := h.studioProjectByID(c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取项目失败"})
		return
	}
	var body struct {
		Name         string   `json:"name"`
		ProductName  string   `json:"product_name"`
		Status       string   `json:"status"`
		CoverAssetID string   `json:"cover_asset_id"`
		Tags         []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "项目资料格式不正确"})
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		project.Name = strings.TrimSpace(body.Name)
	}
	if strings.TrimSpace(body.ProductName) != "" {
		project.ProductName = strings.TrimSpace(body.ProductName)
	}
	if body.Status != "" {
		if !projectStatuses[body.Status] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "项目状态不支持"})
			return
		}
		project.Status = body.Status
	}
	if body.CoverAssetID != "" {
		project.CoverAssetID = body.CoverAssetID
	}
	if body.Tags != nil {
		project.Tags = body.Tags
	}
	_, err = h.db.Exec(`UPDATE studio_projects SET name=?, product_name=?, status=?, cover_asset_id=?, tags=?,
		updated_at=datetime('now','localtime') WHERE id=?`, project.Name, project.ProductName, project.Status,
		project.CoverAssetID, encodeJSON(project.Tags, "[]"), project.ID)
	if err == nil {
		_, err = h.db.Exec(`UPDATE canvases SET name=?, updated_at=datetime('now','localtime') WHERE id=?`, project.Name, project.CanvasID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存项目失败"})
		return
	}
	updated, _ := h.studioProjectByID(project.ID)
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) ArchiveStudioProject(c *gin.Context) {
	result, err := h.db.Exec(`UPDATE studio_projects SET status='archived', deleted_at=datetime('now','localtime'),
		updated_at=datetime('now','localtime') WHERE id=? AND deleted_at=''`, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "归档项目失败"})
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) PutProductPack(c *gin.Context) {
	project, err := h.studioProjectByID(c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取项目失败"})
		return
	}
	var input ProductPack
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "产品资料格式不正确"})
		return
	}
	pack, err := h.replaceProductPack(project, input)
	if err != nil {
		h.log.Error("save product pack", "project_id", project.ID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pack)
}
