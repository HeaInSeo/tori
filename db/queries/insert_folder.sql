INSERT INTO folders (path, total_size, file_count, created_time)
VALUES (?, ?, ?, ?)
    ON CONFLICT(path) DO NOTHING;
