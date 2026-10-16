package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProbeResult — замер доступности одного endpoint'а Cloudflare.
type ProbeResult struct {
	IP    string  `json:"ip"`
	OK    bool    `json:"ok"`
	RttMs float64 `json:"rttMs,omitempty"` // лучшее время TCP-соединения с портом 443
	TLS   bool    `json:"tls"`             // удалось ли TLS-рукопожатие
	Error string  `json:"error,omitempty"`
}

const (
	maxProbeIPs   = 32
	probeAttempts = 3
	probePort     = "443"
)

// probePortOverride — только для тестов.
var probePortOverride = ""

// probeAllowPrivate — только для тестов: в обычной работе зондируются
// лишь публичные адреса (GUI не должен превращаться в сканер локальной сети).
var probeAllowPrivate = false

func publicIP(ip net.IP) bool {
	if probeAllowPrivate {
		return ip != nil
	}
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified()
}

// normalizeProbeIPs: уникальные валидные публичные IP, не более maxProbeIPs.
func normalizeProbeIPs(in []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, errors.New("не IP-адрес: " + s)
		}
		if !publicIP(ip) {
			return nil, errors.New("адрес не публичный: " + s)
		}
		k := ip.String()
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("список адресов пуст")
	}
	if len(out) > maxProbeIPs {
		return nil, errors.New("слишком много адресов (максимум 32)")
	}
	return out, nil
}

func probeOne(ctx context.Context, ip, sni string, timeout time.Duration) ProbeResult {
	r := ProbeResult{IP: ip}
	port := probePort
	if probePortOverride != "" {
		port = probePortOverride
	}
	addr := net.JoinHostPort(ip, port)
	var best time.Duration
	var lastErr error
	for i := 0; i < probeAttempts; i++ {
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			break
		}
		start := time.Now()
		d := net.Dialer{Timeout: timeout}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		el := time.Since(start)
		if !r.OK || el < best {
			best = el
		}
		if !r.OK {
			// TLS-рукопожатие один раз: подтверждает, что отвечает именно веб-сервис.
			_ = conn.SetDeadline(time.Now().Add(timeout))
			tc := tls.Client(conn, &tls.Config{ServerName: sni, InsecureSkipVerify: true}) //nolint:gosec // проверяем только доступность
			if tc.HandshakeContext(ctx) == nil {
				r.TLS = true
			}
		}
		r.OK = true
		_ = conn.Close()
	}
	if r.OK {
		r.RttMs = float64(best.Microseconds()) / 1000
		return r
	}
	if lastErr != nil {
		r.Error = shortNetErr(lastErr)
	}
	return r
}

func shortNetErr(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "таймаут"
	case strings.Contains(err.Error(), "refused"):
		return "отказ в соединении"
	case strings.Contains(err.Error(), "unreachable"), strings.Contains(err.Error(), "no route"):
		return "сеть недоступна"
	}
	return err.Error()
}

// ProbeEndpoints проверяет адреса параллельно (до 8) и сортирует: рабочие по RTT, затем остальные.
func ProbeEndpoints(ctx context.Context, ips []string, sni string, timeout time.Duration) []ProbeResult {
	if sni == "" {
		sni = "cloudflare.com"
	}
	res := make([]ProbeResult, len(ips))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			res[i] = probeOne(ctx, ip, sni, timeout)
		}(i, ip)
	}
	wg.Wait()
	sort.SliceStable(res, func(a, b int) bool {
		if res[a].OK != res[b].OK {
			return res[a].OK
		}
		return res[a].RttMs < res[b].RttMs
	})
	return res
}
