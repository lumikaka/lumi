CREATE TABLE project_picture_book_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL UNIQUE,
    format TEXT NOT NULL,
    aspect_ratio_mode TEXT NOT NULL,
    aspect_width INTEGER NOT NULL,
    aspect_height INTEGER NOT NULL,
    large_image_minimal_text INTEGER,
    interaction_mode TEXT,
    comic_layout TEXT,
    created_at DATETIME NOT NULL,
    CONSTRAINT project_picture_book_profiles_project_fk FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT project_picture_book_profiles_format_check CHECK (format IN ('classic_picture_book','wordless_picture_book','interactive_picture_book','comic_story','vertical_strip')),
    CONSTRAINT project_picture_book_profiles_aspect_mode_check CHECK (aspect_ratio_mode IN ('landscape','square','portrait','custom','fixed')),
    CONSTRAINT project_picture_book_profiles_aspect_value_check CHECK (aspect_width BETWEEN 1 AND 100 AND aspect_height BETWEEN 1 AND 100 AND aspect_width * 3 >= aspect_height AND aspect_height * 3 >= aspect_width),
    CONSTRAINT project_picture_book_profiles_shape_check CHECK (
        (format = 'classic_picture_book' AND aspect_ratio_mode <> 'fixed' AND large_image_minimal_text IN (0,1) AND interaction_mode IS NULL AND comic_layout IS NULL) OR
        (format = 'wordless_picture_book' AND aspect_ratio_mode <> 'fixed' AND large_image_minimal_text IS NULL AND interaction_mode IS NULL AND comic_layout IS NULL) OR
        (format = 'interactive_picture_book' AND aspect_ratio_mode = 'landscape' AND aspect_width = 4 AND aspect_height = 3 AND large_image_minimal_text IS NULL AND interaction_mode IN ('find_it','make_a_choice','guess','follow_along') AND comic_layout IS NULL) OR
        (format = 'comic_story' AND aspect_ratio_mode <> 'fixed' AND large_image_minimal_text IS NULL AND interaction_mode IS NULL AND comic_layout IN ('four_panel','page_comic')) OR
        (format = 'vertical_strip' AND aspect_ratio_mode = 'fixed' AND aspect_width = 1 AND aspect_height = 3 AND large_image_minimal_text IS NULL AND interaction_mode IS NULL AND comic_layout IS NULL)
    ),
    CONSTRAINT project_picture_book_profiles_preset_check CHECK (
        (aspect_ratio_mode = 'landscape' AND aspect_width = 4 AND aspect_height = 3) OR
        (aspect_ratio_mode = 'square' AND aspect_width = 1 AND aspect_height = 1) OR
        (aspect_ratio_mode = 'portrait' AND aspect_width = 3 AND aspect_height = 4) OR
        (aspect_ratio_mode = 'fixed' AND aspect_width = 1 AND aspect_height = 3) OR
        aspect_ratio_mode = 'custom'
    )
);

CREATE TRIGGER project_picture_book_profiles_immutable
BEFORE UPDATE ON project_picture_book_profiles
BEGIN
    SELECT RAISE(ABORT, 'picture book profile is immutable');
END;

INSERT INTO project_picture_book_profiles (
    project_id, format, aspect_ratio_mode, aspect_width, aspect_height,
    large_image_minimal_text, interaction_mode, comic_layout, created_at
)
SELECT id, 'vertical_strip', 'fixed', 1, 3, NULL, NULL, NULL, created_at
FROM projects;
