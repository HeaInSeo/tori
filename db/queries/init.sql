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

-- TDI-I2A source envelope. Separates logical source identity (source_envelope) from
-- observation meaning (source_revisions) and physical access route (source_endpoints).
-- db/source.go creates these idempotently too, so a pre-existing DB needs no migration
-- step; they are declared here so a freshly initialized DB has the full schema.

CREATE TABLE IF NOT EXISTS source_envelope (
                                     singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                                     source_id TEXT NOT NULL,
                                     adoption_origin TEXT NOT NULL,
                                     inventory_predates_id INTEGER NOT NULL,
                                     adopted_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Append-only: the primary key is the content hash of `canonical`, so an existing
-- revision's meaning can never be edited underneath a snapshot accepted against it.
CREATE TABLE IF NOT EXISTS source_revisions (
                                     source_id TEXT NOT NULL,
                                     revision_id TEXT NOT NULL,
                                     canonical TEXT NOT NULL,
                                     origin TEXT NOT NULL,
                                     created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
                                     PRIMARY KEY (source_id, revision_id)
);

-- Append-only endpoint history. Relocation and credential rotation append here and move
-- the current-endpoint pointer; neither touches source_revisions.
CREATE TABLE IF NOT EXISTS source_endpoints (
                                     source_id TEXT NOT NULL,
                                     endpoint_id TEXT NOT NULL,
                                     canonical TEXT NOT NULL,
                                     first_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
                                     PRIMARY KEY (source_id, endpoint_id)
);

