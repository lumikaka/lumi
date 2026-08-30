CREATE TABLE comic_section_premise_asset_selections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    comic_section_id INTEGER NOT NULL,
    premise_asset_id INTEGER NOT NULL,
    sort_order INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT comic_section_premise_asset_selections_section_fk
        FOREIGN KEY (comic_section_id) REFERENCES comic_sections(id) ON DELETE CASCADE,
    CONSTRAINT comic_section_premise_asset_selections_asset_fk
        FOREIGN KEY (premise_asset_id) REFERENCES premise_assets(id) ON DELETE CASCADE,
    CONSTRAINT comic_section_premise_asset_selections_order_check CHECK (sort_order > 0),
    UNIQUE(comic_section_id, premise_asset_id),
    UNIQUE(comic_section_id, sort_order)
);

CREATE INDEX comic_section_premise_asset_selections_asset_index
    ON comic_section_premise_asset_selections(premise_asset_id, comic_section_id);
