package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type env struct {
	t      *testing.T
	srv    *httptest.Server
	dir    string
	client *http.Client
}

func newEnv(t *testing.T) *env {
	dir := t.TempDir()
	conf := filepath.Join(dir, "usque.conf")
	b, _ := os.ReadFile("testdata/usque.conf")
	_ = os.WriteFile(conf, b, 0o644)
	init := filepath.Join(dir, "S51usque")
	// Подставной init-скрипт: start/stop пишут pidfile процесса "usque".
	_ = os.WriteFile(init, []byte("#!/bin/sh\necho \"$1 ok\"\n"), 0o755)
	p := Paths{InitScript: init, UsqueBin: filepath.Join(dir, "usque"), Conf: conf,
		Session: filepath.Join(dir, "session.conf"), PidFile: filepath.Join(dir, "usque.pid")}
	cfg := &GUIConfig{Listen: defaultListen}
	auth := NewAuth(filepath.Join(dir, "gui.json"), cfg)
	srv := httptest.NewServer(NewApp(p, auth).Handler())
	t.Cleanup(srv.Close)
	jar, _ := newJar()
	return &env{t: t, srv: srv, dir: dir, client: &http.Client{Jar: jar}}
}

func (e *env) do(method, path, body string) (int, map[string]any) {
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	req.Header.Set("X-Requested-With", "usque-gui")
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func TestFlow(t *testing.T) {
	e := newEnv(t)

	if code, _ := e.do("GET", "/api/status", ""); code != 401 {
		t.Fatalf("без входа ожидался 401, получено %d", code)
	}
	if _, m := e.do("GET", "/api/auth", ""); m["needSetup"] != true {
		t.Fatalf("ожидался needSetup: %v", m)
	}
	if code, _ := e.do("POST", "/api/setup", `{"password":"123"}`); code != 400 {
		t.Fatalf("короткий пароль должен отклоняться, %d", code)
	}
	if code, _ := e.do("POST", "/api/setup", `{"password":"secret1"}`); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	if code, _ := e.do("POST", "/api/setup", `{"password":"other11"}`); code != 409 {
		t.Fatalf("повторный setup должен давать 409, %d", code)
	}

	code, m := e.do("GET", "/api/status", "")
	if code != 200 {
		t.Fatalf("status: %d %v", code, m)
	}
	st := m["status"].(map[string]any)
	if st["state"] != "stopped" || st["hasSession"] != false {
		t.Fatalf("статус: %v", st)
	}

	// конфиг: чтение, запись, отклонение инъекции
	if code, m := e.do("PUT", "/api/config", `{"iface":"opkgtun4","sni":"a.com","http2":true}`); code != 200 {
		t.Fatalf("put config: %d %v", code, m)
	}
	if code, _ := e.do("PUT", "/api/config", `{"iface":"opkgtun4","sni":"a\"; id; \""}`); code != 400 {
		t.Fatalf("инъекция должна отклоняться: %d", code)
	}
	if _, m := e.do("GET", "/api/config", ""); m["iface"] != "opkgtun4" || m["http2"] != true {
		t.Fatalf("config: %v", m)
	}

	// управление сервисом
	if code, m := e.do("POST", "/api/service", `{"action":"start"}`); code != 200 || !strings.Contains(m["output"].(string), "start ok") {
		t.Fatalf("service: %d %v", code, m)
	}
	if code, _ := e.do("POST", "/api/service", `{"action":"format"}`); code == 200 {
		t.Fatal("неизвестное действие должно отклоняться")
	}

	// сессия: сброс при отсутствии файла — не ошибка; регистрация без бинарника — ошибка
	if code, _ := e.do("POST", "/api/session/reset", ""); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	if code, _ := e.do("POST", "/api/session/register", ""); code != 500 {
		t.Fatalf("register без usque должен падать: %d", code)
	}

	// журнал
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/log", nil)
	resp, _ := e.client.Do(req)
	var log []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&log)
	resp.Body.Close()
	if len(log) < 3 {
		t.Fatalf("журнал: %v", log)
	}

	// выход и повторный вход
	e.do("POST", "/api/logout", "{}")
	if code, _ := e.do("GET", "/api/status", ""); code != 401 {
		t.Fatalf("после выхода: %d", code)
	}
	if code, _ := e.do("POST", "/api/login", `{"password":"nope"}`); code != 401 {
		t.Fatalf("неверный пароль: %d", code)
	}
	if code, _ := e.do("POST", "/api/login", `{"password":"secret1"}`); code != 200 {
		t.Fatalf("login: %d", code)
	}
}

func TestCSRFHeaderRequired(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/setup", strings.NewReader(`{"password":"secret1"}`))
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("без X-Requested-With ожидался 403, %d", resp.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/api/setup", `{"password":"secret1"}`)
	e.do("POST", "/api/logout", "{}")
	for i := 0; i < 5; i++ {
		e.do("POST", "/api/login", `{"password":"bad"}`)
	}
	// после 5 неудач даже верный пароль блокируется
	if code, _ := e.do("POST", "/api/login", `{"password":"secret1"}`); code != 401 {
		t.Fatalf("ожидалась блокировка, %d", code)
	}
}

func TestStaticServed(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.srv.URL + "/")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("index: %v %v", err, resp)
	}
	resp.Body.Close()
	if h := resp.Header.Get("Content-Security-Policy"); !strings.Contains(h, "script-src 'self'") {
		t.Errorf("CSP: %q", h)
	}
}

func TestControlSurvivesDaemonHoldingPipe(t *testing.T) {
	dir := t.TempDir()
	init := filepath.Join(dir, "S51usque")
	// фоновый процесс наследует stdout — как «грязный» демон
	_ = os.WriteFile(init, []byte("#!/bin/sh\nsleep 5 &\necho started\n"), 0o755)
	s := NewService(Paths{InitScript: init})
	out, err := s.Control(t.Context(), "start")
	if err != nil || !strings.Contains(out, "started") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
