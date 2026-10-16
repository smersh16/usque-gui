package main

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const opkgStatus = "/opt/lib/opkg/status"

// Управляемые пакеты. Обновлять из GUI разрешено только usque-keenetic:
// обновление usque-gui остановило бы сам интерфейс посреди запроса.
var watchedPkgs = []string{"usque-keenetic", "usque-gui"}

type PkgVersion struct {
	Name        string `json:"name"`
	Installed   string `json:"installed,omitempty"`
	Available   string `json:"available,omitempty"`
	Upgradable  bool   `json:"upgradable"`
	BinaryVer   string `json:"binaryVersion,omitempty"` // версия самого бинарника usque, если он её отдаёт
	CanUpgrade  bool   `json:"canUpgrade"`              // можно ли обновить кнопкой в GUI
	UpgradeHint string `json:"upgradeHint,omitempty"`
}

type VersionsInfo struct {
	Packages  []PkgVersion `json:"packages"`
	CheckedAt *time.Time   `json:"checkedAt,omitempty"`
	Warning   string       `json:"warning,omitempty"`
}

var (
	updMu      sync.Mutex
	lastCheck  *time.Time
	lastWarn   string
	lastUpg    = map[string]string{} // pkg -> доступная версия по итогам проверки
	reSemverIn = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:[-+][\w.]+)?)`)
)

// parseOpkgStatus достаёт Version нужных пакетов из /opt/lib/opkg/status.
func parseOpkgStatus(data string, names []string) map[string]string {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	res := map[string]string{}
	for _, block := range strings.Split(data, "\n\n") {
		var name, ver string
		for _, l := range strings.Split(block, "\n") {
			if v, ok := strings.CutPrefix(l, "Package: "); ok {
				name = strings.TrimSpace(v)
			} else if v, ok := strings.CutPrefix(l, "Version: "); ok {
				ver = strings.TrimSpace(v)
			}
		}
		if want[name] && ver != "" {
			res[name] = ver
		}
	}
	return res
}

// parseUpgradable разбирает вывод `opkg list-upgradable`:
// "name - installed - available".
func parseUpgradable(out string) map[string][2]string {
	res := map[string][2]string{}
	for _, l := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(l), " - ")
		if len(parts) == 3 {
			res[strings.TrimSpace(parts[0])] = [2]string{strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])}
		}
	}
	return res
}

// usqueBinaryVersion пробует `usque version` / `usque --version`.
func usqueBinaryVersion(ctx context.Context, bin string) string {
	if !fileExists(bin) {
		return ""
	}
	for _, args := range [][]string{{"version"}, {"--version"}} {
		out, err := run(ctx, 5*time.Second, bin, args...)
		if err != nil {
			continue
		}
		// Короткий ответ с версией вида X.Y.Z; длинный вывод — это справка
		// (команда не распознана), цифры оттуда версией не считаем.
		if len(out) > 300 {
			continue
		}
		if m := reSemverIn.FindStringSubmatch(out); m != nil {
			return m[1]
		}
	}
	return ""
}

func (a *App) versions(ctx context.Context) VersionsInfo {
	var installed map[string]string
	if b, err := os.ReadFile(opkgStatus); err == nil {
		installed = parseOpkgStatus(string(b), watchedPkgs)
	}
	updMu.Lock()
	defer updMu.Unlock()
	info := VersionsInfo{CheckedAt: lastCheck, Warning: lastWarn}
	for _, n := range watchedPkgs {
		pv := PkgVersion{Name: n, Installed: installed[n]}
		if n == "usque-gui" && pv.Installed == "" {
			pv.Installed = version // запуск не из пакета (отладка)
		}
		if av, ok := lastUpg[n]; ok {
			pv.Available, pv.Upgradable = av, true
		}
		pv.CanUpgrade = n == "usque-keenetic"
		if n == "usque-gui" {
			pv.UpgradeHint = "opkg update && opkg upgrade usque-gui"
		}
		if n == "usque-keenetic" {
			pv.BinaryVer = usqueBinaryVersion(ctx, a.paths.UsqueBin)
		}
		info.Packages = append(info.Packages, pv)
	}
	return info
}

var errOpkgMissing = errors.New("opkg не найден (нужен Entware)")

// checkUpdates: opkg update + opkg list-upgradable.
func (a *App) checkUpdates(ctx context.Context) (VersionsInfo, error) {
	if !a.svc.busy.TryLock() {
		return VersionsInfo{}, errBusy
	}
	defer a.svc.busy.Unlock()

	warn := ""
	out, err := run(ctx, 120*time.Second, "opkg", "update")
	if err != nil && strings.Contains(err.Error(), "executable file not found") {
		return VersionsInfo{}, errOpkgMissing
	}
	if err != nil {
		warn = "opkg update завершился с ошибкой (данные могут быть устаревшими): " + lastLine(out, err)
	}
	upg, err2 := run(ctx, 30*time.Second, "opkg", "list-upgradable")
	if err2 != nil {
		return VersionsInfo{}, errors.New("opkg list-upgradable: " + lastLine(upg, err2))
	}
	parsed := parseUpgradable(upg)

	updMu.Lock()
	now := time.Now()
	lastCheck, lastWarn = &now, warn
	lastUpg = map[string]string{}
	for _, n := range watchedPkgs {
		if v, ok := parsed[n]; ok {
			lastUpg[n] = v[1]
		}
	}
	updMu.Unlock()

	a.svc.addLog("check-updates", warn == "", "проверка обновлений выполнена"+ifThen(warn != "", ": "+warn, ""))
	return a.versions(ctx), nil
}

func ifThen(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

func lastLine(out string, err error) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return err.Error()
	}
	l := strings.Split(out, "\n")
	return l[len(l)-1]
}

// upgradePkg обновляет usque-keenetic (postinst пакета сам перезапускает сервис).
func (a *App) upgradePkg(ctx context.Context, pkg string) (string, error) {
	if pkg != "usque-keenetic" {
		return "", errors.New("из интерфейса можно обновить только usque-keenetic")
	}
	if !a.svc.busy.TryLock() {
		return "", errBusy
	}
	defer a.svc.busy.Unlock()

	out, err := run(ctx, 300*time.Second, "opkg", "upgrade", pkg)
	a.svc.addLog("upgrade "+pkg, err == nil, out)
	if err == nil {
		updMu.Lock()
		delete(lastUpg, pkg)
		updMu.Unlock()
	}
	return out, err
}
