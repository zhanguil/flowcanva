package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func initDB(path string, log *slog.Logger) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	// The canvas often saves node content and image dimensions back-to-back.
	// A single writer connection avoids transient SQLITE_BUSY responses while
	// preserving SQLite as the project's lightweight local store.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if err := migrate(db, log); err != nil {
		return nil, err
	}

	log.Info("database initialized", "path", path)
	return db, nil
}

func migrate(db *sql.DB, log *slog.Logger) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS canvases (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			project_type TEXT NOT NULL DEFAULT 'canvas',
			created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id         TEXT PRIMARY KEY,
			canvas_id  TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
			node_type  TEXT NOT NULL DEFAULT 'text',
			x          REAL NOT NULL DEFAULT 0,
			y          REAL NOT NULL DEFAULT 0,
			width      REAL NOT NULL DEFAULT 280,
			height     REAL NOT NULL DEFAULT 200,
			content    TEXT NOT NULL DEFAULT '',
			config     TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS node_configs (
			id              TEXT PRIMARY KEY,
			node_type       TEXT UNIQUE NOT NULL,
			model_name      TEXT NOT NULL DEFAULT '',
			api_channel     TEXT NOT NULL DEFAULT '',
			base_url        TEXT NOT NULL DEFAULT '',
			api_key         TEXT NOT NULL DEFAULT '',
			parameters      TEXT NOT NULL DEFAULT '{}',
			prompt_template TEXT NOT NULL DEFAULT '',
			extra_config    TEXT NOT NULL DEFAULT '{}',
			enabled         INTEGER NOT NULL DEFAULT 1,
			created_at      TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS logs (
			id         TEXT PRIMARY KEY,
			level      TEXT NOT NULL DEFAULT 'INFO',
			module     TEXT NOT NULL DEFAULT '',
			message    TEXT NOT NULL DEFAULT '',
			detail     TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS assets (
			id         TEXT PRIMARY KEY,
			filename   TEXT NOT NULL DEFAULT '',
			url        TEXT NOT NULL DEFAULT '',
			size       INTEGER NOT NULL DEFAULT 0,
			mime_type  TEXT NOT NULL DEFAULT '',
			width      INTEGER NOT NULL DEFAULT 0,
			height     INTEGER NOT NULL DEFAULT 0,
			category   TEXT NOT NULL DEFAULT '其他',
			tags       TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS presets (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			prompt      TEXT NOT NULL DEFAULT '',
			category    TEXT NOT NULL DEFAULT '通用',
			preset_type TEXT NOT NULL DEFAULT 'image',
			scope       TEXT NOT NULL DEFAULT 'global',
			canvas_id   TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS edges (
			id             TEXT PRIMARY KEY,
			canvas_id      TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
			source_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			target_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			created_at     TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS studio_projects (
			id             TEXT PRIMARY KEY,
			workspace_id   TEXT NOT NULL DEFAULT 'workspace_company',
			canvas_id      TEXT NOT NULL UNIQUE REFERENCES canvases(id),
			name           TEXT NOT NULL DEFAULT '',
			product_name   TEXT NOT NULL DEFAULT '',
			created_by     TEXT NOT NULL DEFAULT 'member',
			status         TEXT NOT NULL DEFAULT 'draft',
			cover_asset_id TEXT NOT NULL DEFAULT '',
			tags           TEXT NOT NULL DEFAULT '[]',
			deleted_at     TEXT NOT NULL DEFAULT '',
			created_at     TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at     TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS product_packs (
			id           TEXT PRIMARY KEY,
			project_id   TEXT NOT NULL UNIQUE REFERENCES studio_projects(id),
			product_name TEXT NOT NULL DEFAULT '',
			created_by   TEXT NOT NULL DEFAULT 'member',
			product_dna  TEXT NOT NULL DEFAULT '{}',
			created_at   TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at   TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS reference_packs (
			id              TEXT PRIMARY KEY,
			project_id      TEXT NOT NULL REFERENCES studio_projects(id),
			product_pack_id TEXT NOT NULL UNIQUE REFERENCES product_packs(id),
			created_by      TEXT NOT NULL DEFAULT 'member',
			created_at      TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS product_skus (
			id                    TEXT PRIMARY KEY,
			product_pack_id       TEXT NOT NULL REFERENCES product_packs(id) ON DELETE CASCADE,
			name                  TEXT NOT NULL DEFAULT '',
			label                 TEXT NOT NULL DEFAULT '',
			width                 REAL,
			height                REAL,
			depth                 REAL,
			reference_asset_ids   TEXT NOT NULL DEFAULT '[]',
			sort_order            INTEGER NOT NULL DEFAULT 0,
			created_at            TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at            TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS reference_assets (
			id                TEXT PRIMARY KEY,
			reference_pack_id TEXT NOT NULL REFERENCES reference_packs(id) ON DELETE CASCADE,
			asset_id          TEXT NOT NULL REFERENCES assets(id),
			role              TEXT NOT NULL DEFAULT 'product_main',
			weight            REAL NOT NULL DEFAULT 1,
			locked            INTEGER NOT NULL DEFAULT 0,
			description       TEXT NOT NULL DEFAULT '',
			sort_order        INTEGER NOT NULL DEFAULT 0,
			created_at        TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			UNIQUE(reference_pack_id, asset_id)
		)`,
		`CREATE TABLE IF NOT EXISTS recipes (
			id                     TEXT PRIMARY KEY,
			name                   TEXT NOT NULL,
			version                INTEGER NOT NULL DEFAULT 1,
			outputs                TEXT NOT NULL DEFAULT '[]',
			prompt_template_id     TEXT NOT NULL DEFAULT '',
			estimated_cost_per_job REAL NOT NULL DEFAULT 0,
			currency               TEXT NOT NULL DEFAULT 'USD',
			created_by             TEXT NOT NULL DEFAULT 'admin',
			enabled                INTEGER NOT NULL DEFAULT 1,
			created_at             TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at             TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS recipe_runs (
			id             TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL REFERENCES studio_projects(id),
			recipe_id      TEXT NOT NULL REFERENCES recipes(id),
			recipe_version INTEGER NOT NULL,
			request_id     TEXT NOT NULL UNIQUE,
			status         TEXT NOT NULL DEFAULT 'pending',
			job_count      INTEGER NOT NULL DEFAULT 0,
			total_cost     REAL NOT NULL DEFAULT 0,
			created_by     TEXT NOT NULL DEFAULT 'member',
			created_at     TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			started_at     TEXT NOT NULL DEFAULT '',
			finished_at    TEXT NOT NULL DEFAULT '',
			layout_origin_x REAL NOT NULL DEFAULT 200,
			layout_origin_y REAL NOT NULL DEFAULT 200
		)`,
		`CREATE TABLE IF NOT EXISTS generation_jobs (
			id                 TEXT PRIMARY KEY,
			project_id         TEXT NOT NULL REFERENCES studio_projects(id),
			recipe_run_id      TEXT NOT NULL REFERENCES recipe_runs(id) ON DELETE CASCADE,
			sku_id             TEXT NOT NULL REFERENCES product_skus(id),
			output_type        TEXT NOT NULL,
			status             TEXT NOT NULL DEFAULT 'pending',
			provider           TEXT NOT NULL DEFAULT 'legacy',
			model              TEXT NOT NULL DEFAULT '',
			prompt             TEXT NOT NULL DEFAULT '',
			prompt_version     INTEGER NOT NULL DEFAULT 1,
			reference_pack     TEXT NOT NULL DEFAULT '{}',
			aspect_ratio       TEXT NOT NULL DEFAULT '1:1',
			created_by         TEXT NOT NULL DEFAULT 'member',
			created_at         TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			started_at         TEXT NOT NULL DEFAULT '',
			finished_at        TEXT NOT NULL DEFAULT '',
			estimated_cost     REAL NOT NULL DEFAULT 0,
			actual_cost        REAL NOT NULL DEFAULT 0,
			duration_ms        INTEGER NOT NULL DEFAULT 0,
			retry_count        INTEGER NOT NULL DEFAULT 0,
			result_asset_id    TEXT NOT NULL DEFAULT '',
			error_code         TEXT NOT NULL DEFAULT '',
			error_message      TEXT NOT NULL DEFAULT ''
		)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}

	// seed default node configs if not exist
	types := []string{"text", "image", "video", "table", "full_image", "agent", "workflow", "asset", "director"}
	for _, t := range types {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO node_configs (id, node_type) VALUES (?, ?)`,
			"cfg_"+t, t,
		)
		if err != nil {
			return err
		}
	}

	// apply seed data from embedded seed.json
	raw := EmbeddedSeedJSON
	if len(raw) > 0 {
		var seed struct {
			NodeConfigs map[string]struct {
				ModelName   string         `json:"model_name"`
				APIChannel  string         `json:"api_channel"`
				BaseURL     string         `json:"base_url"`
				APIKey      string         `json:"api_key"`
				Parameters  map[string]any `json:"parameters"`
				ExtraConfig map[string]any `json:"extra_config"`
			} `json:"node_configs"`
		}
		if json.Unmarshal(raw, &seed) == nil {
			for nodeType, cfg := range seed.NodeConfigs {
				paramsJSON, _ := json.Marshal(cfg.Parameters)
				extraJSON, _ := json.Marshal(cfg.ExtraConfig)
				_, err := db.Exec(
					`UPDATE node_configs SET model_name=?, api_channel=?, base_url=?, api_key=?, parameters=?, extra_config=?, updated_at=datetime('now','localtime') WHERE node_type=? AND model_name=''`,
					cfg.ModelName, cfg.APIChannel, cfg.BaseURL, cfg.APIKey, string(paramsJSON), string(extraJSON), nodeType,
				)
				if err != nil {
					log.Info("seed apply failed", "type", nodeType, "error", err)
				}
			}
		}
	}

	// ensure upload dir exists
	_ = os.MkdirAll("uploads", 0755)

	// backward-compat: add category/tags columns to existing assets table
	addColsIfMissing(db, "assets", []string{
		"mime_type TEXT NOT NULL DEFAULT ''", "category TEXT NOT NULL DEFAULT '其他'", "tags TEXT NOT NULL DEFAULT '[]'",
		"project_id TEXT NOT NULL DEFAULT ''", "type TEXT NOT NULL DEFAULT 'image'", "thumbnail_url TEXT NOT NULL DEFAULT ''",
		"aspect_ratio TEXT NOT NULL DEFAULT ''", "source_type TEXT NOT NULL DEFAULT 'upload'", "role TEXT NOT NULL DEFAULT ''",
		"sku_id TEXT NOT NULL DEFAULT ''", "parent_asset_id TEXT NOT NULL DEFAULT ''", "created_by TEXT NOT NULL DEFAULT 'member'",
		"generation_metadata TEXT NOT NULL DEFAULT '{}'", "deleted_at TEXT NOT NULL DEFAULT ''",
	})
	addColsIfMissing(db, "product_skus", []string{"sort_order INTEGER NOT NULL DEFAULT 0"})
	addColsIfMissing(db, "recipe_runs", []string{"layout_origin_x REAL NOT NULL DEFAULT 200", "layout_origin_y REAL NOT NULL DEFAULT 200"})
	// Generation nodes are controls. Their images now live in independent asset nodes,
	// so collapse legacy 300px preview cards that no longer render an output preview.
	if _, err := db.Exec(`UPDATE nodes SET height = 88 WHERE node_type = 'image' AND height > 120`); err != nil {
		log.Warn("compact legacy generation nodes", "error", err)
	}

	// backward-compat: add project_type column to canvases
	addColsIfMissing(db, "canvases", []string{"project_type TEXT NOT NULL DEFAULT 'canvas'"})

	// backward-compat: add preset_type column to presets
	addColsIfMissing(db, "presets", []string{"preset_type TEXT NOT NULL DEFAULT 'image'"})

	// performance index for asset pagination
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_assets_created_at ON assets(created_at)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_studio_projects_updated_at ON studio_projects(updated_at)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_product_skus_pack ON product_skus(product_pack_id)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_reference_assets_pack ON reference_assets(reference_pack_id, sort_order)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_recipe_runs_project ON recipe_runs(project_id, created_at)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_generation_jobs_run ON generation_jobs(recipe_run_id, status)`)
	// Existing canvases remain usable and appear as draft V0.2 projects.
	db.Exec(`INSERT OR IGNORE INTO studio_projects (id, canvas_id, name, product_name)
		SELECT 'prj_' || substr(id, 4), id, name, name FROM canvases WHERE project_type = 'canvas'`)
	// Legacy canvases also need an editable empty Product Pack, otherwise their
	// new project detail page would have no setup form.
	db.Exec(`INSERT OR IGNORE INTO product_packs (id, project_id, product_name, created_by, product_dna)
		SELECT 'pack_' || substr(id, 5), id, product_name, created_by,
		'{"productType":"","structuralFeatures":{},"materials":{},"forbiddenChanges":[],"allowedChanges":[]}'
		FROM studio_projects WHERE deleted_at=''`)
	db.Exec(`INSERT OR IGNORE INTO reference_packs (id, project_id, product_pack_id, created_by)
		SELECT 'refs_' || substr(pp.id, 6), pp.project_id, pp.id, pp.created_by FROM product_packs pp`)
	db.Exec(`INSERT OR IGNORE INTO recipes (id, name, version, outputs) VALUES
		('furniture_sku_main_images', 'SKU 主图套装', 1, '[{"outputType":"scene_front","aspectRatio":"3:4"},{"outputType":"scene_front","aspectRatio":"1:1"}]'),
		('furniture_sku_full_pack', 'SKU 全套图片', 1, '[{"outputType":"scene_front","aspectRatio":"3:4"},{"outputType":"scene_front","aspectRatio":"1:1"},{"outputType":"scene_45","aspectRatio":"3:4"},{"outputType":"white_background","aspectRatio":"1:1"},{"outputType":"material_detail","aspectRatio":"1:1"},{"outputType":"feature_detail","aspectRatio":"3:4"}]')`)

	return nil
}

// addColsIfMissing adds columns to a table if they don't exist (SQLite compat)
func addColsIfMissing(db *sql.DB, table string, cols []string) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return
	}
	defer rows.Close()
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return
		}
		existing[name] = true
	}
	for _, c := range cols {
		parts := strings.SplitN(c, " ", 2)
		colName := parts[0]
		if existing[colName] {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, c)); err != nil {
			// column may already exist or add unsupported; ignore
			continue
		}
	}
}
