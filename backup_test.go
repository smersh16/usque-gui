package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateListen(t *testing.T) {
	for in, want := range map[string]string{
		"0.0.0.0:2223": "0.0.0.0:2223", " 192.168.1.1:8080 ": "192.168.1.1:8080", ":2223": ":2223", "[::]:2223": "[::]:2223",
	} {
		got, err := validateListen(in)
		if err != nil || got != want {
			t.Errorf("%q -> %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"2223", "host:2223", "1.2.3.4:0", "1.2.3.4:70000", "1.2.3.4:abc", ""} {
		if _, err := validateListen(bad); err == nil {
			t.Errorf("должно отклоняться: %q", bad)
		}
	}
}

func TestBackupRoundTrip(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/api/setup", `{"password":"secret1"}`)
	sess, _ := json.Marshal(map[string]string{"private_key": "c2VjcmV0", "endpoint_v4": "162.159.198.2", "access_token": "tok"})
	sp := filepath.Join(e.dir, "session.conf")
	_ = os.WriteFile(sp, sess, 0o600)
	e.do("PUT", "/api/config", `{"iface":"opkgtun5","sni":"a.com","http2":true}`)

	// копия с сессией требует пароль
	if code, _ := e.do("POST", "/api/backup", `{"includeSession":true,"password":"wrong"}`); code != 403 {
		t.Fatalf("без пароля ожидался 403, %d", code)
	}
	// копия без сессии — без пароля, секретов в ней нет
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/backup", strings.NewReader(`{"includeSession":false}`))
	req.Header.Set("X-Requested-With", "usque-gui")
	resp, _ := e.client.Do(req)
	var b0 Backup
	_ = json.NewDecoder(resp.Body).Decode(&b0)
	resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") || b0.Session != nil || b0.Config.Iface != "opkgtun5" {
		t.Fatalf("копия без сессии: %+v %v", b0, resp.Header)
	}

	// копия с сессией
	req, _ = http.NewRequest("POST", e.srv.URL+"/api/backup", strings.NewReader(`{"includeSession":true,"password":"secret1"}`))
	req.Header.Set("X-Requested-With", "usque-gui")
	resp, _ = e.client.Do(req)
	var b Backup
	_ = json.NewDecoder(resp.Body).Decode(&b)
	resp.Body.Close()
	if b.Session["private_key"] != "c2VjcmV0" || b.Session["access_token"] != "tok" {
		t.Fatalf("сессия в копии: %+v", b.Session)
	}

	// меняем всё и восстанавливаем
	e.do("PUT", "/api/config", `{"iface":"opkgtun9"}`)
	_ = os.Remove(sp)
	body, _ := json.Marshal(map[string]any{"password": "secret1", "backup": b, "applyConfig": true, "applySession": true})
	if code, m := e.do("POST", "/api/restore", string(body)); code != 200 {
		t.Fatalf("restore: %d %v", code, m)
	}
	c, _ := readConf(filepath.Join(e.dir, "usque.conf"))
	if c.Iface != "opkgtun5" || !c.HTTP2 {
		t.Errorf("конфиг не восстановлен: %+v", c)
	}
	raw, _ := loadSessionRaw(sp)
	if raw["private_key"] != "c2VjcmV0" {
		t.Errorf("сессия не восстановлена: %v", raw)
	}
	if st, _ := os.Stat(sp); st.Mode().Perm() != 0o600 {
		t.Errorf("права сессии: %v", st.Mode())
	}

	// восстановление сессии без пароля запрещено
	body, _ = json.Marshal(map[string]any{"backup": b, "applySession": true})
	if code, _ := e.do("POST", "/api/restore", string(body)); code != 403 {
		t.Errorf("ожидался 403, %d", code)
	}
}

func TestRestoreRejectsMalicious(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/api/setup", `{"password":"secret1"}`)
	before, _ := os.ReadFile(filepath.Join(e.dir, "usque.conf"))
	evil := Backup{Format: backupFormat, Version: 1, Config: &UsqueConf{Iface: "opkgtun0", SNI: `a"; reboot; "`}}
	body, _ := json.Marshal(map[string]any{"backup": evil, "applyConfig": true})
	if code, _ := e.do("POST", "/api/restore", string(body)); code != 400 {
		t.Fatalf("инъекция должна отклоняться: %d", code)
	}
	after, _ := os.ReadFile(filepath.Join(e.dir, "usque.conf"))
	if string(before) != string(after) {
		t.Error("usque.conf изменился")
	}
	notBackup, _ := json.Marshal(map[string]any{"backup": map[string]any{"format": "other"}, "applyConfig": true})
	if code, _ := e.do("POST", "/api/restore", string(notBackup)); code != 400 {
		t.Errorf("чужой файл: %d", code)
	}
}

func TestGUISettingsAPI(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/api/setup", `{"password":"secret1"}`)
	if code, _ := e.do("PUT", "/api/gui", `{"listen":"bad"}`); code != 400 {
		t.Fatalf("%d", code)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	addr := ln.Addr().String()
	ln.Close() // свободный порт
	if code, m := e.do("PUT", "/api/gui", `{"listen":"`+addr+`"}`); code != 200 || m["restartRequired"] != true {
		t.Fatalf("%d %v", code, m)
	}
	if _, m := e.do("GET", "/api/gui", ""); m["listen"] != addr {
		t.Fatalf("%v", m)
	}
	// занятый порт отклоняется
	busy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer busy.Close()
	if code, _ := e.do("PUT", "/api/gui", `{"listen":"`+busy.Addr().String()+`"}`); code != 400 {
		t.Fatalf("занятый порт: %d", code)
	}
	if code, _ := e.do("POST", "/api/gui/restart", "{}"); code != 501 {
		t.Fatalf("в тестах перезапуск недоступен: %d", code)
	}
}
