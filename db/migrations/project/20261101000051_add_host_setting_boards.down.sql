-- Preserve the host batch history rather than silently dropping it.
CREATE TEMP TABLE host_setting_down_guard (n INTEGER CHECK(n=0));
INSERT INTO host_setting_down_guard SELECT count(*) FROM host_setting_boards;
DROP TABLE host_setting_down_guard;
DROP TABLE host_setting_breakdowns;
DROP TABLE host_setting_boards;
