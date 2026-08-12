ALTER TABLE projects
ADD COLUMN generation_language TEXT NOT NULL DEFAULT 'zh-Hans'
CHECK (generation_language IN ('zh-Hans', 'en'));
