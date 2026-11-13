package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_init.sql
var migration string

var ErrNotFound = errors.New("file or share not found")
var ErrTooLarge = errors.New("file exceeds upload limit")

type File struct {
	ID          string         `json:"file_id"`
	Name        string         `json:"name"`
	MIME        string         `json:"mime_type"`
	Size        int64          `json:"size_bytes"`
	Created     int64          `json:"created_at"`
	ShareStatus string         `json:"share_status"`
	ShareURL    *string        `json:"share_url"`
	Key         string         `json:"-"`
	Token       sql.NullString `json:"-"`
}
type Page struct {
	Items    []File `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
type Store struct {
	db           *sql.DB
	root, public string
	max          int64
}

func Open(root, public string, max int64) (*Store, error) {
	for _, d := range []string{root, filepath.Join(root, "files"), filepath.Join(root, "tmp")} {
		if e := os.MkdirAll(d, 0700); e != nil {
			return nil, e
		}
	}
	db, e := sql.Open("sqlite", filepath.Join(root, "fileshare.db"))
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	if _, e = db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); e != nil {
		return fail(e)
	}
	var version int
	if e = db.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return fail(e)
	}
	if version > 1 {
		return fail(errors.New("database schema is newer than this server"))
	}
	if version == 0 {
		tx, e := db.Begin()
		if e != nil {
			return fail(e)
		}
		if _, e = tx.Exec(migration); e == nil {
			_, e = tx.Exec("PRAGMA user_version=1")
		}
		if e != nil {
			tx.Rollback()
			return fail(e)
		}
		if e = tx.Commit(); e != nil {
			return fail(e)
		}
	}
	if e = os.Chmod(filepath.Join(root, "fileshare.db"), 0600); e != nil {
		return fail(e)
	}
	s := &Store{db: db, root: root, public: strings.TrimRight(public, "/"), max: max}
	if e = s.cleanup(); e != nil {
		return fail(e)
	}
	return s, nil
}
func (s *Store) Close() error       { return s.db.Close() }
func (s *Store) Path(f File) string { return filepath.Join(s.root, "files", f.Key) }
func (s *Store) cleanup() error {
	// Called only at startup, before requests; one server owns a data directory.
	rows, e := s.db.Query("SELECT storage_key FROM files WHERE deleted_at IS NULL")
	if e != nil {
		return e
	}
	live := map[string]bool{}
	for rows.Next() {
		var k string
		if e = rows.Scan(&k); e != nil {
			rows.Close()
			return e
		}
		live[k] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, dir := range []string{"tmp", "files"} {
		entries, e := os.ReadDir(filepath.Join(s.root, dir))
		if e != nil {
			return e
		}
		for _, v := range entries {
			if !v.IsDir() && (dir == "tmp" || !live[v.Name()]) {
				if e = os.Remove(filepath.Join(s.root, dir, v.Name())); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func (s *Store) Upload(ctx context.Context, name string, r io.Reader) (File, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" || len(name) > 1024 {
		return File{}, errors.New("invalid filename")
	}
	tmp, e := os.CreateTemp(filepath.Join(s.root, "tmp"), "upload-*")
	if e != nil {
		return File{}, e
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	n, e := io.Copy(tmp, io.LimitReader(r, s.max+1))
	if e != nil {
		return File{}, e
	}
	if n > s.max {
		return File{}, ErrTooLarge
	}
	if _, e = tmp.Seek(0, 0); e != nil {
		return File{}, e
	}
	head := make([]byte, 512)
	hn, e := tmp.Read(head)
	if e != nil && e != io.EOF {
		return File{}, e
	}
	mime := http.DetectContentType(head[:hn])
	if e = tmp.Sync(); e != nil {
		return File{}, e
	}
	if e = tmp.Close(); e != nil {
		return File{}, e
	}
	f := File{ID: uuid.NewString(), Name: name, MIME: mime, Size: n, Created: time.Now().UnixMilli(), Key: uuid.NewString(), ShareStatus: "unshared"}
	path := s.Path(f)
	if e = os.Rename(tmp.Name(), path); e != nil {
		return File{}, e
	}
	_, e = s.db.ExecContext(ctx, "INSERT INTO files(id,name,storage_key,mime_type,size_bytes,created_at) VALUES(?,?,?,?,?,?)", f.ID, f.Name, f.Key, f.MIME, f.Size, f.Created)
	if e != nil {
		os.Remove(path)
		return File{}, e
	}
	return f, nil
}

const selectFile = `SELECT f.id,f.name,f.storage_key,f.mime_type,f.size_bytes,f.created_at,s.token FROM files f LEFT JOIN shares s ON s.file_id=f.id AND s.revoked_at IS NULL `

type scanner interface{ Scan(...any) error }

func (s *Store) scan(row scanner) (File, error) {
	var f File
	e := row.Scan(&f.ID, &f.Name, &f.Key, &f.MIME, &f.Size, &f.Created, &f.Token)
	if errors.Is(e, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	if e != nil {
		return f, e
	}
	f.ShareStatus = "unshared"
	if f.Token.Valid {
		f.ShareStatus = "shared"
		v := s.public + "/s/" + f.Token.String
		f.ShareURL = &v
	}
	return f, nil
}
func (s *Store) Get(ctx context.Context, id string) (File, error) {
	return s.scan(s.db.QueryRowContext(ctx, selectFile+"WHERE f.id=? AND f.deleted_at IS NULL", id))
}
func (s *Store) Public(ctx context.Context, token string) (File, error) {
	if len(token) != 43 {
		return File{}, ErrNotFound
	}
	return s.scan(s.db.QueryRowContext(ctx, selectFile+"WHERE s.token=? AND f.deleted_at IS NULL", token))
}
func (s *Store) List(ctx context.Context, search string, page, size int) (Page, error) {
	p := Page{Items: []File{}, Page: page, PageSize: size}
	term := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(search) + "%"
	// A read transaction gives count and rows a consistent snapshot.
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return p, e
	}
	defer tx.Rollback()
	where := `WHERE f.deleted_at IS NULL AND f.name LIKE ? ESCAPE '\'`
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM files f "+where, term).Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := tx.QueryContext(ctx, selectFile+where+" ORDER BY f.created_at DESC,f.id DESC LIMIT ? OFFSET ?", term, size, (page-1)*size)
	if e != nil {
		return p, e
	}
	for rows.Next() {
		f, e := s.scan(rows)
		if e != nil {
			rows.Close()
			return p, e
		}
		p.Items = append(p.Items, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return p, e
	}
	return p, tx.Commit()
}
func (s *Store) Share(ctx context.Context, id string) (File, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return File{}, e
	}
	defer tx.Rollback()
	f, e := s.scan(tx.QueryRowContext(ctx, selectFile+"WHERE f.id=? AND f.deleted_at IS NULL", id))
	if e != nil {
		return f, e
	}
	if !f.Token.Valid {
		b := make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			return f, e
		}
		token := base64.RawURLEncoding.EncodeToString(b)
		if _, e = tx.ExecContext(ctx, "INSERT INTO shares(id,file_id,token,created_at) VALUES(?,?,?,?)", uuid.NewString(), id, token, time.Now().UnixMilli()); e != nil {
			return f, e
		}
		u := s.public + "/s/" + token
		f.ShareStatus = "shared"
		f.ShareURL = &u
	}
	return f, tx.Commit()
}
func (s *Store) Revoke(ctx context.Context, id string) (File, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return File{}, e
	}
	defer tx.Rollback()
	f, e := s.scan(tx.QueryRowContext(ctx, selectFile+"WHERE f.id=? AND f.deleted_at IS NULL", id))
	if e != nil {
		return f, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE shares SET revoked_at=? WHERE file_id=? AND revoked_at IS NULL", time.Now().UnixMilli(), id); e != nil {
		return f, e
	}
	f.ShareStatus = "unshared"
	f.ShareURL = nil
	return f, tx.Commit()
}
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var key string
	var deleted sql.NullInt64
	e = tx.QueryRowContext(ctx, "SELECT storage_key,deleted_at FROM files WHERE id=?", id).Scan(&key, &deleted)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	now := time.Now().UnixMilli()
	if !deleted.Valid {
		if _, e = tx.ExecContext(ctx, "UPDATE files SET deleted_at=? WHERE id=?", now, id); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE shares SET revoked_at=? WHERE file_id=? AND revoked_at IS NULL", now, id); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	e = os.Remove(filepath.Join(s.root, "files", key))
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return fmt.Errorf("file unavailable; disk cleanup pending until restart: %w", e)
	}
	return nil
}
func TokenFromURL(raw string) string {
	u, _ := url.Parse(raw)
	if u == nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/s/")
}
