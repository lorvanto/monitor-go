package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlineBackupLockedSourceAndRestore(t *testing.T) {
	s, _, c := fixture(t)
	body := []byte("snapshot content\n")
	f := uploadForTest(t, c, "1", "snapshot.txt", body, 200)
	// Trashed objects must also survive a restore.
	mustSQL(t, s, "UPDATE files SET deleted_at=1 WHERE id=?", f.ID)
	cipher, err := s.Store.encrypt("backup secret")
	if err != nil {
		t.Fatal(err)
	}
	mustSQL(t, s, "INSERT INTO monitors(name,type,target,group_name,interval_seconds,timeout_seconds,secret) VALUES('backup','TCP','example.com:443','test',60,5,?)", cipher)
	out := filepath.Join(t.TempDir(), "complete")
	manifest, err := backup(context.Background(), s.Store.Dir, out, func() {
		uploadForTest(t, c, "1", "after-snapshot.txt", []byte("later"), 200)
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.FileCount != 1 || manifest.SchemaVersion != currentSchemaVersion {
		t.Fatalf("manifest: %+v", manifest)
	}
	for _, name := range []string{backupIncomplete, "setup-token", "instance.lock", "uploads/staging", "uploads/cache"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected backup file %s: %v", name, err)
		}
	}
	for _, name := range []string{"master.key", "monitor.db", "backup-manifest.json", "uploads/objects/" + manifest.Objects[0].Key} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("permissions %s: %v %v", name, info, err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(out, "monitor.db"))
	sum := sha256.Sum256(b)
	if manifest.DatabaseSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("database hash differs")
	}
	b, _ = os.ReadFile(filepath.Join(out, "backup-manifest.json"))
	var saved BackupManifest
	if err = json.Unmarshal(b, &saved); err != nil || saved.FileCount != 1 {
		t.Fatalf("saved manifest: %v", err)
	}
	if _, err = Backup(context.Background(), s.Store.Dir, out); err == nil {
		t.Fatal("existing output accepted")
	}
	if extra, err := OpenStore(s.Store.Dir); err == nil {
		extra.Close()
		t.Fatal("source lock lost")
	}
	restored, err := OpenStore(out)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var count int
	var encrypted string
	if err = restored.DB.QueryRow("SELECT count(*) FROM files WHERE kind='file'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("snapshot boundary %d %v", count, err)
	}
	if err = restored.DB.QueryRow("SELECT secret FROM monitors WHERE name='backup'").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	plain, err := restored.decrypt(encrypted)
	if err != nil || plain != "backup secret" {
		t.Fatalf("key restore: %s %v", plain, err)
	}
	b, _ = os.ReadFile(filepath.Join(out, "uploads/objects", manifest.Objects[0].Key))
	if !bytes.Equal(b, body) {
		t.Fatal("object changed")
	}
}

func TestOnlineBackupRejectsMissingCorruptAndEscapingObjects(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "symlink", "key-change", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s, _, c := fixture(t)
			f := uploadForTest(t, c, "1", "object.txt", []byte("original"), 200)
			var key string
			if err := s.Store.DB.QueryRow("SELECT object_key FROM files WHERE id=?", f.ID).Scan(&key); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "failed")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := backup(ctx, s.Store.Dir, out, func() {
				object := filepath.Join(s.Store.Dir, "uploads/objects", key)
				switch kind {
				case "missing":
					os.Remove(object)
				case "corrupt":
					os.WriteFile(object, []byte("changed!"), 0600)
				case "symlink":
					os.Remove(object)
					outside := filepath.Join(t.TempDir(), "secret")
					os.WriteFile(outside, []byte("original"), 0600)
					os.Symlink(outside, object)
				case "key-change":
					os.WriteFile(filepath.Join(s.Store.Dir, "master.key"), bytes.Repeat([]byte{42}, 32), 0600)
				case "cancel":
					cancel()
				}
			})
			if err == nil {
				t.Fatal("incomplete backup succeeded")
			}
			if kind == "missing" && !strings.Contains(err.Error(), key) {
				t.Fatalf("missing object not listed: %v", err)
			}
			if _, err = os.Stat(filepath.Join(out, backupIncomplete)); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(out, "backup-manifest.json")); !os.IsNotExist(err) {
				t.Fatal("completion manifest exists")
			}
			if opened, err := OpenStore(out); err == nil {
				opened.Close()
				t.Fatal("incomplete restore accepted")
			}
		})
	}
}

func TestOnlineBackupDoesNotMigrateSource(t *testing.T) {
	dir, db := versionFourDatabase(t)
	if _, err := db.Exec(migration5); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "master.key"), bytes.Repeat([]byte{1}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "uploads/objects"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest, err := Backup(context.Background(), dir, filepath.Join(t.TempDir(), "old"))
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err = db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 || manifest.SchemaVersion != 5 {
		t.Fatalf("backup migrated database: %d %+v", version, manifest)
	}
}
