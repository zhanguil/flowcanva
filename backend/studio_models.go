package main

type StudioProject struct {
	ID           string   `json:"id"`
	WorkspaceID  string   `json:"workspace_id"`
	CanvasID     string   `json:"canvas_id"`
	Name         string   `json:"name"`
	ProductName  string   `json:"product_name"`
	CreatedBy    string   `json:"created_by"`
	Status       string   `json:"status"`
	CoverAssetID string   `json:"cover_asset_id"`
	Tags         []string `json:"tags"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

type ProductDNA struct {
	ProductType        string            `json:"productType"`
	StructuralFeatures map[string]any    `json:"structuralFeatures"`
	Materials          map[string]string `json:"materials"`
	ForbiddenChanges   []string          `json:"forbiddenChanges"`
	AllowedChanges     []string          `json:"allowedChanges"`
}

type ProductSKU struct {
	ID                string   `json:"id"`
	ProductPackID     string   `json:"product_pack_id"`
	Name              string   `json:"name"`
	Label             string   `json:"label"`
	Width             *float64 `json:"width"`
	Height            *float64 `json:"height"`
	Depth             *float64 `json:"depth"`
	ReferenceAssetIDs []string `json:"reference_asset_ids"`
	SortOrder         int      `json:"sort_order"`
}

type ReferenceAsset struct {
	ID          string  `json:"id"`
	AssetID     string  `json:"asset_id"`
	Role        string  `json:"role"`
	Weight      float64 `json:"weight"`
	Locked      bool    `json:"locked"`
	Description string  `json:"description"`
	SortOrder   int     `json:"sort_order"`
}

type ReferencePack struct {
	ID            string           `json:"id"`
	ProjectID     string           `json:"project_id"`
	ProductPackID string           `json:"product_pack_id"`
	CreatedBy     string           `json:"created_by"`
	References    []ReferenceAsset `json:"references"`
}

type ProductPack struct {
	ID          string        `json:"id"`
	ProjectID   string        `json:"project_id"`
	ProductName string        `json:"product_name"`
	CreatedBy   string        `json:"created_by"`
	ProductDNA  ProductDNA    `json:"product_dna"`
	SKUs        []ProductSKU  `json:"skus"`
	References  ReferencePack `json:"reference_pack"`
}

type StudioProjectDetail struct {
	StudioProject
	ProductPack *ProductPack `json:"product_pack,omitempty"`
}

func emptyProductDNA() ProductDNA {
	return ProductDNA{
		StructuralFeatures: map[string]any{}, Materials: map[string]string{},
		ForbiddenChanges: []string{}, AllowedChanges: []string{},
	}
}
