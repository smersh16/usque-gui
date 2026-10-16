package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const backupFormat = "usque-gui-backup"

// Backup — переносимая копия настроек. usque.conf хранится структурой, а не
// текстом: при восстановлении значения проходят валидацию (файл исполняется
// как shell), произвольный текст в него не попадает.
type Backup struct {
	Format     string            `json:"format"`
	Version    int               `json:"version"`
	CreatedAt  time.Time         `json:"createdAt"`
	GUIVersion string            `json:"guiVersion"`
	Config     *UsqueConf        `json:"usqueConf,omitempty"`
	Session    map[string]string `json:"session,omitempty"` // содержит секреты!
}

func (a *App) makeBackup(includeSession bool) (Backup, error) {
	b := Backup{Format: backupFormat, Version: 1, CreatedAt: time.Now().UTC(), GUIVersion: version}
	c, err := readConf(a.paths.Conf)
	if err != nil {
		return b, fmt.Errorf("usque.conf: %w", err)
	}
	b.Config = c
	if includeSession && fileExists(a.paths.Session) {
		raw, err := loadSessionRaw(a.paths.Session)
		if err != nil {
			return b, err
		}
		b.Session = map[string]string{}
		for _, f := range sessionFields {
			if s, _ := raw[f.key].(string); s != "" {
				b.Session[f.key] = s
			}
		}
	}
	return b, nil
}

// validateBackup проверяет копию целиком до любых записей.
func validateBackup(b *Backup, wantConf, wantSession bool) error {
	if b.Format != backupFormat {
		return errors.New("это не резервная копия usque-gui")
	}
	if b.Version != 1 {
		return fmt.Errorf("неподдерживаемая версия копии: %d", b.Version)
	}
	if wantConf {
		if b.Config == nil {
			return errors.New("в копии нет настроек usque")
		}
		c := *b.Config
		if err := c.Validate(); err != nil {
			return err
		}
	}
	if wantSession {
		if len(b.Session) == 0 {
			return errors.New("в копии нет сессии WARP (копия сделана без неё)")
		}
		for _, f := range sessionFields {
			if err := validateSessionValue(f, b.Session[f.key]); err != nil {
				return err
			}
		}
		if b.Session["private_key"] == "" {
			return errors.New("в копии нет private_key")
		}
	}
	return nil
}

func (a *App) restore(b *Backup, wantConf, wantSession bool) error {
	if err := validateBackup(b, wantConf, wantSession); err != nil {
		return err
	}
	if !a.svc.busy.TryLock() {
		return errBusy
	}
	defer a.svc.busy.Unlock()

	if wantConf {
		if !fileExists(a.paths.Conf) {
			return errors.New("usque.conf не найден: установите usque-keenetic")
		}
		c := *b.Config
		if err := writeConf(a.paths.Conf, &c); err != nil {
			return err
		}
	}
	if wantSession {
		// Секреты в копии настоящие, маски быть не может — но если кто-то
		// подсунул маску, не затираем ею ключ.
		for _, k := range sessionSecretKeys {
			if b.Session[k] == maskValue {
				return errors.New("в копии маска вместо секрета")
			}
		}
		if err := writeSessionConfOpt(a.paths.Session, b.Session, true); err != nil {
			return err
		}
	}
	if _, alive := processAlive(a.paths.PidFile, a.paths.UsqueBin); alive {
		a.svc.setRestartPending(true)
	}
	a.svc.addLog("restore", true, fmt.Sprintf("восстановлено: настройки=%v, сессия=%v (копия от %s)",
		wantConf, wantSession, b.CreatedAt.Format("2006-01-02 15:04")))
	return nil
}

func (a *App) handleBackup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password       string
		IncludeSession bool
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	if in.IncludeSession && !a.auth.check(in.Password) {
		writeErr(w, http.StatusForbidden, "для копии с сессией нужен верный пароль")
		return
	}
	b, err := a.makeBackup(in.IncludeSession)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	name := "usque-gui-backup-" + b.CreatedAt.Local().Format("20060102-1504") + ".json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(b)
	a.svc.addLog("backup", true, "создана копия (с сессией: "+strconv.FormatBool(in.IncludeSession)+")")
}

func (a *App) handleRestore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	var in struct {
		Password     string
		Backup       Backup
		ApplyConfig  bool
		ApplySession bool
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	if !in.ApplyConfig && !in.ApplySession {
		writeErr(w, 400, "ничего не выбрано для восстановления")
		return
	}
	if in.ApplySession && !a.auth.check(in.Password) {
		writeErr(w, http.StatusForbidden, "для восстановления сессии нужен верный пароль")
		return
	}
	if err := a.restore(&in.Backup, in.ApplyConfig, in.ApplySession); err != nil {
		code := 400
		if errors.Is(err, errBusy) {
			code = 409
		}
		a.svc.addLog("restore", false, err.Error())
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleProbe(w http.ResponseWriter, r *http.Request) {
	var in struct{ IPs []string }
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	ips, err := normalizeProbeIPs(in.IPs)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sni := ""
	if c, err := readConf(a.paths.Conf); err == nil {
		sni = c.SNI
	}
	writeJSON(w, 200, ProbeEndpoints(r.Context(), ips, sni, 3*time.Second))
}

// validateListen: host:port, host — пусто или IP-литерал; порт 1–65535.
func validateListen(s string) (string, error) {
	s = strings.TrimSpace(s)
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return "", errors.New("адрес должен быть вида 0.0.0.0:2223 или 192.168.1.1:2223")
	}
	if host != "" && net.ParseIP(host) == nil {
		return "", errors.New("адрес должен быть IP-адресом (или пустым — все интерфейсы)")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", errors.New("порт должен быть от 1 до 65535")
	}
	return net.JoinHostPort(host, port), nil
}

func (a *App) handleGetGUI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"listen": a.auth.Listen(), "canRestart": a.restart != nil})
}

func (a *App) handlePutGUI(w http.ResponseWriter, r *http.Request) {
	var in struct{ Listen string }
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "некорректный запрос")
		return
	}
	l, err := validateListen(in.Listen)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if l != a.auth.Listen() {
		// Убедиться, что адрес можно занять, пока работает старый.
		ln, err := net.Listen("tcp", l)
		if err != nil {
			writeErr(w, 400, "не удаётся занять "+l+": "+err.Error())
			return
		}
		_ = ln.Close()
	}
	if err := a.auth.SetListen(l); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.svc.addLog("gui-listen", true, "адрес интерфейса: "+l+" (применится после перезапуска GUI)")
	writeJSON(w, 200, map[string]any{"ok": true, "listen": l, "restartRequired": true})
}

func (a *App) handleGUIRestart(w http.ResponseWriter, r *http.Request) {
	if a.restart == nil {
		writeErr(w, 501, "перезапуск недоступен")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "listen": a.auth.Listen()})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	a.svc.addLog("gui-restart", true, "перезапуск интерфейса")
	a.restart()
}
