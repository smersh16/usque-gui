package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Поля session.conf (JSON, формат usque config.Config).
// Секретные поля в браузер не отдаются: вместо значения приходит маска,
// а при сохранении пустое значение или маска означают «не менять».
const maskValue = "********"

var sessionSecretKeys = []string{"private_key", "access_token"}

type sessionField struct {
	key  string
	kind string // ip4 | ip6 | text | pem | b64
}

var sessionFields = []sessionField{
	{"endpoint_v4", "ip4"},
	{"endpoint_v6", "ip6"},
	{"endpoint_h2_v4", "ip4"},
	{"endpoint_h2_v6", "ip6"},
	{"endpoint_pub_key", "pem"},
	{"id", "text"},
	{"ipv4", "ip4"},
	{"ipv6", "ip6"},
	{"private_key", "b64"},
	{"access_token", "text"},
}

func isSecret(k string) bool {
	for _, s := range sessionSecretKeys {
		if s == k {
			return true
		}
	}
	return false
}

func loadSessionRaw(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("session.conf не является JSON-объектом: %w", err)
	}
	return m, nil
}

// SessionView — то, что видит браузер.
type SessionView struct {
	Exists bool              `json:"exists"`
	Config map[string]string `json:"config,omitempty"`
	// Secrets — ключи, значения которых скрыты (задано ли значение — в Config как маска).
	Secrets []string `json:"secrets,omitempty"`
}

func readSessionView(path string) (SessionView, error) {
	if !fileExists(path) {
		return SessionView{}, nil
	}
	raw, err := loadSessionRaw(path)
	if err != nil {
		return SessionView{Exists: true}, err
	}
	v := SessionView{Exists: true, Config: map[string]string{}, Secrets: sessionSecretKeys}
	for _, f := range sessionFields {
		s, _ := raw[f.key].(string)
		if isSecret(f.key) && s != "" {
			s = maskValue
		}
		v.Config[f.key] = s
	}
	return v, nil
}

func validateSessionValue(f sessionField, v string) error {
	if v == "" {
		return nil
	}
	switch f.kind {
	case "ip4":
		ip := net.ParseIP(v)
		if ip == nil || ip.To4() == nil || strings.Contains(v, ":") {
			return fmt.Errorf("%s: ожидается IPv4-адрес", f.key)
		}
	case "ip6":
		ip := net.ParseIP(v)
		if ip == nil || !strings.Contains(v, ":") {
			return fmt.Errorf("%s: ожидается IPv6-адрес", f.key)
		}
	case "pem":
		if blk, _ := pem.Decode([]byte(v)); blk == nil {
			return fmt.Errorf("%s: ожидается PEM-блок (-----BEGIN PUBLIC KEY----- …)", f.key)
		}
	case "b64":
		if _, err := base64.StdEncoding.DecodeString(v); err != nil {
			return fmt.Errorf("%s: ожидается base64", f.key)
		}
	case "text":
		if len(v) > 4096 || strings.ContainsAny(v, "\x00") {
			return fmt.Errorf("%s: недопустимое значение", f.key)
		}
	}
	return nil
}

// writeSessionConf применяет изменения к session.conf: только известные поля,
// секреты при пустом значении/маске остаются прежними, прочие ключи файла
// сохраняются. Запись атомарная, права 0600, перед записью — копия .gui-bak.
func writeSessionConf(path string, in map[string]string) error {
	return writeSessionConfOpt(path, in, false)
}

// writeSessionConfOpt: create=true допускает отсутствие файла (восстановление
// на чистой установке); тогда нужны все обязательные поля из in.
func writeSessionConfOpt(path string, in map[string]string, create bool) error {
	old, err := loadSessionRaw(path)
	if err != nil {
		if !create || !os.IsNotExist(err) {
			return err
		}
		old = map[string]any{}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	}
	next := map[string]any{}
	for k, v := range old {
		next[k] = v
	}
	for _, f := range sessionFields {
		v, ok := in[f.key]
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if isSecret(f.key) && (v == "" || v == maskValue) {
			continue // не менять
		}
		if err := validateSessionValue(f, v); err != nil {
			return err
		}
		next[f.key] = v
	}
	if _, ok := next["private_key"].(string); !ok || next["private_key"] == "" {
		return errors.New("private_key не может быть пустым")
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(next); err != nil {
		return err
	}

	if b, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".gui-bak", b, 0o600)
	}
	tmp := path + ".gui-tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// EditSession — запись под замком операций, чтобы не пересекаться с
// регистрацией/сбросом.
func (s *Service) EditSession(_ context.Context, in map[string]string) error {
	if !s.busy.TryLock() {
		return errBusy
	}
	defer s.busy.Unlock()

	if !fileExists(s.paths.Session) {
		return errors.New("сессии нет: сначала зарегистрируйте аккаунт")
	}
	if err := writeSessionConf(s.paths.Session, in); err != nil {
		s.addLog("session-edit", false, err.Error())
		return err
	}
	if _, alive := processAlive(s.paths.PidFile, s.paths.UsqueBin); alive {
		s.setRestartPending(true)
	}
	s.addLog("session-edit", true, "session.conf обновлён (копия: session.conf.gui-bak)")
	return nil
}
