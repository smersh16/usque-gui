package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GUIConfig — собственные настройки GUI (/opt/etc/usque-gui/gui.json).
type GUIConfig struct {
	Listen       string `json:"listen"`
	AllowWAN     bool   `json:"allowWan"`
	PasswordHash string `json:"passwordHash,omitempty"`
	Salt         string `json:"salt,omitempty"`
	Iterations   int    `json:"iterations,omitempty"`
}

const (
	defaultListen = "0.0.0.0:2223"
	pbkdfIter     = 120000
	minPassword   = 6
	sessionTTL    = 24 * time.Hour
	cookieName    = "usque_gui_session"
)

func loadGUIConfig(path string) (*GUIConfig, error) {
	c := &GUIConfig{Listen: defaultListen}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if c.Listen == "" {
		c.Listen = defaultListen
	}
	return c, nil
}

func saveGUIConfig(path string, c *GUIConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func hashPassword(pw string, salt []byte, iter int) (string, error) {
	k, err := pbkdf2.Key(sha256.New, pw, salt, iter, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(k), nil
}

// Auth — пароль и in-memory сессии.
type Auth struct {
	path string

	mu       sync.Mutex
	cfg      *GUIConfig
	sessions map[string]time.Time
	fails    map[string]*failState
}

type failState struct {
	count int
	until time.Time
}

func NewAuth(path string, cfg *GUIConfig) *Auth {
	return &Auth{path: path, cfg: cfg, sessions: map[string]time.Time{}, fails: map[string]*failState{}}
}

func (a *Auth) NeedSetup() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.PasswordHash == ""
}

func (a *Auth) SetPassword(pw string) error {
	if len(pw) < minPassword {
		return errors.New("пароль слишком короткий (минимум 6 символов)")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	h, err := hashPassword(pw, salt, pbkdfIter)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.PasswordHash = h
	a.cfg.Salt = hex.EncodeToString(salt)
	a.cfg.Iterations = pbkdfIter
	a.sessions = map[string]time.Time{} // смена пароля выкидывает всех
	return saveGUIConfig(a.path, a.cfg)
}

func (a *Auth) check(pw string) bool {
	a.mu.Lock()
	hash, saltHex, iter := a.cfg.PasswordHash, a.cfg.Salt, a.cfg.Iterations
	a.mu.Unlock()
	salt, err := hex.DecodeString(saltHex)
	if err != nil || hash == "" || iter <= 0 {
		return false
	}
	h, err := hashPassword(pw, salt, iter)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(h), []byte(hash)) == 1
}

// Login возвращает токен сессии. После 5 неудач с одного IP — пауза 60 с.
func (a *Auth) Login(ip, pw string) (string, error) {
	a.mu.Lock()
	f := a.fails[ip]
	if f != nil && time.Now().Before(f.until) {
		a.mu.Unlock()
		return "", errors.New("слишком много попыток, подождите минуту")
	}
	a.mu.Unlock()

	if !a.check(pw) {
		a.mu.Lock()
		if f == nil {
			f = &failState{}
			a.fails[ip] = f
		}
		f.count++
		if f.count >= 5 {
			f.count = 0
			f.until = time.Now().Add(60 * time.Second)
		}
		a.mu.Unlock()
		return "", errors.New("неверный пароль")
	}

	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return "", err
	}
	t := hex.EncodeToString(tok)
	a.mu.Lock()
	delete(a.fails, ip)
	a.sessions[t] = time.Now().Add(sessionTTL)
	a.mu.Unlock()
	return t, nil
}

func (a *Auth) Valid(tok string) bool {
	if tok == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(a.sessions, tok)
		return false
	}
	return true
}

func (a *Auth) Logout(tok string) {
	a.mu.Lock()
	delete(a.sessions, tok)
	a.mu.Unlock()
}

func (a *Auth) AllowWAN() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.AllowWAN
}

func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// isLocalClient: loopback, частные сети и link-local (в т.ч. IPv6 ULA).
func isLocalClient(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
