DROP TRIGGER IF EXISTS project_picture_book_profiles_immutable_delete;
DROP TABLE project_creation_bootstraps;
DROP TABLE project_setup_drafts;
ALTER TABLE projects DROP COLUMN setup_status;
