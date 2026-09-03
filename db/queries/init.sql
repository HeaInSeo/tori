CREATE TABLE IF NOT EXISTS folders (
                                       id INTEGER PRIMARY KEY AUTOINCREMENT,
                                       path TEXT NOT NULL UNIQUE,
                                       total_size INTEGER DEFAULT 0,
                                       file_count INTEGER DEFAULT 0,
                                       created_time DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
                                     id INTEGER PRIMARY KEY AUTOINCREMENT,
                                     folder_id INTEGER NOT NULL,
                                     name TEXT NOT NULL,
                                     size INTEGER NOT NULL,
                                     created_time DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
                                     FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE,
    UNIQUE(folder_id, name)
);

CREATE INDEX IF NOT EXISTS idx_files_folder_id ON files(folder_id);

CREATE TABLE IF NOT EXISTS snapshot_meta (
                                     key TEXT PRIMARY KEY,
                                     value TEXT NOT NULL
);

