ALTER TABLE recent_projects ADD COLUMN auto_name_directory INTEGER NOT NULL DEFAULT 0 CHECK (auto_name_directory IN (0,1));
ALTER TABLE recent_projects ADD COLUMN pending_directory_name TEXT NOT NULL DEFAULT '';
