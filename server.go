package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"time"
)

//go:embed web
var webFS embed.FS

type App struct {
	paths Paths
	auth  *Auth
	svc   *Service
}

func NewApp(p Paths, auth *Auth) *App {
	return &App{paths: p, auth: auth, svc: NewService(p)}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 16<<10)
	return json.NewDecoder(r.Body).Decode(v)
}

// guard: доступ только из локальных сетей (если не allowWan), для
// изменяющих запросов — защитный заголовок против CSRF, для API — сессия.
func (a *App) guard(needAuth bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.auth.AllowWAN() && !isLocalClient(remoteIP(r)) {
			writeErr(w, http.StatusForbidden, "доступ разрешён только из локальной сети")
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "usque-gui" {
			writeErr(w, http.StatusForbidden, "некорректный запрос")
			return
		}
		if needAuth {
			c, err := r.Cookie(cookieName)
			if err != nil || !a.auth.Valid(c.Value) {
				writeErr(w, http.StatusUnauthorized, "требуется вход")
				return
			}
		}
		h(w, r)
	}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.auth.AllowWAN() && !isLocalClient(remoteIP(r)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'none'")
		static.ServeHTTP(w, r)
	}))

	mux.HandleFunc("GET /api/auth", a.guard(false, a.handleAuthState))
	mux.HandleFunc("POST /api/setup", a.guard(false, a.handleSetup))
	mux.HandleFunc("POST /api/login", a.guard(false, a.handleLogin))
	mux.HandleFunc("POST /api/logout", a.guard(true, a.handleLogout))
	mux.HandleFunc("POST /api/password", a.guard(true, a.handlePassword))

	mux.HandleFunc("GET /api/status", a.guard(true, a.handleStatus))
	mux.HandleFunc("POST /api/service", a.guard(true, a.handleService))
	mux.HandleFunc("GET /api/config", a.guard(true, a.handleGetConfig))
	mux.HandleFunc("PUT /api/config", a.guard(true, a.handlePutConfig))
	mux.HandleFunc("POST /api/session/register", a.guard(true, a.handleRegister))
	mux.HandleFunc("POST /api/session/reset", a.guard(true, a.handleReset))
	mux.HandleFunc("POST /api/session/renew", a.guard(true, a.handleRenew))
	mux.HandleFunc("GET /api/session", a.guard(true, a.handleGetSession))
	mux.HandleFunc("PUT /api/session", a.guard(true, a.handlePutSession))
	mux.HandleFunc("GET /api/versions", a.guard(true, a.handleVersions))
	mux.HandleFunc("POST /api/updates/check", a.guard(true, a.handleUpdatesCheck))
	mux.HandleFunc("POST /api/updates/upgrade", a.guard(true, a.handleUpgrade))
	mux.HandleFunc("POST /api/check", a.guard(true, a.handleCheck))
	mux.HandleFunc("GET /api/log", a.guard(true, a.handleLog))
	return mux
}

func (a *App) handleAuthState(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieName)
	writeJSON(w, 200, map[string]any{
		"needSetup":     a.auth.NeedSetup(),
		"authenticated": err == nil && a.auth.Valid(c.Value),
		"version":       version,
	})
}

func (a *App) startSession(w http.ResponseWriter, r *http.Request, pw string) {
	tok, err := a.auth.Login(remoteIP(r).String(), pw)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds()),
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !a.auth.NeedSetup() {
		writeErr(w, http.StatusConflict, "пароль уже задан")
		return
	}
	var in struct{ Password string }
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	if err := a.auth.SetPassword(in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.startSession(w, r, in.Password)
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Password string }
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	a.startSession(w, r, in.Password)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		a.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handlePassword(w http.ResponseWriter, r *http.Request) {
	var in struct{ Current, New string }
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	if !a.auth.check(in.Current) {
		writeErr(w, http.StatusForbidden, "текущий пароль неверен")
		return
	}
	if err := a.auth.SetPassword(in.New); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.startSession(w, r, in.New)
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	st := collectStatus(ctx, a.paths)
	writeJSON(w, 200, map[string]any{
		"status":         st,
		"restartPending": st.Running && (a.svc.RestartPending() || st.ConfigDiffers),
		"version":        version,
	})
}

func (a *App) handleService(w http.ResponseWriter, r *http.Request) {
	var in struct{ Action string }
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	out, err := a.svc.Control(r.Context(), in.Action)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error(), "output": out})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "output": out})
}

func (a *App) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	c, err := readConf(a.paths.Conf)
	if err != nil {
		writeErr(w, 500, "не удалось прочитать "+a.paths.Conf+": "+err.Error())
		return
	}
	writeJSON(w, 200, c)
}

func (a *App) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var c UsqueConf
	if err := decode(r, &c); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	if err := writeConf(a.paths.Conf, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, alive := processAlive(a.paths.PidFile, a.paths.UsqueBin); alive {
		a.svc.setRestartPending(true)
	}
	a.svc.addLog("config", true, "usque.conf сохранён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	out, err := a.svc.Register(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error(), "output": out})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "output": out})
}

func (a *App) handleReset(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.ResetSession(r.Context()); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleRenew(w http.ResponseWriter, r *http.Request) {
	out, err := a.svc.Renew(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error(), "output": out})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "output": out})
}

func (a *App) handleCheck(w http.ResponseWriter, r *http.Request) {
	st := collectStatus(r.Context(), a.paths)
	if !st.Running || !st.Link.Exists {
		writeJSON(w, 200, CheckResult{Error: "сервис не запущен или интерфейс отсутствует"})
		return
	}
	writeJSON(w, 200, CheckExit(r.Context(), st.Iface))
}

func (a *App) handleLog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.svc.Log())
}

func (a *App) handleVersions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.versions(r.Context()))
}

func (a *App) handleUpdatesCheck(w http.ResponseWriter, r *http.Request) {
	info, err := a.checkUpdates(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, info)
}

func (a *App) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	var in struct{ Package string }
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	out, err := a.upgradePkg(r.Context(), in.Package)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error(), "output": out})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "output": out})
}

func (a *App) handleGetSession(w http.ResponseWriter, r *http.Request) {
	v, err := readSessionView(a.paths.Session)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, v)
}

func (a *App) handlePutSession(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	if err := a.svc.EditSession(r.Context(), in); err != nil {
		code := 400
		if errors.Is(err, errBusy) {
			code = 409
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
