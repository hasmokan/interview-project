package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONFlagsConfigAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("FILESHARE_CONFIG", path)
	t.Setenv("FILESHARE_SERVER", "")
	t.Setenv("FILESHARE_TOKEN", "")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		if r.URL.Query().Get("search") != "a%_" {
			t.Error("search not preserved")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"items":[],"total":0}`)
	}))
	defer ts.Close()
	var out, errOut bytes.Buffer
	if code := Run([]string{"config", "set", "--server", ts.URL, "--token", "secret", "--json"}, &out, &errOut); code != 0 {
		t.Fatal(errOut.String())
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("insecure config permissions", e)
	}
	out.Reset()
	if code := Run([]string{"list", "--search", "a%_", "--json"}, &out, &errOut); code != 0 {
		t.Fatal(errOut.String())
	}
	var v map[string]any
	if e = json.Unmarshal(out.Bytes(), &v); e != nil {
		t.Fatal("stdout not JSON", out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"get", "--json"}, &out, &errOut); code == 0 || out.Len() != 0 || errOut.Len() == 0 {
		t.Fatal("invalid args not reported correctly")
	}
}
func TestUploadPartialShareFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.pdf")
	os.WriteFile(path, []byte("%PDF-1.7"), 0600)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/files" {
			if e := r.ParseMultipartForm(1024); e != nil {
				t.Error(e)
			}
			defer r.MultipartForm.RemoveAll()
			f, _, e := r.FormFile("file")
			if e != nil {
				t.Error(e)
			} else {
				f.Close()
			}
			io.WriteString(w, `{"file_id":"uploaded-id"}`)
		} else {
			w.WriteHeader(500)
			io.WriteString(w, `{"code":"internal_error","message":"sharing failed"}`)
		}
	}))
	defer ts.Close()
	t.Setenv("FILESHARE_SERVER", ts.URL)
	t.Setenv("FILESHARE_TOKEN", "secret")
	t.Setenv("FILESHARE_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	var out, errOut bytes.Buffer
	code := Run([]string{"upload", path, "--share", "--json"}, &out, &errOut)
	if code == 0 || out.Len() != 0 || !strings.Contains(errOut.String(), "file_id=uploaded-id") || !strings.Contains(errOut.String(), "retry fileshare share uploaded-id") {
		t.Fatal("missing recovery details", code, out.String(), errOut.String())
	}
}
func TestRedirectDoesNotLeakToken(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer ts.Close()
	t.Setenv("FILESHARE_SERVER", ts.URL)
	t.Setenv("FILESHARE_TOKEN", "secret")
	t.Setenv("FILESHARE_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	var out, errOut bytes.Buffer
	if Run([]string{"get", "id", "--json"}, &out, &errOut) == 0 || hit {
		t.Fatal("redirect followed")
	}
}
