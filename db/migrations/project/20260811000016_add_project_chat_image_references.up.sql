CREATE TABLE chat_item_file_references (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    chat_item_id INTEGER NOT NULL,
    file_id INTEGER NOT NULL,
    upload_stashed_id INTEGER,
    position INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT chat_item_file_references_item_fk FOREIGN KEY (chat_item_id) REFERENCES chat_items(id) ON DELETE CASCADE,
    CONSTRAINT chat_item_file_references_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT chat_item_file_references_upload_fk FOREIGN KEY (upload_stashed_id) REFERENCES upload_stashed(id) ON DELETE SET NULL,
    CONSTRAINT chat_item_file_references_position_check CHECK (position BETWEEN 1 AND 4),
    UNIQUE(chat_item_id, position)
);

CREATE INDEX chat_item_file_references_item_position_index
    ON chat_item_file_references(chat_item_id, position, id);
CREATE INDEX chat_item_file_references_file_index
    ON chat_item_file_references(file_id);

CREATE TABLE chat_follow_up_file_references (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    follow_up_id INTEGER NOT NULL,
    file_id INTEGER NOT NULL,
    upload_stashed_id INTEGER,
    position INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    CONSTRAINT chat_follow_up_file_references_follow_up_fk FOREIGN KEY (follow_up_id) REFERENCES chat_follow_ups(id) ON DELETE CASCADE,
    CONSTRAINT chat_follow_up_file_references_file_fk FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE RESTRICT,
    CONSTRAINT chat_follow_up_file_references_upload_fk FOREIGN KEY (upload_stashed_id) REFERENCES upload_stashed(id) ON DELETE SET NULL,
    CONSTRAINT chat_follow_up_file_references_position_check CHECK (position BETWEEN 1 AND 4),
    UNIQUE(follow_up_id, position)
);

CREATE INDEX chat_follow_up_file_references_follow_up_position_index
    ON chat_follow_up_file_references(follow_up_id, position, id);
CREATE INDEX chat_follow_up_file_references_file_index
    ON chat_follow_up_file_references(file_id);
