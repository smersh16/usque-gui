package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
)

// UsqueConf — управляемые GUI параметры /opt/etc/usque/usque.conf.
// Файл source'ится init-скриптом как shell, поэтому любые значения
// проходят строгую валидацию, а запись идёт только в виде KEY="value".
type UsqueConf struct {
	Iface     string `json:"iface"`
	IfaceIP   string `json:"ifaceIp"`
	IfaceMask string `json:"ifaceMask"`
	SNI       string `json:"sni"`
	HTTP2     bool   `json:"http2"`
}

var (
	reIface = regexp.MustCompile(`^opkgtun[0-9]{1,2}$`)
	reHost  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	reKV    = regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)=(.*)$`)
)

// Validate проверяет значения; пустые IfaceIP/IfaceMask/SNI допустимы.
func (c *UsqueConf) Validate() error {
	if !reIface.MatchString(c.Iface) {
		return errors.New("интерфейс должен иметь вид opkgtunN (например, opkgtun0)")
	}
	if c.IfaceIP != "" {
		ip := net.ParseIP(c.IfaceIP)
		if ip == nil || ip.To4() == nil || strings.Contains(c.IfaceIP, ":") {
			return errors.New("IP-адрес интерфейса: ожидается IPv4, например 172.16.1.100")
		}
	}
	if c.IfaceMask != "" {
		if !validMask(c.IfaceMask) {
			return errors.New("маска: ожидается IPv4-маска, например 255.255.255.255")
		}
	}
	if c.SNI != "" && !reHost.MatchString(c.SNI) {
		return errors.New("SNI: допустимы только латиница, цифры, точка и дефис")
	}
	return nil
}

func validMask(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil || strings.Contains(s, ":") {
		return false
	}
	m := ip.To4()
	if m == nil {
		return false
	}
	ones, bits := net.IPMask(m).Size()
	return bits == 32 && ones >= 0
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	// значение без кавычек: отрезаем хвостовой комментарий
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

// parseConf читает активные (не закомментированные) ключи.
func parseConf(data string) *UsqueConf {
	c := &UsqueConf{Iface: "opkgtun0"}
	for _, line := range strings.Split(data, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") {
			continue
		}
		m := reKV.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		val := unquote(m[2])
		switch m[1] {
		case "IFACE":
			c.Iface = strings.ToLower(val)
		case "IFACE_IP":
			c.IfaceIP = val
		case "IFACE_MASK":
			c.IfaceMask = val
		case "SNI":
			c.SNI = val
		case "HTTP2_ENABLE":
			c.HTTP2 = val == "1"
		}
	}
	return c
}

func readConf(path string) (*UsqueConf, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseConf(string(b)), nil
}

// renderConf возвращает новое содержимое файла: комментарии и прочие
// ключи (CONFIG_VERSION и т.д.) сохраняются.
func renderConf(old string, c *UsqueConf) string {
	lines := strings.Split(old, "\n")
	// убираем хвостовую пустую строку, вернём в конце
	trailingNL := len(lines) > 0 && lines[len(lines)-1] == ""
	if trailingNL {
		lines = lines[:len(lines)-1]
	}

	http2 := "0"
	if c.HTTP2 {
		http2 = "1"
	}
	type kv struct {
		key, val string
		// commentIfEmpty: пустое значение — закомментировать строку,
		// иначе писать KEY="".
		commentIfEmpty bool
		// bare: без кавычек
		bare bool
	}
	set := []kv{
		{"IFACE", c.Iface, false, false},
		{"IFACE_IP", c.IfaceIP, true, false},
		{"IFACE_MASK", c.IfaceMask, true, false},
		{"SNI", c.SNI, false, false},
		{"HTTP2_ENABLE", http2, false, true},
	}

	for _, s := range set {
		activeRe := regexp.MustCompile(`^\s*` + s.key + `=`)
		commentRe := regexp.MustCompile(`^\s*#\s*` + s.key + `=`)
		active, commented := -1, -1
		for i, l := range lines {
			if active < 0 && activeRe.MatchString(l) {
				active = i
			}
			if commented < 0 && commentRe.MatchString(l) {
				commented = i
			}
		}
		render := fmt.Sprintf(`%s="%s"`, s.key, s.val)
		if s.bare {
			render = fmt.Sprintf(`%s=%s`, s.key, s.val)
		}
		if s.val == "" && s.commentIfEmpty {
			if active >= 0 {
				lines[active] = "# " + strings.TrimSpace(lines[active])
			}
			continue
		}
		switch {
		case active >= 0:
			lines[active] = render
		case commented >= 0:
			lines[commented] = render
		default:
			lines = insertBeforeVersion(lines, render)
		}
	}
	out := strings.Join(lines, "\n")
	if trailingNL || out != "" {
		out += "\n"
	}
	return out
}

func insertBeforeVersion(lines []string, line string) []string {
	idx := len(lines)
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "# Configuration version") {
			idx = i
			break
		}
	}
	res := make([]string, 0, len(lines)+2)
	res = append(res, lines[:idx]...)
	res = append(res, line, "")
	res = append(res, lines[idx:]...)
	return res
}

// writeConf валидирует и атомарно записывает конфиг (с резервной копией).
func writeConf(path string, c *UsqueConf) error {
	c.Iface = strings.ToLower(strings.TrimSpace(c.Iface))
	c.IfaceIP = strings.TrimSpace(c.IfaceIP)
	c.IfaceMask = strings.TrimSpace(c.IfaceMask)
	c.SNI = strings.TrimSpace(c.SNI)
	if err := c.Validate(); err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	_ = os.WriteFile(path+".gui-bak", old, st.Mode().Perm())

	tmp := path + ".gui-tmp"
	if err := os.WriteFile(tmp, []byte(renderConf(string(old), c)), st.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
