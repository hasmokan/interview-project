package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr, PublicURL, DataDir, WebDir, AdminUser, PasswordHash, TokenHash, SessionKey string
	MaxUpload                                                                        int64
	SecureCookie                                                                     bool
}

func Load() (Config, error) {
	c := Config{Addr: env("FILESHARE_ADDR", ":8080"), PublicURL: strings.TrimRight(env("FILESHARE_PUBLIC_URL", "http://localhost:8080"), "/"), DataDir: env("FILESHARE_DATA_DIR", "./data"), WebDir: env("FILESHARE_WEB_DIR", "./web/dist/client"), AdminUser: env("FILESHARE_ADMIN_USER", "admin"), PasswordHash: os.Getenv("FILESHARE_PASSWORD_HASH"), TokenHash: os.Getenv("FILESHARE_TOKEN_HASH"), SessionKey: os.Getenv("FILESHARE_SESSION_KEY"), MaxUpload: 100 * 1024 * 1024}
	if v := os.Getenv("FILESHARE_MAX_UPLOAD_BYTES"); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n <= 0 || n > 10*1024*1024*1024 {
			return c, errors.New("FILESHARE_MAX_UPLOAD_BYTES must be between 1 and 10737418240")
		}
		c.MaxUpload = n
	}
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return c, errors.New("FILESHARE_PUBLIC_URL must be an http(s) origin without a path")
	}
	c.SecureCookie = u.Scheme == "https"
	if c.PasswordHash == "" || len(c.SessionKey) < 32 {
		return c, errors.New("FILESHARE_PASSWORD_HASH and FILESHARE_SESSION_KEY (at least 32 characters) are required")
	}
	b, e := hex.DecodeString(c.TokenHash)
	if e != nil || len(b) != 32 {
		return c, fmt.Errorf("FILESHARE_TOKEN_HASH must be a SHA-256 hex digest")
	}
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
