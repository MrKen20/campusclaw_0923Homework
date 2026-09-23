// Package auth 实现登录、登出、会话中间件与登录限流。
// 关键约束（design.md D2/D8、spec R1/R2）：
//   - 会话存服务端表，Cookie 只持不透明 ID；库中存 ID 的 HMAC（SESSION_SECRET）。
//   - 角色与班级每次请求从用户表读取，不采信客户端声明。
//   - 登录成功换发新会话 ID；登出删会话行立即失效。
//   - 账号不存在 / 口令错误 / 被限流三类失败响应完全同形。
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"campusclaw/backend/internal/config"
)

const CookieName = "campusclaw_session"

// User 是每次请求从数据库读出的授权上下文（用户、角色、班级三项齐备）。
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	ClassID   int64  `json:"class_id"`
	ClassName string `json:"class_name"`
}

type ctxKey struct{}

// FromContext 取出中间件注入的用户；未登录时 ok=false。
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// Service 持有认证所需的依赖。
type Service struct {
	DB      *sql.DB
	Cfg     *config.Config
	limiter *rateLimiter
}

func NewService(d *sql.DB, cfg *config.Config) *Service {
	return &Service{DB: d, Cfg: cfg, limiter: newRateLimiter(cfg.LoginFailLimit, cfg.LoginFailWindow)}
}

// sessionKey 把不透明会话 ID 映射为库中存储的 HMAC 值：
// 即使数据库泄露，攻击者也无法直接拿到可用的会话 ID。
func (s *Service) sessionKey(id string) string {
	mac := hmac.New(sha256.New, []byte(s.Cfg.SessionSecret))
	mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil))
}

func newSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- 登录 / 登出 / 身份 ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// dummyHash 用于"账号不存在"时的等价耗时比较，保证三类失败响应同形（spec R1）。
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

// Login 处理 POST /api/login。
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	key := req.Username + "|" + clientIP(r)
	// 限流命中：即使口令正确也拒绝，响应与口令错误完全同形（不暴露锁定状态）。
	limited := s.limiter.limited(key)

	var (
		id       int64
		hash     string
		role     string
		classID  int64
		username string
	)
	row := s.DB.QueryRow(`SELECT id, username, password_hash, role, class_id FROM users WHERE username = ?`, req.Username)
	err := row.Scan(&id, &username, &hash, &role, &classID)

	checkAgainst := dummyHash
	if err == nil {
		checkAgainst = []byte(hash)
	} else if err != sql.ErrNoRows {
		log.Printf("登录查询失败：%v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "服务暂时不可用"})
		return
	}
	// 无论账号是否存在都执行一次 bcrypt 比较，消除耗时差异。
	pwOK := bcrypt.CompareHashAndPassword(checkAgainst, []byte(req.Password)) == nil && err == nil

	if limited || !pwOK {
		if !pwOK {
			s.limiter.fail(key)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	s.limiter.reset(key)

	// 防会话固定：换发新会话 ID；若请求带了旧会话 Cookie 则一并作废。
	if old, err := r.Cookie(CookieName); err == nil && old.Value != "" {
		_, _ = s.DB.Exec(`DELETE FROM sessions WHERE id = ?`, s.sessionKey(old.Value))
	}
	sid, err := newSessionID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器错误"})
		return
	}
	expires := time.Now().Add(s.Cfg.SessionTTL)
	if _, err := s.DB.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		s.sessionKey(sid), id, expires); err != nil {
		log.Printf("写入会话失败：%v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "服务暂时不可用"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sid,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // 本机 HTTP 环境不加 Secure（课件 s3）
	})
	// 登录成功即返回授权上下文（用户、角色、班级），与 GET /api/me 同构。
	var className string
	if err := s.DB.QueryRow(`SELECT name FROM classes WHERE id = ?`, classID).Scan(&className); err != nil {
		className = ""
	}
	writeJSON(w, http.StatusOK, User{ID: id, Username: username, Role: role, ClassID: classID, ClassName: className})
}

// Logout 处理 POST /api/logout：删除服务端会话行，旧 Cookie 立即失效。
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		_, _ = s.DB.Exec(`DELETE FROM sessions WHERE id = ?`, s.sessionKey(c.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Me 处理 GET /api/me：返回用户、角色、班级三项，供前端决定上传入口。
func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	u, _ := FromContext(r.Context())
	writeJSON(w, http.StatusOK, u)
}

// Middleware 校验会话 Cookie 并把 User 注入上下文。
// 未登录返回 401，响应体不含任何业务数据（spec R1 未登录 Scenario）。
func (s *Service) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
			return
		}
		var u User
		row := s.DB.QueryRow(`
			SELECT u.id, u.username, u.role, u.class_id, c.name
			FROM sessions s
			JOIN users u ON u.id = s.user_id
			JOIN classes c ON c.id = u.class_id
			WHERE s.id = ? AND s.expires_at > NOW()`, s.sessionKey(c.Value))
		if err := row.Scan(&u.ID, &u.Username, &u.Role, &u.ClassID, &u.ClassName); err != nil {
			if err != sql.ErrNoRows {
				log.Printf("会话查询失败：%v", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "服务暂时不可用"})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

// RequireTeacher 在会话之上再加角色判断；学生写操作一律 403（spec R2）。
func RequireTeacher(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
			return
		}
		if u.Role != "teacher" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "仅教师可执行此操作"})
			return
		}
		next(w, r)
	}
}

// --- 登录失败限流（进程内存，故本迭代为单实例；design.md D8） ---

type failRecord struct {
	count   int
	resetAt time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	records map[string]*failRecord
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, records: map[string]*failRecord{}}
}

func (rl *rateLimiter) limited(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rec, ok := rl.records[key]
	if !ok || time.Now().After(rec.resetAt) {
		return false
	}
	return rec.count >= rl.limit
}

func (rl *rateLimiter) fail(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rec, ok := rl.records[key]
	now := time.Now()
	if !ok || now.After(rec.resetAt) {
		rl.records[key] = &failRecord{count: 1, resetAt: now.Add(rl.window)}
		return
	}
	rec.count++
}

func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.records, key)
}
