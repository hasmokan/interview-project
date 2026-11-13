package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openTest(t *testing.T, root string) *Store {
	t.Helper()
	s, e := Open(root, "http://localhost:8080", 1024)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestFileLifecycleAndPersistence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := openTest(t, root)
	f, e := s.Upload(ctx, "report.pdf", strings.NewReader("%PDF-1.7\nhello"))
	if e != nil {
		t.Fatal(e)
	}
	if f.ShareURL != nil || f.ShareStatus != "unshared" {
		t.Fatal("upload unexpectedly shared")
	}
	duplicate, e := s.Upload(ctx, "report.pdf", strings.NewReader("another"))
	if e != nil || duplicate.ID == f.ID {
		t.Fatal("same-name files not independent", e)
	}
	page, e := s.List(ctx, "report", 1, 20)
	if e != nil || page.Total != 2 {
		t.Fatal(page, e)
	}
	a, e := s.Share(ctx, f.ID)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Share(ctx, f.ID)
	if e != nil || *a.ShareURL != *b.ShareURL {
		t.Fatal("share not idempotent", e)
	}
	old := TokenFromURL(*a.ShareURL)
	if _, e = s.Public(ctx, old); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Revoke(ctx, f.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Revoke(ctx, f.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Public(ctx, old); !errors.Is(e, ErrNotFound) {
		t.Fatal("revoked token accessible", e)
	}
	fresh, e := s.Share(ctx, f.ID)
	if e != nil || *fresh.ShareURL == *a.ShareURL {
		t.Fatal("token reused", e)
	}
	s.Close()
	s = openTest(t, root)
	persisted, e := s.Get(ctx, f.ID)
	if e != nil || *persisted.ShareURL != *fresh.ShareURL {
		t.Fatal("share not persisted", e)
	}
	if _, e = s.Public(ctx, old); !errors.Is(e, ErrNotFound) {
		t.Fatal("revoked token resurrected")
	}
	if body, e := os.ReadFile(s.Path(persisted)); e != nil || string(body) != "%PDF-1.7\nhello" {
		t.Fatal("content not persisted", e)
	}
	if e = s.Delete(ctx, f.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(ctx, f.ID); e != nil {
		t.Fatal("delete not idempotent", e)
	}
	if _, e = s.Public(ctx, TokenFromURL(*fresh.ShareURL)); !errors.Is(e, ErrNotFound) {
		t.Fatal("deleted file accessible", e)
	}
	if _, e = s.Share(ctx, f.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("deleted file can be shared")
	}
	if _, e = os.Stat(s.Path(f)); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("content not removed", e)
	}
	if _, e = s.Get(ctx, f.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("deleted file in details")
	}
	page, e = s.List(ctx, "", 1, 20)
	if e != nil || page.Total != 1 {
		t.Fatal("deleted file in list", e)
	}
	rows, e := s.db.Query("PRAGMA foreign_key_list(shares)")
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("physical foreign key exists")
	}
}
func TestConcurrentShareAndDelete(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	f, e := s.Upload(ctx, "a", strings.NewReader("a"))
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	urls := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x, e := s.Share(ctx, f.ID)
			if e != nil {
				t.Error(e)
				return
			}
			urls <- *x.ShareURL
		}()
	}
	wg.Wait()
	close(urls)
	var first string
	for u := range urls {
		if first == "" {
			first = u
		}
		if u != first {
			t.Fatal("multiple active links")
		}
	}
	var count int
	if e = s.db.QueryRow("SELECT count(*) FROM shares WHERE revoked_at IS NULL").Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Share(ctx, f.ID)
			if e != nil && !errors.Is(e, ErrNotFound) {
				t.Error(e)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if e := s.Delete(ctx, f.ID); e != nil {
			t.Error(e)
		}
	}()
	wg.Wait()
	if e = s.db.QueryRow("SELECT count(*) FROM shares WHERE revoked_at IS NULL").Scan(&count); e != nil || count != 0 {
		t.Fatal("active share survived delete", count, e)
	}
}
func TestUploadLimitSearchAndCleanup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := openTest(t, root)
	if _, e := s.Upload(ctx, "big", strings.NewReader(strings.Repeat("x", 1025))); !errors.Is(e, ErrTooLarge) {
		t.Fatal(e)
	}
	for _, name := range []string{"100%_report", "100abreport", "../../report.pdf"} {
		if _, e := s.Upload(ctx, name, strings.NewReader("ok")); e != nil {
			t.Fatal(e)
		}
	}
	p, e := s.List(ctx, "%_", 1, 20)
	if e != nil || p.Total != 1 {
		t.Fatal("LIKE wildcards were not escaped", p, e)
	}
	p, e = s.List(ctx, "report", 1, 2)
	if e != nil || p.Total != 3 || len(p.Items) != 2 {
		t.Fatal("pagination", p, e)
	}
	for _, f := range p.Items {
		if strings.Contains(f.Name, "/") {
			t.Fatal("unsafe filename")
		}
	}
	if e = os.WriteFile(filepath.Join(root, "files", "orphan"), []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s = openTest(t, root)
	if _, e = os.Stat(filepath.Join(root, "files", "orphan")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("orphan not cleaned")
	}
	entries, e := os.ReadDir(filepath.Join(root, "tmp"))
	if e != nil || len(entries) != 0 {
		t.Fatal("temporary upload leak", e)
	}
}
