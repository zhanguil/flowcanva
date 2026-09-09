package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type rowScanner interface{ Scan(...any) error }

func decodeStringList(raw string) []string {
	result := []string{}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func encodeJSON(value any, fallback string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(raw)
}

func scanStudioProject(scanner rowScanner) (StudioProject, error) {
	var project StudioProject
	var tags string
	err := scanner.Scan(&project.ID, &project.WorkspaceID, &project.CanvasID, &project.Name,
		&project.ProductName, &project.CreatedBy, &project.Status, &project.CoverAssetID,
		&tags, &project.CreatedAt, &project.UpdatedAt)
	project.Tags = decodeStringList(tags)
	return project, err
}

func (h *Handler) studioProjectByID(id string) (StudioProject, error) {
	return scanStudioProject(h.db.QueryRow(`SELECT id, workspace_id, canvas_id, name, product_name,
		created_by, status, cover_asset_id, tags, created_at, updated_at
		FROM studio_projects WHERE id = ? AND deleted_at = ''`, id))
}

func (h *Handler) loadProductPack(projectID string) (*ProductPack, error) {
	var pack ProductPack
	var dnaRaw string
	err := h.db.QueryRow(`SELECT id, project_id, product_name, created_by, product_dna
		FROM product_packs WHERE project_id = ?`, projectID).
		Scan(&pack.ID, &pack.ProjectID, &pack.ProductName, &pack.CreatedBy, &dnaRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pack.ProductDNA = emptyProductDNA()
	_ = json.Unmarshal([]byte(dnaRaw), &pack.ProductDNA)
	pack.SKUs = []ProductSKU{}
	rows, err := h.db.Query(`SELECT id, product_pack_id, name, label, width, height, depth, reference_asset_ids, sort_order
		FROM product_skus WHERE product_pack_id = ? ORDER BY sort_order, created_at, id`, pack.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sku ProductSKU
		var width, height, depth sql.NullFloat64
		var references string
		if err := rows.Scan(&sku.ID, &sku.ProductPackID, &sku.Name, &sku.Label, &width, &height, &depth, &references, &sku.SortOrder); err != nil {
			rows.Close()
			return nil, err
		}
		if width.Valid {
			sku.Width = &width.Float64
		}
		if height.Valid {
			sku.Height = &height.Float64
		}
		if depth.Valid {
			sku.Depth = &depth.Float64
		}
		sku.ReferenceAssetIDs = decodeStringList(references)
		pack.SKUs = append(pack.SKUs, sku)
	}
	rows.Close()

	pack.References = ReferencePack{References: []ReferenceAsset{}}
	err = h.db.QueryRow(`SELECT id, project_id, product_pack_id, created_by FROM reference_packs WHERE product_pack_id = ?`, pack.ID).
		Scan(&pack.References.ID, &pack.References.ProjectID, &pack.References.ProductPackID, &pack.References.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return &pack, nil
	}
	if err != nil {
		return nil, err
	}
	refRows, err := h.db.Query(`SELECT id, asset_id, role, weight, locked, description, sort_order
		FROM reference_assets WHERE reference_pack_id = ? ORDER BY sort_order, created_at`, pack.References.ID)
	if err != nil {
		return nil, err
	}
	defer refRows.Close()
	for refRows.Next() {
		var ref ReferenceAsset
		var locked int
		if err := refRows.Scan(&ref.ID, &ref.AssetID, &ref.Role, &ref.Weight, &locked, &ref.Description, &ref.SortOrder); err != nil {
			return nil, err
		}
		ref.Locked = locked != 0
		pack.References.References = append(pack.References.References, ref)
	}
	return &pack, refRows.Err()
}

func (h *Handler) replaceProductPack(project StudioProject, input ProductPack) (*ProductPack, error) {
	tx, err := h.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	packID := input.ID
	if packID == "" {
		packID = "pack_" + uuid.New().String()[:8]
	}
	createdBy := input.CreatedBy
	if createdBy == "" {
		createdBy = project.CreatedBy
	}
	productName := input.ProductName
	if productName == "" {
		productName = project.ProductName
	}
	dna := input.ProductDNA
	if dna.StructuralFeatures == nil {
		dna.StructuralFeatures = map[string]any{}
	}
	if dna.Materials == nil {
		dna.Materials = map[string]string{}
	}
	if dna.ForbiddenChanges == nil {
		dna.ForbiddenChanges = []string{}
	}
	if dna.AllowedChanges == nil {
		dna.AllowedChanges = []string{}
	}

	var existingID string
	err = tx.QueryRow(`SELECT id FROM product_packs WHERE project_id = ?`, project.ID).Scan(&existingID)
	if err == nil {
		packID = existingID
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	_, err = tx.Exec(`INSERT INTO product_packs (id, project_id, product_name, created_by, product_dna)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(project_id) DO UPDATE SET product_name=excluded.product_name,
		product_dna=excluded.product_dna, updated_at=datetime('now','localtime')`,
		packID, project.ID, productName, createdBy, encodeJSON(dna, "{}"))
	if err != nil {
		return nil, err
	}

	refPackID := "refs_" + uuid.New().String()[:8]
	_ = tx.QueryRow(`SELECT id FROM reference_packs WHERE product_pack_id = ?`, packID).Scan(&refPackID)
	_, err = tx.Exec(`INSERT OR IGNORE INTO reference_packs (id, project_id, product_pack_id, created_by) VALUES (?, ?, ?, ?)`, refPackID, project.ID, packID, createdBy)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`DELETE FROM product_skus WHERE product_pack_id = ?`, packID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`DELETE FROM reference_assets WHERE reference_pack_id = ?`, refPackID); err != nil {
		return nil, err
	}

	for index, sku := range input.SKUs {
		if sku.Name == "" {
			return nil, errors.New("SKU 名称不能为空")
		}
		if sku.ID == "" {
			sku.ID = "sku_" + uuid.New().String()[:8]
		}
		_, err = tx.Exec(`INSERT INTO product_skus (id, product_pack_id, name, label, width, height, depth, reference_asset_ids, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, sku.ID, packID, sku.Name, sku.Label, sku.Width, sku.Height, sku.Depth, encodeJSON(sku.ReferenceAssetIDs, "[]"), index)
		if err != nil {
			return nil, err
		}
	}
	for index, ref := range input.References.References {
		if ref.AssetID == "" {
			return nil, errors.New("参考资产不能为空")
		}
		if ref.ID == "" {
			ref.ID = "ref_" + uuid.New().String()[:8]
		}
		if ref.Role == "" {
			ref.Role = "product_main"
		}
		if ref.Weight <= 0 {
			ref.Weight = 1
		}
		ref.SortOrder = index
		_, err = tx.Exec(`INSERT INTO reference_assets (id, reference_pack_id, asset_id, role, weight, locked, description, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, ref.ID, refPackID, ref.AssetID, ref.Role, ref.Weight, ref.Locked, ref.Description, ref.SortOrder)
		if err != nil {
			return nil, fmt.Errorf("保存参考资产: %w", err)
		}
	}
	_, err = tx.Exec(`UPDATE studio_projects SET product_name = ?, updated_at=datetime('now','localtime') WHERE id = ?`, productName, project.ID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return h.loadProductPack(project.ID)
}
