package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/lorvanto/monitor-go/internal/buildinfo"
)

const backupIncomplete = "BACKUP_INCOMPLETE"

type BackupObject struct {
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type BackupManifest struct {
	Format             int            `json:"format"`
	CreatedAt          time.Time      `json:"createdAt"`
	ApplicationVersion string         `json:"applicationVersion"`
	SchemaVersion      int            `json:"schemaVersion"`
	FileCount          int            `json:"fileCount"`
	DatabaseSHA256     string         `json:"databaseSHA256"`
	Objects            []BackupObject `json:"objects"`
}

// Backup opens the source in SQLite read-only mode without migrations or the
// instance lock. Only complete snapshots receive a manifest and lose the marker.
func Backup(ctx context.Context, source, destination string) (BackupManifest, error) {
	return backup(ctx, source, destination, nil)
}

// afterSnapshot lets file-race tests remove/restore an object deterministically.
func backup(ctx context.Context, source, destination string, afterSnapshot func()) (manifest BackupManifest, err error) {
	source, err = filepath.Abs(source)
	if err != nil {
		return manifest, err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return manifest, err
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return manifest, err
	}
	defer root.Close()
	for _, name := range []string{"monitor.db", "master.key"} {
		info, e := root.Lstat(name)
		if e != nil {
			return manifest, e
		}
		if !info.Mode().IsRegular() {
			return manifest, fmt.Errorf("backup source %s must be a regular file", name)
		}
	}
	key, err := root.ReadFile("master.key")
	if err != nil {
		return manifest, err
	}
	if len(key) != 32 {
		return manifest, errors.New("invalid master.key length")
	}
	// Mkdir (not MkdirAll) refuses existing destinations and missing parents.
	if err = os.Mkdir(destination, 0700); err != nil {
		return manifest, fmt.Errorf("create new backup directory: %w", err)
	}
	output, err := os.OpenRoot(destination)
	if err != nil {
		return manifest, err
	}
	defer output.Close()
	if err = output.WriteFile(backupIncomplete, []byte("Backup incomplete. Do not restore this directory.\n"), 0600); err != nil {
		return manifest, err
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.Join(source, "monitor.db")}).String()+"?mode=ro&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)")
	if err != nil {
		return manifest, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var sourceVersion int
	if err = db.QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&sourceVersion); err != nil {
		return manifest, err
	}
	if sourceVersion < 3 || sourceVersion > currentSchemaVersion {
		return manifest, fmt.Errorf("unsupported backup schema %d (supported: 3–%d); use a cold backup", sourceVersion, currentSchemaVersion)
	}
	// query_only is intentionally omitted: this driver rejects VACUUM INTO with
	// that pragma. mode=ro protects the source while allowing a separate output.
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(destination, "monitor.db")); err != nil {
		return manifest, err
	}
	if err = output.Chmod("monitor.db", 0600); err != nil {
		return manifest, err
	}
	snapshotDB, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.Join(destination, "monitor.db")}).String()+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return manifest, err
	}
	defer snapshotDB.Close()
	snapshotDB.SetMaxOpenConns(1)
	manifest = BackupManifest{Format: 1, CreatedAt: time.Now().UTC(), ApplicationVersion: buildinfo.Version, Objects: []BackupObject{}}
	if err = snapshotDB.QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&manifest.SchemaVersion); err != nil {
		return manifest, err
	}
	if manifest.SchemaVersion != sourceVersion {
		return manifest, errors.New("schema changed during backup; retry after the upgrade")
	}
	var integrity string
	if err = snapshotDB.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return manifest, err
	}
	if integrity != "ok" {
		return manifest, errors.New("snapshot integrity check failed")
	}
	rows, err := snapshotDB.QueryContext(ctx, "SELECT object_key,size,sha256 FROM files WHERE kind='file' AND purged_at IS NULL ORDER BY id")
	if err != nil {
		return manifest, err
	}
	for rows.Next() {
		var object BackupObject
		if err = rows.Scan(&object.Key, &object.Size, &object.SHA256); err != nil {
			rows.Close()
			return manifest, err
		}
		if !validObjectKey(object.Key) || !validObjectKey(object.SHA256) || object.Size < 0 || object.Size == 1<<63-1 {
			rows.Close()
			return manifest, errors.New("snapshot contains invalid object metadata")
		}
		manifest.Objects = append(manifest.Objects, object)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return manifest, err
	}
	if afterSnapshot != nil {
		afterSnapshot()
	}
	uploads, err := root.OpenRoot("uploads")
	if err != nil {
		return manifest, err
	}
	defer uploads.Close()
	if err = output.MkdirAll("uploads/objects", 0700); err != nil {
		return manifest, err
	}
	failures := []error{}
	for _, object := range manifest.Objects {
		if err = ctx.Err(); err != nil {
			return manifest, err
		}
		var copyErr error
		for attempt := 0; attempt < 3; attempt++ {
			copyErr = copyBackupObject(ctx, uploads, output, object)
			if copyErr == nil || !errors.Is(copyErr, os.ErrNotExist) {
				break
			}
			if attempt < 2 {
				select {
				case <-ctx.Done():
					return manifest, ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
		if copyErr != nil {
			failures = append(failures, fmt.Errorf("object %s: %w", object.Key, copyErr))
		}
	}
	if len(failures) > 0 {
		return manifest, fmt.Errorf("incomplete backup: %w", errors.Join(failures...))
	}
	currentKey, err := root.ReadFile("master.key")
	if err != nil {
		return manifest, err
	}
	if !bytes.Equal(key, currentKey) {
		return manifest, errors.New("master.key changed during backup")
	}
	if err = writeBackupFile(output, "master.key", key); err != nil {
		return manifest, err
	}
	snapshotFile, err := output.Open("monitor.db")
	if err != nil {
		return manifest, err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, snapshotFile)
	snapshotFile.Close()
	if err != nil {
		return manifest, err
	}
	manifest.DatabaseSHA256 = hex.EncodeToString(hash.Sum(nil))
	manifest.FileCount = len(manifest.Objects)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return manifest, err
	}
	if err = writeBackupFile(output, "backup-manifest.json", append(data, '\n')); err != nil {
		return manifest, err
	}
	if err = output.Remove(backupIncomplete); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func writeBackupFile(root *os.Root, name string, data []byte) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func copyBackupObject(ctx context.Context, source, output *os.Root, object BackupObject) error {
	input, err := source.Open("objects/" + object.Key)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("object is not a regular file")
	}
	target := "uploads/objects/" + object.Key
	file, err := output.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			output.Remove(target)
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(contextReader{ctx, input}, object.Size+1))
	if err != nil {
		return err
	}
	if n != object.Size || hex.EncodeToString(hash.Sum(nil)) != object.SHA256 {
		return errors.New("size or SHA-256 differs from snapshot")
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
