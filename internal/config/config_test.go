package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
application:
  concurrency: 5
  work_path: ./data/workspace/
  log:
    dir: ./logs/
  database:
    db_type: sqlite
    file_path: ./data/meta.db
    journal_mode: WAL
    synchronous: NORMAL
    busy_timeout: 5000
    max_open_conns: 1
  notify:
    type: http
  snapshots:
    - id: DB01
      db_type: mysql
      ip: 192.168.1.1
      port: 3306
      username: user
      password: pass
      schema: testdb
      storages:
        - id: local01
          type: local
          path: /data/backup/
          retention_count: 7
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Application.Concurrency != 5 {
		t.Errorf("expected concurrency 5, got %d", cfg.Application.Concurrency)
	}
	if len(cfg.Application.Snapshots) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(cfg.Application.Snapshots))
	}
}

func TestValidate_MissingConcurrency(t *testing.T) {
	cfg := &Config{
		Application: Application{
			Concurrency: 0,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for missing concurrency")
	}
}

func TestValidate_DuplicateSnapshotID(t *testing.T) {
	cfg := &Config{
		Application: Application{
			Concurrency: 1,
			WorkPath:    "/tmp",
			Log:         LogConfig{Dir: "/tmp/logs"},
			Database:    Database{DBType: "sqlite", FilePath: "/tmp/db"},
			Snapshots: []SnapshotsSpec{
				{ID: "DB01", DBType: "mysql", IP: "127.0.0.1", Port: 3306, Schema: "test"},
				{ID: "DB01", DBType: "mysql", IP: "127.0.0.2", Port: 3306, Schema: "test"},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for duplicate snapshot ID")
	}
}
