package storage

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uuid TEXT UNIQUE NOT NULL,
    package_name TEXT NOT NULL,
    package_version TEXT NOT NULL,
    maintainer TEXT NOT NULL,
    component TEXT NOT NULL,
    is_experimental BOOLEAN DEFAULT FALSE,
    submitted_at DATETIME NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    current_stage TEXT DEFAULT 'build',
    build_state TEXT DEFAULT '',
    repo_state TEXT DEFAULT '',
    package_url TEXT DEFAULT '',
    source_url TEXT DEFAULT '',
    package_branch TEXT DEFAULT '',
    source_branch TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS iso_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uuid TEXT UNIQUE NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL,
    submitted_at DATETIME NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jobs_submitted_at ON jobs(submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_task_uuid ON jobs(task_uuid);
CREATE INDEX IF NOT EXISTS idx_iso_jobs_submitted_at ON iso_jobs(submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_iso_jobs_task_uuid ON iso_jobs(task_uuid);
`
