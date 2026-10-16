package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Paths — расположение файлов usque-keenetic.
type Paths struct {
	InitScript string
	UsqueBin   string
	Conf       string
	Session    string
	PidFile    string
}

func defaultPaths() Paths {
	return Paths{
		InitScript: "/opt/etc/init.d/S51usque",
		UsqueBin:   "/opt/usr/bin/usque",
		Conf:       "/opt/etc/usque/usque.conf",
		Session:    "/opt/etc/usque/session.conf",
		PidFile:    "/opt/var/run/usque.pid",
	}
}

type LinkInfo struct {
	Exists  bool   `json:"exists"`
	Up      bool   `json:"up"`
	Running bool   `json:"running"`
	Oper    string `json:"oper,omitempty"`
	// Счётчики интерфейса (байты); по двум опросам GUI считает скорость.
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

type NDMSInfo struct {
	Available bool   `json:"available"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	Link      string `json:"link,omitempty"`
	Connected string `json:"connected,omitempty"`
	Address   string `json:"address,omitempty"`
	MTU       string `json:"mtu,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Status struct {
	// State: running | degraded | stopped
	State       string    `json:"state"`
	Running     bool      `json:"running"`
	PID         int       `json:"pid,omitempty"`
	Iface       string    `json:"iface"`
	Link        LinkInfo  `json:"link"`
	NDMS        NDMSInfo  `json:"ndms"`
	HasSession  bool      `json:"hasSession"`
	BinaryFound bool      `json:"binaryFound"`
	InitFound   bool      `json:"initFound"`
	Config      UsqueConf `json:"config"`
	// RunningConfig — параметры, с которыми процесс был запущен (usque.conf.run).
	RunningConfig *UsqueConf `json:"runningConfig,omitempty"`
	ConfigDiffers bool       `json:"configDiffers"`
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// processAlive проверяет pid из pidfile: процесс жив и это именно usque
// (argv[0], а не подстрока — иначе подошёл бы и сам usque-gui).
func processAlive(pidFile, bin string) (int, bool) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	cmd, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(cmd) == 0 {
		return pid, false
	}
	argv0 := strings.SplitN(string(cmd), "\x00", 2)[0]
	return pid, filepath.Base(argv0) == filepath.Base(bin)
}

func linkInfo(iface string) LinkInfo {
	base := "/sys/class/net/" + iface
	if !fileExists(base) {
		return LinkInfo{}
	}
	li := LinkInfo{Exists: true}
	if b, err := os.ReadFile(base + "/flags"); err == nil {
		if v, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "0x")), 16, 32); err == nil {
			li.Up = v&0x1 != 0       // IFF_UP
			li.Running = v&0x40 != 0 // IFF_RUNNING
		}
	}
	if b, err := os.ReadFile(base + "/operstate"); err == nil {
		li.Oper = strings.TrimSpace(string(b))
	}
	li.RxBytes = readCounter(base + "/statistics/rx_bytes")
	li.TxBytes = readCounter(base + "/statistics/tx_bytes")
	return li
}

func readCounter(path string) uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return v
}

// ndmName: opkgtun0 -> OpkgTun0.
func ndmName(iface string) string {
	if strings.HasPrefix(iface, "opkgtun") {
		return "OpkgTun" + strings.TrimPrefix(iface, "opkgtun")
	}
	return iface
}

var reNdmLine = regexp.MustCompile(`^\s*([a-z0-9_-]+):\s*(.*?)\s*$`)

// ndmsInfo спрашивает NDMS: сначала RCI по HTTP (127.0.0.1:79), затем ndmc.
func ndmsInfo(ctx context.Context, iface string) NDMSInfo {
	name := ndmName(iface)
	info := NDMSInfo{Name: name}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:79/rci/show/interface/"+name, nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var m map[string]any
			if json.NewDecoder(resp.Body).Decode(&m) == nil && len(m) > 0 {
				info.Available = true
				str := func(k string) string {
					switch v := m[k].(type) {
					case string:
						return v
					case float64:
						return strconv.FormatFloat(v, 'f', -1, 64)
					case bool:
						return strconv.FormatBool(v)
					}
					return ""
				}
				info.State, info.Link, info.Connected = str("state"), str("link"), str("connected")
				info.Address, info.MTU = str("address"), str("mtu")
				return info
			}
		}
	}

	out, err := exec.CommandContext(ctx, "ndmc", "-c", "show interface "+name).Output()
	if err != nil {
		info.Error = "NDMS недоступен"
		return info
	}
	info.Available = true
	for _, l := range strings.Split(string(out), "\n") {
		m := reNdmLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		switch m[1] {
		case "state":
			info.State = m[2]
		case "link":
			info.Link = m[2]
		case "connected":
			info.Connected = m[2]
		case "address":
			info.Address = m[2]
		case "mtu":
			info.MTU = m[2]
		}
	}
	return info
}

func collectStatus(ctx context.Context, p Paths) Status {
	st := Status{
		BinaryFound: fileExists(p.UsqueBin),
		InitFound:   fileExists(p.InitScript),
		HasSession:  fileExists(p.Session),
	}
	conf, err := readConf(p.Conf)
	if err != nil {
		conf = &UsqueConf{Iface: "opkgtun0"}
	}
	st.Config = *conf
	if run, err := readConf(p.Conf + ".run"); err == nil {
		st.RunningConfig = run
	}

	st.PID, st.Running = processAlive(p.PidFile, p.UsqueBin)

	// Интерфейс, который реально используется: у запущенного — из .run.
	iface := conf.Iface
	if st.RunningConfig != nil && st.Running && st.RunningConfig.Iface != "" {
		iface = st.RunningConfig.Iface
	}
	st.Iface = iface
	st.Link = linkInfo(iface)
	st.NDMS = ndmsInfo(ctx, iface)

	if st.Running && st.RunningConfig != nil {
		a, b := *st.RunningConfig, st.Config
		// IP/маска в .run — фактические (автовыбранные), в conf могут быть пустыми.
		if b.IfaceIP == "" {
			a.IfaceIP = ""
		}
		if b.IfaceMask == "" {
			a.IfaceMask = ""
		}
		a.SNI, a.HTTP2 = b.SNI, b.HTTP2 // эти поля .run не хранит
		st.ConfigDiffers = a != b
	}

	switch {
	case !st.Running:
		st.State = "stopped"
	case st.Link.Exists && st.Link.Up:
		st.State = "running"
	default:
		st.State = "degraded"
	}
	return st
}
