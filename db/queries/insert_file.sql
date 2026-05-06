INSERT INTO files (folder_id, name, size, created_time)
VALUES (?, ?, ?, ?)
    ON CONFLICT(folder_id, name) DO NOTHING;

