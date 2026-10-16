package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDefault(t *testing.T) {
	b, _ := os.ReadFile("testdata/usque.conf")
	c := parseConf(string(b))
	if c.Iface != "opkgtun0" || c.SNI != "ozon.ru" || c.HTTP2 || c.IfaceIP != "" || c.IfaceMask != "" {
		t.Fatalf("неожиданный разбор: %+v", c)
	}
}

func TestRenderPreservesComments(t *testing.T) {
	b, _ := os.ReadFile("testdata/usque.conf")
	c := &UsqueConf{Iface: "opkgtun3", IfaceIP: "172.16.1.150", IfaceMask: "255.255.255.0", SNI: "example.com", HTTP2: true}
	out := renderConf(string(b), c)
	for _, want := range []string{
		`IFACE="opkgtun3"`, `IFACE_IP="172.16.1.150"`, `IFACE_MASK="255.255.255.0"`,
		`SNI="example.com"`, `HTTP2_ENABLE=1`, "CONFIG_VERSION=1", "# DO NOT CHANGE THIS VALUE", "# SNI for traffic masking",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("нет %q в:\n%s", want, out)
		}
	}
	if strings.Contains(out, `# IFACE_IP=`) {
		t.Errorf("закомментированный IFACE_IP должен быть раскомментирован:\n%s", out)
	}
	// повторный разбор даёт то же самое
	if got := parseConf(out); *got != *c {
		t.Errorf("round-trip: %+v != %+v", got, c)
	}
}

func TestRenderEmptyOptionalCommentsOut(t *testing.T) {
	in := "IFACE=\"opkgtun0\"\nIFACE_IP=\"1.2.3.4\"\nSNI=\"a.b\"\nHTTP2_ENABLE=1\nCONFIG_VERSION=1\n"
	out := renderConf(in, &UsqueConf{Iface: "opkgtun0", SNI: "", HTTP2: false})
	if strings.Contains(out, "\nIFACE_IP=") {
		t.Errorf("IFACE_IP должен быть закомментирован:\n%s", out)
	}
	c := parseConf(out)
	if c.IfaceIP != "" || c.SNI != "" || c.HTTP2 {
		t.Errorf("после очистки: %+v", c)
	}
}

func TestInsertMissingKeysBeforeVersion(t *testing.T) {
	in := "IFACE=\"opkgtun0\"\n\n# Configuration version\nCONFIG_VERSION=1\n"
	out := renderConf(in, &UsqueConf{Iface: "opkgtun0", SNI: "x.y", HTTP2: true})
	i, j := strings.Index(out, `SNI="x.y"`), strings.Index(out, "CONFIG_VERSION")
	if i < 0 || j < 0 || i > j {
		t.Errorf("SNI должен идти до CONFIG_VERSION:\n%s", out)
	}
}

func TestValidateRejectsInjection(t *testing.T) {
	bad := []UsqueConf{
		{Iface: `opkgtun0"; rm -rf /; "`},
		{Iface: "eth0"},
		{Iface: "opkgtun0", SNI: `a.com"; reboot; "`},
		{Iface: "opkgtun0", SNI: "a b"},
		{Iface: "opkgtun0", SNI: "$(id)"},
		{Iface: "opkgtun0", IfaceIP: "1.2.3.4; id"},
		{Iface: "opkgtun0", IfaceIP: "::1"},
		{Iface: "opkgtun0", IfaceMask: "255.0.255.0"},
		{Iface: "opkgtun0", IfaceMask: "abc"},
		{Iface: "opkgtun100"},
	}
	for _, c := range bad {
		c := c
		if err := c.Validate(); err == nil {
			t.Errorf("должно отклоняться: %+v", c)
		}
	}
	good := []UsqueConf{
		{Iface: "opkgtun0"},
		{Iface: "opkgtun12", IfaceIP: "172.16.1.100", IfaceMask: "255.255.255.255", SNI: "ozon.ru", HTTP2: true},
		{Iface: "opkgtun1", IfaceMask: "255.255.255.0"},
	}
	for _, c := range good {
		c := c
		if err := c.Validate(); err != nil {
			t.Errorf("должно приниматься %+v: %v", c, err)
		}
	}
}

func TestWriteConfAtomicAndBackup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "usque.conf")
	b, _ := os.ReadFile("testdata/usque.conf")
	_ = os.WriteFile(p, b, 0o644)
	if err := writeConf(p, &UsqueConf{Iface: "OpkgTun2", SNI: "a.com"}); err != nil {
		t.Fatal(err)
	}
	got, _ := readConf(p)
	if got.Iface != "opkgtun2" || got.SNI != "a.com" {
		t.Errorf("%+v", got)
	}
	if bak, err := os.ReadFile(p + ".gui-bak"); err != nil || string(bak) != string(b) {
		t.Errorf("резервная копия не совпадает: %v", err)
	}
	// невалидное значение не трогает файл
	before, _ := os.ReadFile(p)
	if err := writeConf(p, &UsqueConf{Iface: "bad"}); err == nil {
		t.Error("ожидалась ошибка")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Error("файл изменился при ошибке валидации")
	}
}
