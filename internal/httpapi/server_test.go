package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"interview-project/internal/config"
	"interview-project/internal/store"
)

func setup(t *testing.T) (http.Handler, *store.Store, config.Config) {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	token := sha256.Sum256([]byte("test-token"))
	web := t.TempDir()
	os.WriteFile(filepath.Join(web, "_shell.html"), []byte("<html>shell</html>"), 0600)
	cfg := config.Config{PublicURL: "http://localhost:8080", DataDir: t.TempDir(), WebDir: web, AdminUser: "admin", PasswordHash: string(hash), TokenHash: hex.EncodeToString(token[:]), SessionKey: strings.Repeat("k", 32), MaxUpload: 1024}
	s, e := store.Open(cfg.DataDir, cfg.PublicURL, cfg.MaxUpload)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return New(cfg, s), s, cfg
}
func call(h http.Handler, method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func bearer() map[string]string { return map[string]string{"Authorization": "Bearer test-token"} }
func upload(t *testing.T, h http.Handler, name, body string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	p, e := mw.CreateFormFile("file", name)
	if e != nil {
		t.Fatal(e)
	}
	io.WriteString(p, body)
	mw.Close()
	headers := bearer()
	headers["Content-Type"] = mw.FormDataContentType()
	return call(h, "POST", "/api/files", &b, headers)
}
func fileResult(t *testing.T, w *httptest.ResponseRecorder) store.File {
	t.Helper()
	if w.Code >= 300 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body)
	}
	var f store.File
	if e := json.Unmarshal(w.Body.Bytes(), &f); e != nil {
		t.Fatal(e)
	}
	return f
}
func TestHTTPSharingLifecycle(t *testing.T) {
	h, _, _ := setup(t)
	f := fileResult(t, upload(t, h, "报告.pdf", "%PDF-1.7\nhello world"))
	if f.ShareURL != nil {
		t.Fatal("upload shared")
	}
	f = fileResult(t, call(h, "POST", "/api/files/"+f.ID+"/share", nil, bearer()))
	old := store.TokenFromURL(*f.ShareURL)
	content := "/api/public/shares/" + old + "/content"
	w := call(h, "GET", content, nil, nil)
	if w.Code != 200 || w.Body.String() != "%PDF-1.7\nhello world" || w.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline") {
		t.Fatal(w.Code, w.Header(), w.Body)
	}
	w = call(h, "HEAD", content, nil, nil)
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD", w.Code)
	}
	w = call(h, "GET", content, nil, map[string]string{"Range": "bytes=0-4"})
	if w.Code != 206 || w.Body.String() != "%PDF-" {
		t.Fatal("range", w.Code, w.Body)
	}
	w = call(h, "GET", content+"?download=1", nil, nil)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("download", w.Header())
	}
	fileResult(t, call(h, "DELETE", "/api/files/"+f.ID+"/share", nil, bearer()))
	for _, method := range []string{"GET", "HEAD"} {
		w = call(h, method, content, nil, map[string]string{"Range": "bytes=0-4"})
		if w.Code != 404 {
			t.Fatal("revoked share accessible", method, w.Code)
		}
	}
	if w = call(h, "GET", "/api/public/shares/"+old, nil, nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
	f = fileResult(t, call(h, "POST", "/api/files/"+f.ID+"/share", nil, bearer()))
	if store.TokenFromURL(*f.ShareURL) == old {
		t.Fatal("reused token")
	}
	if w = call(h, "DELETE", "/api/files/"+f.ID, nil, bearer()); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = call(h, "GET", "/api/public/shares/"+store.TokenFromURL(*f.ShareURL)+"/content", nil, nil); w.Code != 404 {
		t.Fatal("deleted file accessible")
	}
}
func TestAllManagementRoutesRequireAuthentication(t *testing.T) {
	h, _, _ := setup(t)
	paths := [][2]string{{"POST", "/api/files"}, {"GET", "/api/files"}, {"GET", "/api/files/id"}, {"POST", "/api/files/id/share"}, {"DELETE", "/api/files/id/share"}, {"DELETE", "/api/files/id"}, {"GET", "/api/session"}, {"POST", "/api/logout"}}
	for _, p := range paths {
		for _, headers := range []map[string]string{nil, {"Authorization": "Bearer invalid"}, {"Cookie": cookieName + "=forged.value"}} {
			if w := call(h, p[0], p[1], nil, headers); w.Code != 401 {
				t.Errorf("%s %s: got %d", p[0], p[1], w.Code)
			}
		}
	}
}
func TestCookieAuthenticationAndCSRF(t *testing.T) {
	h, _, cfg := setup(t)
	headers := map[string]string{"Content-Type": "application/json", "Origin": cfg.PublicURL}
	bad := call(h, "POST", "/api/login", strings.NewReader(`{"username":"admin","password":"wrong"}`), headers)
	if bad.Code != 401 {
		t.Fatal(bad.Code)
	}
	w := call(h, "POST", "/api/login", strings.NewReader(`{"username":"admin","password":"test-password"}`), headers)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("bad cookie")
	}
	var sess map[string]any
	json.Unmarshal(w.Body.Bytes(), &sess)
	cookie := cookies[0].String()
	if w = call(h, "GET", "/api/session", nil, map[string]string{"Cookie": cookie}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for _, extra := range []map[string]string{{}, {"X-CSRF-Token": "wrong"}, {"X-CSRF-Token": fmt.Sprint(sess["csrf_token"]), "Origin": "https://evil.example"}} {
		extra["Cookie"] = cookie
		if w = call(h, "POST", "/api/logout", nil, extra); w.Code != 403 {
			t.Fatal("CSRF accepted", w.Code)
		}
	}
	w = call(h, "POST", "/api/logout", nil, map[string]string{"Cookie": cookie, "X-CSRF-Token": fmt.Sprint(sess["csrf_token"]), "Origin": cfg.PublicURL})
	if w.Code != 200 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout", w.Code)
	}
}
func TestLimitsAndNoStorageExposure(t *testing.T) {
	h, _, _ := setup(t)
	if w := upload(t, h, "big.txt", strings.Repeat("x", 1025)); w.Code != 413 {
		t.Fatal(w.Code, w.Body)
	}
	f := fileResult(t, upload(t, h, "page.html", "<!DOCTYPE html><script>alert(1)</script>"))
	f = fileResult(t, call(h, "POST", "/api/files/"+f.ID+"/share", nil, bearer()))
	w := call(h, "GET", "/api/public/shares/"+store.TokenFromURL(*f.ShareURL)+"/content", nil, nil)
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("active content inline")
	}
	for _, path := range []string{"/data/fileshare.db", "/files/secret", "/assets/missing.js", "/api/missing"} {
		if w = call(h, "GET", path, nil, nil); w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	if w = call(h, "GET", "/?x=1", nil, nil); w.Code != 200 {
		t.Fatal("SPA shell", w.Code)
	}
	if w = call(h, "GET", "/api/files?page=0", nil, bearer()); w.Code != 400 {
		t.Fatal("invalid pagination")
	}
}
