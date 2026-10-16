package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// LogEntry — запись журнала действий GUI (у самого usque логов нет:
// init-скрипт запускает его без перенаправления вывода).
type LogEntry struct {
	Time   time.Time `json:"time"`
	Action string    `json:"action"`
	OK     bool      `json:"ok"`
	Output string    `json:"output"`
}

type Service struct {
	paths Paths

	busy sync.Mutex // одна операция управления за раз

	mu             sync.Mutex
	log            []LogEntry
	restartPending bool
}

const maxLog = 50

func NewService(p Paths) *Service { return &Service{paths: p} }

func (s *Service) addLog(action string, ok bool, out string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, LogEntry{Time: time.Now(), Action: action, OK: ok, Output: out})
	if len(s.log) > maxLog {
		s.log = s.log[len(s.log)-maxLog:]
	}
}

func (s *Service) Log() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogEntry, len(s.log))
	copy(out, s.log)
	return out
}

func (s *Service) RestartPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restartPending
}

func (s *Service) setRestartPending(v bool) {
	s.mu.Lock()
	s.restartPending = v
	s.mu.Unlock()
}

const maxOut = 8 << 10

func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "PATH=/opt/sbin:/opt/bin:/opt/usr/sbin:/opt/usr/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	// start-stop-daemon оставляет демона в той же группе: не ждать закрытия pipe'ов потомком.
	cmd.WaitDelay = 2 * time.Second
	b, err := cmd.CombinedOutput()
	// Команда завершилась успешно, но потомок-демон держит pipe открытым:
	// для нас это успех.
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	out := strings.TrimSpace(string(b))
	if len(out) > maxOut {
		out = "…" + out[len(out)-maxOut:]
	}
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("превышено время ожидания (%s)", timeout)
	}
	return out, err
}

var errBusy = errors.New("другая операция ещё выполняется")

// Control: start | stop | restart.
func (s *Service) Control(ctx context.Context, action string) (string, error) {
	switch action {
	case "start", "stop", "restart":
	default:
		return "", errors.New("неизвестное действие")
	}
	if !s.busy.TryLock() {
		return "", errBusy
	}
	defer s.busy.Unlock()
	return s.control(ctx, action)
}

// control — Control без захвата busy (вызывающий уже держит его).
func (s *Service) control(ctx context.Context, action string) (string, error) {
	if !fileExists(s.paths.InitScript) {
		return "", errors.New("init-скрипт usque-keenetic не найден: " + s.paths.InitScript)
	}
	out, err := run(ctx, 90*time.Second, s.paths.InitScript, action)
	s.addLog(action, err == nil, out)
	if err == nil && (action == "start" || action == "restart") {
		s.setRestartPending(false)
	}
	return out, err
}

func (s *Service) Register(ctx context.Context) (string, error) {
	if !s.busy.TryLock() {
		return "", errBusy
	}
	defer s.busy.Unlock()
	return s.register(ctx)
}

func (s *Service) register(ctx context.Context) (string, error) {
	if fileExists(s.paths.Session) {
		return "", errors.New("сессия уже существует; воспользуйтесь перерегистрацией")
	}
	if !fileExists(s.paths.UsqueBin) {
		return "", errors.New("бинарник usque не найден: " + s.paths.UsqueBin)
	}
	out, err := run(ctx, 90*time.Second, s.paths.UsqueBin, "register", "--accept-tos", "--config", s.paths.Session)
	if err == nil && !fileExists(s.paths.Session) {
		err = errors.New("регистрация завершилась, но файл сессии не создан")
	}
	if err == nil {
		_ = os.Chmod(s.paths.Session, 0o600)
	}
	s.addLog("register", err == nil, out)
	return out, err
}

// ResetSession удаляет session.conf; запущенный сервис предварительно
// останавливается (иначе usque держит ключи в памяти).
func (s *Service) ResetSession(ctx context.Context) error {
	if !s.busy.TryLock() {
		return errBusy
	}
	defer s.busy.Unlock()
	_, err := s.dropSession(ctx)
	return err
}

// dropSession: остановить сервис (если запущен) и удалить сессию.
// Возвращает, был ли сервис запущен.
func (s *Service) dropSession(ctx context.Context) (wasRunning bool, err error) {
	if _, alive := processAlive(s.paths.PidFile, s.paths.UsqueBin); alive {
		wasRunning = true
		if _, err := s.control(ctx, "stop"); err != nil {
			return true, fmt.Errorf("не удалось остановить сервис: %w", err)
		}
	}
	if err := os.Remove(s.paths.Session); err != nil && !os.IsNotExist(err) {
		s.addLog("reset-session", false, err.Error())
		return wasRunning, err
	}
	s.addLog("reset-session", true, "session.conf удалён")
	return wasRunning, nil
}

// Renew: сброс сессии и регистрация нового аккаунта; сервис, если был
// запущен, поднимается снова.
func (s *Service) Renew(ctx context.Context) (string, error) {
	if !s.busy.TryLock() {
		return "", errBusy
	}
	defer s.busy.Unlock()

	wasRunning, err := s.dropSession(ctx)
	if err != nil {
		return "", err
	}
	out, err := s.register(ctx)
	if err != nil {
		return out, err
	}
	if wasRunning {
		if o, err := s.control(ctx, "start"); err != nil {
			return out + "\n" + o, fmt.Errorf("аккаунт создан, но сервис не запустился: %w", err)
		}
	}
	return out, nil
}

type CheckResult struct {
	IP    string `json:"ip,omitempty"`
	Warp  string `json:"warp,omitempty"`
	Loc   string `json:"loc,omitempty"`
	Colo  string `json:"colo,omitempty"`
	Error string `json:"error,omitempty"`
}

// CheckExit запрашивает Cloudflare trace, привязав сокет к интерфейсу usque
// (SO_BINDTODEVICE, нужен root). Показывает внешний IP и warp=on/plus.
func CheckExit(ctx context.Context, iface string) CheckResult {
	dialer := &net.Dialer{
		Timeout: 6 * time.Second,
		Control: func(_, _ string, c syscall.RawConn) error {
			var serr error
			if err := c.Control(func(fd uintptr) {
				serr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
			}); err != nil {
				return err
			}
			return serr
		},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, DisableKeepAlives: true},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.cloudflare.com/cdn-cgi/trace", nil)
	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{Error: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r CheckResult
	for _, l := range strings.Split(string(body), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		switch k {
		case "ip":
			r.IP = v
		case "warp":
			r.Warp = v
		case "loc":
			r.Loc = v
		case "colo":
			r.Colo = v
		}
	}
	if r.IP == "" {
		r.Error = "неожиданный ответ Cloudflare"
	}
	return r
}
