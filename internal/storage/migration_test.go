package storage

import (
	"path/filepath"
	"testing"
	"time"
)

// A chief that already ran created import_jobs without the maintainer column.
// CREATE TABLE IF NOT EXISTS leaves it alone, so opening the database has to
// migrate it.
func TestInitSchema_AddsMaintainerToAnExistingImportJobsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "irgsh.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Recreate the pre-migration table.
	if _, err := db.Exec(`DROP TABLE import_jobs`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE import_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_uuid TEXT UNIQUE NOT NULL,
		source_url TEXT NOT NULL,
		dist TEXT NOT NULL,
		packages TEXT NOT NULL,
		component TEXT NOT NULL DEFAULT 'main',
		is_experimental BOOLEAN DEFAULT FALSE,
		submitted_at DATETIME NOT NULL,
		state TEXT NOT NULL DEFAULT 'PENDING',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO import_jobs (task_uuid, source_url, dist, packages, component, submitted_at, state)
		VALUES ('old-job', 'https://example.id/debian', 'sid', 'firefox', 'main', ?, 'PENDING')`, time.Now()); err != nil {
		t.Fatal(err)
	}

	exists, err := db.columnExists("import_jobs", "maintainer")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("the pre-migration table must not have the column")
	}
	db.Close()

	// Reopening runs the migration.
	db, err = NewDB(dbPath)
	if err != nil {
		t.Fatalf("reopening a pre-migration database must succeed: %v", err)
	}
	defer db.Close()

	exists, err = db.columnExists("import_jobs", "maintainer")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("maintainer column was not added")
	}

	// The existing row survives and reads back with an empty maintainer.
	store := NewImportJobStore(db, 10)
	job, err := store.GetImportJob("old-job")
	if err != nil {
		t.Fatalf("the existing row must still be readable: %v", err)
	}
	if job.Maintainer != "" {
		t.Fatalf("expected an empty maintainer for an old row, got %q", job.Maintainer)
	}

	// And a new row round-trips the maintainer.
	if err := store.RecordImportJob(ImportJobInfo{
		TaskUUID: "new-job", SourceURL: "https://example.id/debian", Dist: "sid",
		Packages: "firefox", Component: "main", Maintainer: "Herpiko Dwi Aguno <herpiko@gmail.com>",
		SubmittedAt: time.Now(), State: "PENDING",
	}); err != nil {
		t.Fatal(err)
	}
	job, err = store.GetImportJob("new-job")
	if err != nil {
		t.Fatal(err)
	}
	if job.Maintainer != "Herpiko Dwi Aguno <herpiko@gmail.com>" {
		t.Fatalf("unexpected maintainer: %q", job.Maintainer)
	}
}

func TestApplyMigrations_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "irgsh.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 3; i++ {
		if err := db.applyMigrations(); err != nil {
			t.Fatalf("migration run %d failed: %v", i+1, err)
		}
	}
}
