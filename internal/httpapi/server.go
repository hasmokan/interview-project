package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"interview-project/internal/config"
	"interview-project/internal/store"
)

type Server struct {
	cfg   config.Config
	store *store.Store
}
type session struct {
	Exp   int64  `json:"exp"`
	Nonce string `json:"nonce"`
}

const cookieName = "fileshare_session"

func New(cfg config.Config, s *store.Store) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) { fail(c, 500, "internal_error", "服务暂时不可用") }))
	r.SetTrustedProxies(nil)
	app := &Server{cfg: cfg, store: s}
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Cache-Control", "no-store")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Next()
	})
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.POST("/api/login", app.login)
	a := r.Group("/api", app.auth)
	a.GET("/session", func(c *gin.Context) {
		c.JSON(200, gin.H{"username": cfg.AdminUser, "csrf_token": c.GetString("csrf"), "max_upload_bytes": cfg.MaxUpload})
	})
	a.POST("/logout", func(c *gin.Context) { app.setCookie(c, "", -1); c.JSON(200, gin.H{"ok": true}) })
	a.POST("/files", app.upload)
	a.GET("/files", app.list)
	a.GET("/files/:id", func(c *gin.Context) { f, e := s.Get(c.Request.Context(), c.Param("id")); respond(c, f, e) })
	a.POST("/files/:id/share", func(c *gin.Context) { f, e := s.Share(c.Request.Context(), c.Param("id")); respond(c, f, e) })
	a.DELETE("/files/:id/share", func(c *gin.Context) { f, e := s.Revoke(c.Request.Context(), c.Param("id")); respond(c, f, e) })
	a.DELETE("/files/:id", func(c *gin.Context) {
		if e := s.Delete(c.Request.Context(), c.Param("id")); e != nil {
			respond(c, nil, e)
			return
		}
		c.JSON(200, gin.H{"file_id": c.Param("id"), "deleted": true})
	})
	r.GET("/api/public/shares/:token", func(c *gin.Context) {
		f, e := s.Public(c.Request.Context(), c.Param("token"))
		if e != nil {
			respond(c, nil, e)
			return
		}
		f.ShareURL = nil
		c.JSON(200, f)
	})
	r.GET("/api/public/shares/:token/content", app.content)
	r.HEAD("/api/public/shares/:token/content", app.content)
	r.NoRoute(app.static)
	return r
}
func fail(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"code": code, "message": msg})
}
func respond(c *gin.Context, v any, e error) {
	if e == nil {
		c.JSON(200, v)
		return
	}
	if errors.Is(e, store.ErrNotFound) {
		fail(c, 404, "not_found", "文件不存在或分享已失效")
	} else if errors.Is(e, store.ErrTooLarge) {
		fail(c, 413, "file_too_large", "文件超过上传大小限制")
	} else {
		fail(c, 500, "internal_error", "操作失败，请稍后重试")
	}
}
func (s *Server) validOrigin(c *gin.Context) bool {
	origin := c.GetHeader("Origin")
	return (origin == "" || origin == s.cfg.PublicURL) && c.GetHeader("Sec-Fetch-Site") != "cross-site"
}
func (s *Server) mac(v string) string {
	h := hmac.New(sha256.New, []byte(s.cfg.SessionKey))
	h.Write([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *Server) setCookie(c *gin.Context, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: s.cfg.SecureCookie, SameSite: http.SameSiteStrictMode})
}
func (s *Server) login(c *gin.Context) {
	if !s.validOrigin(c) {
		fail(c, 403, "forbidden", "请求来源无效")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if e := c.ShouldBindJSON(&in); e != nil {
		fail(c, 400, "invalid_request", "请输入账号和密码")
		return
	}
	// Always compare the password, regardless of the supplied username.
	err := bcrypt.CompareHashAndPassword([]byte(s.cfg.PasswordHash), []byte(in.Password))
	if subtle.ConstantTimeCompare([]byte(in.Username), []byte(s.cfg.AdminUser)) != 1 || err != nil {
		fail(c, 401, "unauthorized", "账号或密码错误")
		return
	}
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		respond(c, nil, e)
		return
	}
	sess := session{Exp: time.Now().Add(12 * time.Hour).Unix(), Nonce: base64.RawURLEncoding.EncodeToString(b)}
	raw, _ := json.Marshal(sess)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	s.setCookie(c, payload+"."+s.mac(payload), 12*3600)
	c.JSON(200, gin.H{"username": s.cfg.AdminUser, "csrf_token": s.mac("csrf:" + sess.Nonce), "max_upload_bytes": s.cfg.MaxUpload})
}
func (s *Server) auth(c *gin.Context) {
	if header := c.GetHeader("Authorization"); header != "" {
		token, ok := strings.CutPrefix(header, "Bearer ")
		sum := sha256.Sum256([]byte(token))
		expected, _ := hex.DecodeString(s.cfg.TokenHash)
		if !ok || token == "" || subtle.ConstantTimeCompare(sum[:], expected) != 1 {
			fail(c, 401, "unauthorized", "访问令牌无效")
			return
		}
		c.Next()
		return
	}
	raw, e := c.Cookie(cookieName)
	parts := strings.Split(raw, ".")
	if e != nil || len(parts) != 2 || !hmac.Equal([]byte(s.mac(parts[0])), []byte(parts[1])) {
		fail(c, 401, "unauthorized", "请先登录")
		return
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	var sess session
	if e != nil || json.Unmarshal(b, &sess) != nil || sess.Exp <= time.Now().Unix() || sess.Nonce == "" {
		fail(c, 401, "unauthorized", "登录已过期，请重新登录")
		return
	}
	csrf := s.mac("csrf:" + sess.Nonce)
	c.Set("csrf", csrf)
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" && (!s.validOrigin(c) || !hmac.Equal([]byte(c.GetHeader("X-CSRF-Token")), []byte(csrf))) {
		fail(c, 403, "forbidden", "请求校验失败，请刷新页面")
		return
	}
	c.Next()
}
func (s *Server) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.cfg.MaxUpload+1024*1024)
	err := c.Request.ParseMultipartForm(8 * 1024 * 1024)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			respond(c, nil, store.ErrTooLarge)
		} else {
			fail(c, 400, "invalid_request", "需要 multipart/form-data 文件上传")
		}
		return
	}
	form := c.Request.MultipartForm
	if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value) > 0 {
		fail(c, 400, "invalid_request", "每次只允许上传一个 file 字段")
		return
	}
	part := form.File["file"][0]
	if part.Size > s.cfg.MaxUpload {
		respond(c, nil, store.ErrTooLarge)
		return
	}
	file, e := part.Open()
	if e != nil {
		respond(c, nil, e)
		return
	}
	defer file.Close()
	f, e := s.store.Upload(c.Request.Context(), part.Filename, file)
	if e != nil {
		respond(c, nil, e)
		return
	}
	c.JSON(201, f)
}
func (s *Server) list(c *gin.Context) {
	page, e := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, se := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if e != nil || se != nil || page < 1 || page > 1000000 || size < 1 || size > 100 || len(c.Query("search")) > 1024 {
		fail(c, 400, "invalid_request", "分页或搜索参数无效")
		return
	}
	p, e := s.store.List(c.Request.Context(), c.Query("search"), page, size)
	respond(c, p, e)
}
func CanPreview(m string) bool {
	switch m {
	case "application/pdf", "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}
func (s *Server) content(c *gin.Context) {
	f, e := s.store.Public(c.Request.Context(), c.Param("token"))
	if e != nil {
		respond(c, nil, e)
		return
	}
	file, e := os.Open(s.store.Path(f))
	if e != nil {
		fail(c, 404, "not_found", "文件不存在或分享已失效")
		return
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil {
		respond(c, nil, e)
		return
	}
	disp := "attachment"
	if CanPreview(f.MIME) && c.Query("download") != "1" {
		disp = "inline"
	}
	c.Header("Content-Type", f.MIME)
	c.Header("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.Name}))
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	http.ServeContent(c.Writer, c.Request, f.Name, info.ModTime(), file)
}
func (s *Server) static(c *gin.Context) {
	path := c.Request.URL.Path
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		fail(c, 404, "not_found", "接口不存在")
		return
	}
	if strings.HasPrefix(path, "/assets/") {
		// Only build assets, never the data directory or arbitrary filesystem paths.
		clean := filepath.Clean(strings.TrimPrefix(path, "/assets/"))
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			fail(c, 404, "not_found", "资源不存在")
			return
		}
		p := filepath.Join(s.cfg.WebDir, "assets", clean)
		info, e := os.Stat(p)
		if e == nil && info.Mode().IsRegular() {
			c.File(p)
			return
		}
		fail(c, 404, "not_found", "资源不存在")
		return
	}
	if path != "/" && path != "/login" && !validSharePath(path) {
		fail(c, 404, "not_found", "页面不存在")
		return
	}
	shell := filepath.Join(s.cfg.WebDir, "_shell.html")
	if _, e := os.Stat(shell); e != nil {
		fail(c, 503, "frontend_unavailable", "请先构建前端资源")
		return
	}
	c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'self'")
	c.File(shell)
}
func validSharePath(path string) bool {
	if !strings.HasPrefix(path, "/s/") {
		return false
	}
	t := strings.TrimPrefix(path, "/s/")
	b, e := base64.RawURLEncoding.DecodeString(t)
	return e == nil && len(b) == 32
}

// Hash helpers read secrets from stdin rather than process arguments.
func HashInput(kind string, r io.Reader, w io.Writer) error {
	b, e := io.ReadAll(io.LimitReader(r, 4096))
	if e != nil {
		return e
	}
	v := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	if v == "" {
		return errors.New("empty secret")
	}
	switch kind {
	case "hash-password":
		h, e := bcrypt.GenerateFromPassword([]byte(v), bcrypt.DefaultCost)
		if e != nil {
			return e
		}
		_, e = fmt.Fprintln(w, string(h))
		return e
	case "hash-token":
		h := sha256.Sum256([]byte(v))
		_, e = fmt.Fprintf(w, "%x\n", h)
		return e
	}
	return errors.New("unknown command")
}
