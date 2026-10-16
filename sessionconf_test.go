package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPEM = "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEexample\n-----END PUBLIC KEY-----\n"

func writeTestSession(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "session.conf")
	raw := map[string]any{
		"private_key": "c2VjcmV0", "endpoint_v4": "162.159.198.2", "endpoint_v6": "2606:4700:103::",
		"endpoint_pub_key": testPEM, "id": "dev-1", "access_token": "tok-secret",
		"ipv4": "172.16.0.2", "ipv6": "2606:4700:110::1", "extra_unknown": "keep-me",
	}
	b, _ := json.Marshal(raw)
	_ = os.WriteFile(p, b, 0o600)
	return p
}

func TestSessionViewMasksSecrets(t *testing.T) {
	p := writeTestSession(t)
	v, err := readSessionView(p)
	if err != nil || !v.Exists {
		t.Fatal(err, v)
	}
	if v.Config["private_key"] != maskValue || v.Config["access_token"] != maskValue {
		t.Errorf("секреты не замаскированы: %v", v.Config)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "c2VjcmV0") || strings.Contains(string(b), "tok-secret") {
		t.Errorf("секрет просочился в ответ: %s", b)
	}
	if v.Config["endpoint_v4"] != "162.159.198.2" {
		t.Errorf("%v", v.Config)
	}
	if v, _ := readSessionView(filepath.Join(t.TempDir(), "нет")); v.Exists {
		t.Error("exists должен быть false")
	}
}

func TestSessionWriteKeepsSecretsAndUnknown(t *testing.T) {
	p := writeTestSession(t)
	err := writeSessionConf(p, map[string]string{
		"endpoint_v4": "162.159.198.9", "private_key": maskValue, "access_token": "", "ipv4": "172.16.0.9",
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := loadSessionRaw(p)
	if m["endpoint_v4"] != "162.159.198.9" || m["ipv4"] != "172.16.0.9" {
		t.Errorf("%v", m)
	}
	if m["private_key"] != "c2VjcmV0" || m["access_token"] != "tok-secret" {
		t.Errorf("секреты изменились: %v", m)
	}
	if m["extra_unknown"] != "keep-me" || m["endpoint_pub_key"] != testPEM {
		t.Errorf("потеряны прочие поля: %v", m)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("права: %v", st.Mode())
	}
	if _, err := os.Stat(p + ".gui-bak"); err != nil {
		t.Errorf("нет резервной копии: %v", err)
	}
}

func TestSessionWriteReplacesSecretAndClearsOptional(t *testing.T) {
	p := writeTestSession(t)
	if err := writeSessionConf(p, map[string]string{"private_key": "bmV3a2V5", "endpoint_h2_v4": "", "endpoint_v6": ""}); err != nil {
		t.Fatal(err)
	}
	m, _ := loadSessionRaw(p)
	if m["private_key"] != "bmV3a2V5" || m["endpoint_v6"] != "" {
		t.Errorf("%v", m)
	}
}

func TestSessionValidation(t *testing.T) {
	bad := []map[string]string{
		{"endpoint_v4": "not-an-ip"},
		{"endpoint_v4": "2606:4700::1"},
		{"endpoint_v6": "1.2.3.4"},
		{"ipv4": "1.2.3.4:443"},
		{"endpoint_pub_key": "просто текст"},
		{"private_key": "!!!не base64!!!"},
	}
	for _, in := range bad {
		p := writeTestSession(t)
		before, _ := os.ReadFile(p)
		if err := writeSessionConf(p, in); err == nil {
			t.Errorf("должно отклоняться: %v", in)
		}
		after, _ := os.ReadFile(p)
		if string(before) != string(after) {
			t.Errorf("файл изменился при ошибке: %v", in)
		}
	}
}

func TestSessionAPI(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/api/setup", `{"password":"secret1"}`)
	if _, m := e.do("GET", "/api/session", ""); m["exists"] == true {
		t.Fatalf("сессии быть не должно: %v", m)
	}
	if code, _ := e.do("PUT", "/api/session", `{"endpoint_v4":"1.1.1.1"}`); code != 400 {
		t.Fatalf("PUT без сессии: %d", code)
	}
	b, _ := json.Marshal(map[string]string{"private_key": "c2VjcmV0", "endpoint_v4": "162.159.198.2"})
	_ = os.WriteFile(filepath.Join(e.dir, "session.conf"), b, 0o600)
	code, m := e.do("PUT", "/api/session", `{"endpoint_v4":"162.159.198.5","private_key":"********"}`)
	if code != 200 {
		t.Fatalf("PUT: %d %v", code, m)
	}
	_, m = e.do("GET", "/api/session", "")
	cfg := m["config"].(map[string]any)
	if cfg["endpoint_v4"] != "162.159.198.5" || cfg["private_key"] != maskValue {
		t.Fatalf("%v", m)
	}
}
