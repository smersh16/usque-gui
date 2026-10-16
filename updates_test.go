package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOpkgStatus(t *testing.T) {
	data := "Package: busybox\nVersion: 1.36.1-1\nStatus: install user installed\n\n" +
		"Package: usque-keenetic\nVersion: 0.3.0\nDepends: x\nStatus: install user installed\n\n" +
		"Package: usque-gui\nVersion: 0.1.0\n"
	got := parseOpkgStatus(data, watchedPkgs)
	if got["usque-keenetic"] != "0.3.0" || got["usque-gui"] != "0.1.0" || len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestParseUpgradable(t *testing.T) {
	out := "usque-keenetic - 0.3.0 - 0.4.0\nlibc - 2.27 - 2.28\n\nмусор\n"
	got := parseUpgradable(out)
	if got["usque-keenetic"] != [2]string{"0.3.0", "0.4.0"} || len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestBinaryVersion(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, body string) string {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755)
		return p
	}
	if v := usqueBinaryVersion(t.Context(), mk("a", `echo "usque version 4.2.0"`)); v != "4.2.0" {
		t.Errorf("a: %q", v)
	}
	// длинная справка с цифрами не должна считаться версией
	help := strings.Repeat("Usage: foo http/2.0.1 bar baz\\n", 30)
	if v := usqueBinaryVersion(t.Context(), mk("b", `printf "`+help+`"`)); v != "" {
		t.Errorf("b: %q", v)
	}
	if v := usqueBinaryVersion(t.Context(), filepath.Join(dir, "нет")); v != "" {
		t.Errorf("c: %q", v)
	}
}

// Fake: init-скрипт ведёт «запущено» флагом-файлом, usque register создаёт session.
func newSessionEnv(t *testing.T) (*Service, Paths, string) {
	dir := t.TempDir()
	flag := filepath.Join(dir, "running")
	pid := filepath.Join(dir, "usque.pid")
	// «процесс usque»: копия sleep под именем usque
	bin := filepath.Join(dir, "usque")
	data, _ := os.ReadFile("/bin/sleep")
	_ = os.WriteFile(bin, data, 0o755)
	init := filepath.Join(dir, "S51usque")
	script := "#!/bin/sh\ncase \"$1\" in\n" +
		"start) " + bin + " 300 & echo $! > " + pid + "; echo started;;\n" +
		"stop) kill $(cat " + pid + "); rm -f " + pid + "; echo stopped;;\nesac\n"
	_ = os.WriteFile(init, []byte(script), 0o755)
	_ = flag
	p := Paths{InitScript: init, UsqueBin: bin, Conf: filepath.Join(dir, "usque.conf"),
		Session: filepath.Join(dir, "session.conf"), PidFile: pid}
	return NewService(p), p, dir
}

func TestResetStopsRunningService(t *testing.T) {
	s, p, _ := newSessionEnv(t)
	_ = os.WriteFile(p.Session, []byte("secret"), 0o600)
	if _, err := s.Control(t.Context(), "start"); err != nil {
		t.Fatal(err)
	}
	if _, alive := processAlive(p.PidFile, p.UsqueBin); !alive {
		t.Fatal("процесс должен быть запущен")
	}
	if err := s.ResetSession(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fileExists(p.Session) {
		t.Error("сессия не удалена")
	}
	if _, alive := processAlive(p.PidFile, p.UsqueBin); alive {
		t.Error("сервис должен быть остановлен")
	}
}

func TestRenewRestartsService(t *testing.T) {
	dir := t.TempDir()
	// «Процесс usque»: копия sleep с именем usque в отдельном каталоге
	// (processAlive сверяет только базовое имя argv[0]).
	procDir := filepath.Join(dir, "proc")
	_ = os.MkdirAll(procDir, 0o755)
	sleepData, _ := os.ReadFile("/bin/sleep")
	proc := filepath.Join(procDir, "usque")
	_ = os.WriteFile(proc, sleepData, 0o755)

	pid := filepath.Join(dir, "usque.pid")
	// UsqueBin — скрипт, умеющий register: usque register --accept-tos --config <файл>
	bin := filepath.Join(dir, "usque")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\nif [ \"$1\" = register ]; then echo new > \"$4\"; echo registered; fi\n"), 0o755)
	init := filepath.Join(dir, "S51usque")
	_ = os.WriteFile(init, []byte("#!/bin/sh\ncase \"$1\" in\n"+
		"start) "+proc+" 300 & echo $! > "+pid+"; echo started;;\n"+
		"stop) kill $(cat "+pid+"); rm -f "+pid+"; echo stopped;;\nesac\n"), 0o755)
	p := Paths{InitScript: init, UsqueBin: bin, Conf: filepath.Join(dir, "usque.conf"),
		Session: filepath.Join(dir, "session.conf"), PidFile: pid}
	s := NewService(p)
	_ = os.WriteFile(p.Session, []byte("old"), 0o600)

	if _, err := s.Control(t.Context(), "start"); err != nil {
		t.Fatal(err)
	}
	if _, alive := processAlive(p.PidFile, p.UsqueBin); !alive {
		t.Fatal("сервис должен быть запущен до перерегистрации")
	}
	out, err := s.Renew(t.Context())
	if err != nil {
		t.Fatalf("renew: %v (%s)", err, out)
	}
	b, _ := os.ReadFile(p.Session)
	if strings.TrimSpace(string(b)) != "new" {
		t.Errorf("сессия не перезаписана: %q", b)
	}
	if _, alive := processAlive(p.PidFile, p.UsqueBin); !alive {
		t.Error("сервис должен быть запущен снова")
	}
	_, _ = s.Control(t.Context(), "stop")
}
