package dashboard

import (
	"cmp"
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed public/*
var webFiles embed.FS

type config struct {
	Host, Port, Origin, State, Root, Home, Pricing string
	Share                                          bool
}
type attempt struct {
	Count int
	Until time.Time
}
type application struct {
	config     config
	local      *localStore
	account    *accountClient
	salt, hash []byte
	mu         sync.Mutex
	sessions   map[string]time.Time
	attempts   map[string]attempt
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func passwordHash(password string, salt []byte) []byte {
	key, err := pbkdf2.Key(sha256.New, password, salt, 600_000, 32)
	if err != nil {
		panic(err)
	}
	return key
}
func newApplication(ctx context.Context, c config, password string) (*application, error) {
	if c.Origin != "" {
		u, err := url.Parse(c.Origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("PUBLIC_ORIGIN 必须是 HTTPS 站点地址")
		}
		c.Origin = "https://" + u.Host
	}
	ip := net.ParseIP(c.Host)
	if (ip == nil || !ip.IsLoopback()) && c.Host != "localhost" && c.Origin == "" {
		return nil, errors.New("远程监听需要 HTTPS PUBLIC_ORIGIN")
	}
	if err := os.MkdirAll(c.State, 0700); err != nil {
		return nil, err
	}
	if len(password) < 16 {
		return nil, errors.New("仪表盘密码至少需要 16 个字符")
	}
	salt := []byte(randomToken())
	local, err := newLocalStore(ctx, c.Root, c.State)
	if err != nil {
		return nil, err
	}
	app := &application{config: c, local: local, account: newAccountClient(ctx, c.Root), salt: salt, hash: passwordHash(password, salt), sessions: map[string]time.Time{}, attempts: map[string]attempt{}}
	if c.Share {
		local.public = new(publicSnapshot)
		if data, _, _ := local.view(); data != nil {
			if err := local.public.publish(data); err != nil {
				return nil, err
			}
		}
	}
	return app, nil
}
func (a *application) validHost(authority string) bool {
	if a.config.Origin != "" {
		u, _ := url.Parse(a.config.Origin)
		if authority == u.Host {
			return true
		}
	}
	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		host, port = authority, "80"
	}
	host = strings.ToLower(host)
	if host != "localhost" && host != "127.0.0.1" && host != "::1" && host != "[::1]" {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535 && (a.config.Origin == "" || port == a.config.Port)
}
func (a *application) validOrigin(r *http.Request) bool {
	expected := cmp.Or(a.config.Origin, "http://"+r.Host)
	return (r.Header.Get("Origin") == "" || r.Header.Get("Origin") == expected) && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}
func (a *application) cookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "dashboard_session", Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.config.Origin != "", MaxAge: maxAge})
}
func sessionID(r *http.Request) string {
	cookie, err := r.Cookie("dashboard_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}
func (a *application) authenticated(r *http.Request) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := sessionID(r)
	expiry := a.sessions[id]
	if time.Now().Before(expiry) {
		return true
	}
	delete(a.sessions, id)
	return false
}
func sendJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		http.Error(w, "响应编码失败", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{"error": message})
}

func (a *application) handler() http.Handler {
	mux := http.NewServeMux()
	assets := map[string]string{"/": "index.html", "/app.js": "app.js", "/style.css": "style.css", "/favicon.svg": "favicon.svg"}
	for path, file := range assets {
		pattern := "GET " + path
		if path == "/" {
			pattern = "GET /{$}"
		}
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			raw, err := webFiles.ReadFile("public/" + file)
			if err != nil {
				sendError(w, 500, "页面无法读取")
				return
			}
			types := map[string]string{".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml"}
			w.Header().Set("Content-Type", types[filepath.Ext(file)])
			_, _ = w.Write(raw)
		})
	}
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, 200, map[string]any{"authenticated": a.authenticated(r), "timezone": time.Local.String(), "publicShare": a.config.Share})
	})
	mux.HandleFunc("GET /share", a.serveShare)
	mux.HandleFunc("GET /share/{component}", a.serveShare)
	mux.HandleFunc("POST /api/login", a.login)
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		delete(a.sessions, sessionID(r))
		a.mu.Unlock()
		a.cookie(w, "", -1)
		sendJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/local", a.localUsage)
	mux.HandleFunc("GET /api/limits", func(w http.ResponseWriter, r *http.Request) {
		tokens, err := a.account.credentials()
		if err != nil {
			sendJSON(w, 200, remoteResult{Error: new(err.Error())})
			return
		}
		sendJSON(w, 200, a.account.read(tokens, "usage", nil, normalizeLimits, r.URL.Query().Get("refresh") == "1"))
	})
	mux.HandleFunc("GET /api/account", func(w http.ResponseWriter, r *http.Request) {
		if name := r.URL.Query().Get("period"); name == "24" || name == "custom5" {
			sendError(w, 400, "总统计仅支持自然日范围")
			return
		}
		window, err := periodRange(r.URL.Query().Get("period"), r.URL.Query().Get("end"), time.Now())
		if err != nil {
			sendError(w, 400, err.Error())
			return
		}
		sendJSON(w, 200, a.account.usage(window, r.URL.Query().Get("refresh") == "1"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { sendError(w, 404, "接口不存在") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if a.config.Origin != "" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !a.validHost(r.Host) {
			sendError(w, 403, "访问地址未配置")
			return
		}
		shared := (r.Method == http.MethodGet || r.Method == http.MethodHead) && (r.URL.Path == "/share" || strings.HasPrefix(r.URL.Path, "/share/"))
		if !shared && !a.validOrigin(r) {
			sendError(w, 403, "不允许跨站请求")
			return
		}
		_, asset := assets[r.URL.Path]
		public := shared || (asset && r.Method == http.MethodGet) || (r.URL.Path == "/api/session" && r.Method == http.MethodGet) || (r.URL.Path == "/api/login" && r.Method == http.MethodPost)
		if !public && !a.authenticated(r) {
			sendError(w, 401, "请先登录仪表盘")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *application) login(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	a.mu.Lock()
	maps.DeleteFunc(a.sessions, func(_ string, expiry time.Time) bool { return !now.Before(expiry) })
	maps.DeleteFunc(a.attempts, func(_ string, v attempt) bool { return !now.Before(v.Until) })
	entry := a.attempts[ip]
	if entry.Until.IsZero() {
		entry.Until = now.Add(15 * time.Minute)
	}
	total := 0
	for _, v := range a.attempts {
		total += v.Count
	}
	if entry.Count >= 10 || total >= 100 {
		a.mu.Unlock()
		sendError(w, 429, "登录尝试过多，请在 15 分钟后重试")
		return
	}
	// Reserve the attempt before expensive hashing so concurrent requests cannot bypass limits.
	entry.Count++
	a.attempts[ip] = entry
	a.mu.Unlock()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	var body struct {
		Password string `json:"password"`
	}
	if err != nil || json.Unmarshal(raw, &body) != nil {
		sendError(w, 400, "登录请求无效")
		return
	}
	if subtle.ConstantTimeCompare(passwordHash(body.Password, a.salt), a.hash) != 1 {
		sendError(w, 401, "密码不正确")
		return
	}
	a.mu.Lock()
	delete(a.attempts, ip)
	delete(a.sessions, sessionID(r))
	if len(a.sessions) >= 64 {
		for id := range a.sessions {
			delete(a.sessions, id)
			break
		}
	}
	id := randomToken()
	a.sessions[id] = now.Add(12 * time.Hour)
	a.mu.Unlock()
	a.cookie(w, id, 12*60*60)
	sendJSON(w, 200, map[string]bool{"ok": true})
}
func (a *application) localUsage(w http.ResponseWriter, r *http.Request) {
	window, err := periodRange(r.URL.Query().Get("period"), r.URL.Query().Get("end"), time.Now())
	if name := r.URL.Query().Get("period"); name == "quota5" || name == "quota7" {
		seconds := int64(604800)
		if name == "quota5" {
			seconds = 18000
		}
		window, err = a.account.quotaPeriod(seconds, r.URL.Query().Get("refresh") == "1")
	}
	if err != nil {
		sendError(w, 400, err.Error())
		return
	}
	data, refreshing, message := a.local.view()
	if data == nil || r.URL.Query().Get("refresh") == "1" {
		select {
		case <-a.local.refresh():
		case <-r.Context().Done():
			return
		}
		data, refreshing, message = a.local.view()
	}
	if data == nil {
		sendError(w, 503, cmp.Or(message, "正在建立本机缓存"))
		return
	}
	prices, err := loadPrices(a.config.Home, a.config.Pricing)
	if err != nil {
		log.Printf("价格读取失败: %v", err)
		sendError(w, 500, "价格文件无法读取，请检查价格配置")
		return
	}
	result := aggregateLocal(data, window, prices, loadCurrency(a.config.Home))
	result.Refreshing = refreshing
	result.Error = message
	sendJSON(w, 200, result)
}
func run() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root, err := codexHome("")
	if err != nil {
		return err
	}
	time.Local, err = localZone(os.Getenv("TZ"))
	if err != nil {
		return err
	}
	c := config{Host: cmp.Or(os.Getenv("HOST"), "127.0.0.1"), Port: cmp.Or(os.Getenv("PORT"), "4318"), Origin: os.Getenv("PUBLIC_ORIGIN"), State: cmp.Or(os.Getenv("DASHBOARD_STATE_DIR"), filepath.Join(filepath.Dir(executable), ".state-codex-tally")), Root: root, Home: home, Pricing: os.Getenv("PRICING_FILE")}
	c.Share = os.Getenv("PUBLIC_SHARE") == "1"
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	password := os.Getenv("CODEX_TALLY_PASSWORD")
	generated := password == ""
	if generated {
		password = randomToken()
	}
	app, err := newApplication(ctx, c, password)
	if err != nil {
		return err
	}
	app.local.run(time.Hour)
	server := &http.Server{Addr: net.JoinHostPort(c.Host, c.Port), Handler: app.handler(), ReadHeaderTimeout: 15 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: time.Minute, BaseContext: func(net.Listener) context.Context { return ctx }}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		stop()
		app.local.wg.Wait()
		return err
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	log.Printf("Codex 用量仪表盘：%s", cmp.Or(c.Origin, "http://localhost:"+c.Port))
	if generated {
		log.Printf("本次登录密码：%s", password)
	}
	password = ""
	select {
	case <-ctx.Done():
	case err = <-done:
		stop()
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(shutdown)
	if shutdownErr != nil {
		_ = server.Close()
	}
	app.local.wg.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, shutdownErr)
}
