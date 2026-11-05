CREATE TABLE page_image_reviews (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    image_variant_id INTEGER NOT NULL REFERENCES comic_image_variants(id) ON DELETE CASCADE,
    page_revision INTEGER NOT NULL CHECK(page_revision >= 0),
    revision INTEGER NOT NULL CHECK(revision > 0),
    verdict TEXT NOT NULL CHECK(verdict IN ('passed', 'needs_revision')),
    observation TEXT NOT NULL CHECK(length(trim(observation)) BETWEEN 1 AND 2000),
    UNIQUE(image_variant_id, page_revision)
);
