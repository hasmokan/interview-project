package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}
type Client struct {
	Config
	HTTP *http.Client
}

func configPath() (string, error) {
	if v := os.Getenv("FILESHARE_CONFIG"); v != "" {
		return v, nil
	}
	d, e := os.UserConfigDir()
	return filepath.Join(d, "fileshare", "config.json"), e
}
func load() (Config, error) {
	p, e := configPath()
	if e != nil {
		return Config{}, e
	}
	var c Config
	b, e := os.ReadFile(p)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return c, e
	}
	if len(b) > 0 {
		if e = json.Unmarshal(b, &c); e != nil {
			return c, errors.New("invalid fileshare config")
		}
	}
	if v := os.Getenv("FILESHARE_SERVER"); v != "" {
		c.Server = v
	}
	if v := os.Getenv("FILESHARE_TOKEN"); v != "" {
		c.Token = v
	}
	return c, nil
}
func validServer(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.Fragment == ""
}
func (c Client) request(method, path string, body io.Reader, ctype string) (map[string]any, error) {
	req, e := http.NewRequest(method, strings.TrimRight(c.Server, "/")+path, body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, e := c.HTTP.Do(req)
	if e != nil {
		return nil, errors.New("request failed: check server address, connectivity and timeout")
	}
	defer res.Body.Close()
	var out map[string]any
	if e = json.NewDecoder(io.LimitReader(res.Body, 16*1024*1024)).Decode(&out); e != nil {
		return nil, fmt.Errorf("invalid API response (HTTP %d)", res.StatusCode)
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("%v: %v (HTTP %d)", out["code"], out["message"], res.StatusCode)
	}
	return out, nil
}
func (c Client) upload(path string) (map[string]any, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("cannot open local file")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return nil, errors.New("upload path must be a regular file")
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	mw := multipart.NewWriter(writer)
	go func() {
		part, e := mw.CreateFormFile("file", filepath.Base(path))
		if e == nil {
			_, e = io.Copy(part, f)
		}
		if e == nil {
			e = mw.Close()
		}
		writer.CloseWithError(e)
	}()
	return c.request("POST", "/api/files", reader, mw.FormDataContentType())
}

// Parse interspersed flags so both `get ID --json` and `get --json ID` work.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var opts, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		opts = append(opts, a)
		name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
		f := fs.Lookup(name)
		if f == nil {
			return nil, fmt.Errorf("unknown flag --%s", name)
		}
		boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
		if !strings.Contains(a, "=") && (!ok || !boolean.IsBoolFlag()) {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("missing value for --%s", name)
			}
			opts = append(opts, args[i])
		}
	}
	return pos, fs.Parse(opts)
}
func Run(args []string, out, errOut io.Writer) int {
	if e := run(args, out); e != nil {
		fmt.Fprintln(errOut, e)
		return 1
	}
	return 0
}
func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(out, "fileshare config set --server URL --token TOKEN\nfileshare upload PATH [--share] [--json]\nfileshare list [--search NAME] [--page N] [--page-size N] [--json]\nfileshare get|share|revoke|delete FILE_ID [--json]\nEnvironment: FILESHARE_SERVER, FILESHARE_TOKEN, FILESHARE_CONFIG")
		return nil
	}
	command := args[0]
	rest := args[1:]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	js := fs.Bool("json", false, "structured result")
	timeout := fs.Duration("timeout", 2*time.Minute, "request timeout")
	var server, token, search *string
	var share *bool
	var page, size *int
	switch command {
	case "config":
		if len(rest) == 0 || rest[0] != "set" {
			return errors.New("usage: fileshare config set --server URL --token TOKEN")
		}
		rest = rest[1:]
		server = fs.String("server", "", "API origin")
		token = fs.String("token", "", "access token")
	case "upload":
		share = fs.Bool("share", false, "share after upload")
	case "list":
		search = fs.String("search", "", "filename contains")
		page = fs.Int("page", 1, "page number")
		size = fs.Int("page-size", 20, "page size")
	case "get", "share", "revoke", "delete":
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	pos, e := parse(fs, rest)
	if e != nil {
		return e
	}
	if *timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if command == "config" {
		if len(pos) != 0 {
			return errors.New("unexpected arguments")
		}
		c, e := load()
		if e != nil {
			return e
		}
		if *server != "" {
			c.Server = *server
		}
		if *token != "" {
			c.Token = *token
		}
		if !validServer(c.Server) || c.Token == "" {
			return errors.New("valid server origin and token are required")
		}
		p, e := configPath()
		if e != nil {
			return e
		}
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		b, _ := json.MarshalIndent(c, "", "  ")
		f, e := os.CreateTemp(filepath.Dir(p), ".config-*")
		if e != nil {
			return e
		}
		defer os.Remove(f.Name())
		if _, e = f.Write(b); e != nil {
			f.Close()
			return e
		}
		if e = f.Close(); e != nil {
			return e
		}
		if e = os.Rename(f.Name(), p); e != nil {
			return e
		}
		return output(out, map[string]any{"configured": true, "server": c.Server}, *js)
	}
	cfg, e := load()
	if e != nil {
		return e
	}
	if !validServer(cfg.Server) || cfg.Token == "" {
		return errors.New("configure FILESHARE_SERVER and FILESHARE_TOKEN or run config set first")
	}
	c := Client{Config: cfg, HTTP: &http.Client{Timeout: *timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var result map[string]any
	if command == "list" {
		if len(pos) != 0 {
			return errors.New("list does not accept positional arguments")
		}
		q := url.Values{"search": {*search}, "page": {strconv.Itoa(*page)}, "page_size": {strconv.Itoa(*size)}}
		result, e = c.request("GET", "/api/files?"+q.Encode(), nil, "")
	} else {
		if len(pos) != 1 {
			return fmt.Errorf("%s requires exactly one file path or file ID", command)
		}
		if command == "upload" {
			result, e = c.upload(pos[0])
			if e == nil && *share {
				id, _ := result["file_id"].(string)
				result, e = c.request("POST", "/api/files/"+url.PathEscape(id)+"/share", nil, "")
				if e != nil {
					return fmt.Errorf("upload succeeded (file_id=%s), sharing failed; retry fileshare share %s --json: %w", id, id, e)
				}
			}
		} else {
			path := "/api/files/" + url.PathEscape(pos[0])
			method := "GET"
			switch command {
			case "share":
				path += "/share"
				method = "POST"
			case "revoke":
				path += "/share"
				method = "DELETE"
			case "delete":
				method = "DELETE"
			}
			result, e = c.request(method, path, nil, "")
		}
	}
	if e != nil {
		return e
	}
	return output(out, result, *js)
}
func output(w io.Writer, v any, compact bool) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	if !compact {
		enc.SetIndent("", "  ")
	}
	enc.SetEscapeHTML(false)
	if e := enc.Encode(v); e != nil {
		return e
	}
	_, e := w.Write(b.Bytes())
	return e
}
