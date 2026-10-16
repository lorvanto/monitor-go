package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

//go:embed migration2.sql
var migration2 string

//go:embed migration3.sql
var migration3 string

//go:embed migration4.sql
var migration4 string

//go:embed migration5.sql
var migration5 string

//go:embed migration6.sql
var migration6 string

//go:embed migration7.sql
var migration7 string

//go:embed migration8.sql
var migration8 string

const currentSchemaVersion = 8

type Store struct {
	checkWake        chan struct{}
	notificationWake chan struct{}
	DB               *sql.DB
	readDB           *sql.DB
	lock             *flock.Flock
	seal             cipher.AEAD
	Dir              string
	Files            *os.Root
}

func OpenStore(dir string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if _, markerErr := os.Lstat(filepath.Join(abs, backupIncomplete)); markerErr == nil {
		return nil, errors.New("incomplete backup directory cannot be opened; create a new complete backup")
	} else if !errors.Is(markerErr, os.ErrNotExist) {
		return nil, markerErr
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(abs, "instance.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("此数据目录已有服务运行")
	}
	s := &Store{lock: lock, Dir: abs, checkWake: make(chan struct{}, 1), notificationWake: make(chan struct{}, 1)}
	success := false
	defer func() {
		if !success {
			s.Close()
		}
	}()
	path := filepath.Join(abs, "monitor.db")
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String()+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		return nil, err
	}
	s.DB = db
	// The writer remains single-connection; transactions never include network I/O.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err = db.Ping(); err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		tx, e := db.Begin()
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(schema); e != nil {
			tx.Rollback()
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
	}
	var version int
	if err = db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
		return nil, err
	}
	for version >= 1 && version < currentSchemaVersion {
		migrations := []string{migration2, migration3, migration4, migration5, migration6, migration7, migration8}
		tx, e := db.Begin()
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(migrations[version-1]); e != nil {
			tx.Rollback()
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		version++
	}
	if version != currentSchemaVersion {
		return nil, fmt.Errorf("不支持的数据库版本 %d", version)
	}
	keyPath := filepath.Join(abs, "master.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		var n int
		if err = db.QueryRow("SELECT (SELECT count(*) FROM monitors WHERE secret <> '') + (SELECT count(*) FROM notification_channels)").Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, errors.New("加密主密钥丢失，请恢复备份")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		err = os.WriteFile(keyPath, key, 0600)
	}
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	s.seal, err = cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err = s.seed(); err != nil {
		return nil, err
	}
	uploads := filepath.Join(abs, "uploads")
	if err = os.MkdirAll(uploads, 0700); err != nil {
		return nil, err
	}
	s.Files, err = os.OpenRoot(uploads)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"objects", "staging", "cache"} {
		if err = s.Files.MkdirAll(name, 0700); err != nil {
			return nil, err
		}
	}
	// Open readers only after migrations and seed have committed. Both URI mode and
	// per-connection query_only prevent accidental writes through this pool.
	s.readDB, err = sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String()+"?mode=ro&_pragma=query_only(1)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	s.readDB.SetMaxOpenConns(2)
	s.readDB.SetMaxIdleConns(2)
	if err = s.readDB.Ping(); err != nil {
		return nil, err
	}
	success = true
	return s, nil
}

// Only reporting queries use readers. Transaction callbacks must read through tx
// to observe their own uncommitted writes; authorization and coordination stay on DB.
type readQueries interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) reader() readQueries {
	if s.readDB != nil {
		return s.readDB
	}
	return s.DB
}
func (s *Store) Close() {
	if s.readDB != nil && s.readDB != s.DB {
		s.readDB.Close()
	}
	if s.Files != nil {
		s.Files.Close()
	}
	if s.DB != nil {
		s.DB.Close()
	}
	if s.lock != nil {
		s.lock.Unlock()
	}
}
func (s *Store) encrypt(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	nonce := make([]byte, s.seal.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return "", e
	}
	return base64.RawStdEncoding.EncodeToString(s.seal.Seal(nonce, nonce, []byte(v), nil)), nil
}
func (s *Store) decrypt(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	b, e := base64.RawStdEncoding.DecodeString(v)
	if e != nil || len(b) < s.seal.NonceSize() {
		return "", errors.New("无效的加密凭据")
	}
	v2, e := s.seal.Open(nil, b[:s.seal.NonceSize()], b[s.seal.NonceSize():], nil)
	return string(v2), e
}
func (s *Store) write(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = fn(tx); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	s.wakeWorkers()
	return nil
}
func now() int64 { return time.Now().UTC().UnixMilli() }
func (s *Store) seed() error {
	return s.write(context.Background(), func(tx *sql.Tx) error {
		for i, r := range []struct {
			name, description string
			perms             []string
		}{{"超级管理员", "管理平台全部功能", allPermissions()}, {"运维工程师", "维护主机、检测目标和告警", []string{"host:read", "host:update", "monitor:read", "monitor:update", "monitor:run", "alert:read", "alert:handle"}}, {"只读成员", "查看监控状态和历史数据", []string{"host:read", "monitor:read", "alert:read"}}} {
			res, e := tx.Exec("INSERT OR IGNORE INTO roles(id,name,description,builtin) VALUES(?,?,?,?)", i+1, r.name, r.description, i == 0)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				continue
			}
			for _, p := range r.perms {
				if _, e = tx.Exec("INSERT INTO role_permissions VALUES(?,?)", i+1, p); e != nil {
					return e
				}
			}
		}
		if _, e := tx.Exec("UPDATE notification_deliveries SET state='queued' WHERE state='sending'"); e != nil {
			return e
		}
		_, e := tx.Exec("UPDATE jobs SET state='queued' WHERE state='running'")
		return e
	})
}
