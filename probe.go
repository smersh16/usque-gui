package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Режимы подбора (как в usque: QUIC по UDP или HTTP/2 по TCP).
const (
	ModeQUIC = "quic"
	ModeH2   = "h2"
)

// ProbeResult — замер доступности одного endpoint'а Cloudflare.
// Endpoint'ы нестабильны, поэтому проверка повторяется несколько раз.
type ProbeResult struct {
	IP       string  `json:"ip"`
	OK       bool    `json:"ok"` // хотя бы одна попытка удалась
	Attempts int     `json:"attempts"`
	Success  int     `json:"success"`
	RttMs    float64 `json:"rttMs,omitempty"` // лучшее время отклика среди успешных попыток
	TLS      bool    `json:"tls"`             // только для h2: удалось ли TLS-рукопожатие
	Error    string  `json:"error,omitempty"`
}

const (
	maxProbeIPs     = 32
	maxProbeTries   = 10
	defaultTries    = 3
	probePort       = "443" // usque-keenetic не задаёт порт (-P), значит работает 443
	defaultProbeSNI = "consumer-masque.cloudflareclient.com"
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

func probePortFor() string {
	if probePortOverride != "" {
		return probePortOverride
	}
	return probePort
}

// probeTCPOnce: TCP-соединение (+ опционально TLS-рукопожатие) — режим HTTP/2.
func probeTCPOnce(ctx context.Context, ip, sni string, timeout time.Duration, doTLS bool) (time.Duration, bool, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, probePortFor()))
	if err != nil {
		return 0, false, err
	}
	defer conn.Close()
	el := time.Since(start)
	tlsOK := false
	if doTLS {
		_ = conn.SetDeadline(time.Now().Add(timeout))
		tc := tls.Client(conn, &tls.Config{ServerName: sni, InsecureSkipVerify: true}) //nolint:gosec // проверяем только доступность
		tlsOK = tc.HandshakeContext(ctx) == nil
	}
	return el, tlsOK, nil
}

// quicVNProbe проверяет, что по UDP на адресе отвечает QUIC-сервер: шлёт
// пакет Initial с зарезервированной (неизвестной) версией QUIC, а в ответ
// ждёт Version Negotiation (RFC 9000, §6, §17.2.1). Шифрование не нужно.
// Датаграмма — не менее 1200 байт, иначе сервер не обязан отвечать.
// Ответ подтверждает QUIC-стек на адресе и даёт RTT, но не саму работу MASQUE.
func quicVNProbe(ctx context.Context, ip string, timeout time.Duration) (time.Duration, error) {
	raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(ip, probePortFor()))
	if err != nil {
		return 0, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	dcid := make([]byte, 8)
	scid := make([]byte, 8)
	if _, err := rand.Read(dcid); err != nil {
		return 0, err
	}
	if _, err := rand.Read(scid); err != nil {
		return 0, err
	}
	pkt := make([]byte, 1200)
	pkt[0] = 0xC0                                    // long header, fixed bit
	binary.BigEndian.PutUint32(pkt[1:5], 0x1a2a3a4a) // «греазная» версия вида 0x?a?a?a?a
	pkt[5] = byte(len(dcid))
	copy(pkt[6:], dcid)
	pkt[6+len(dcid)] = byte(len(scid))
	copy(pkt[7+len(dcid):], scid)

	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	start := time.Now()
	if _, err := conn.Write(pkt); err != nil {
		return 0, err
	}
	buf := make([]byte, 2048)
	for {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		n, err := conn.Read(buf)
		if err != nil {
			return 0, err
		}
		el := time.Since(start)
		if isVersionNegotiation(buf[:n], scid, dcid) {
			return el, nil
		}
		// посторонний пакет — ждём дальше до дедлайна
	}
}

// isVersionNegotiation: пакет Version Negotiation, адресованный нам
// (его DCID — наш SCID, SCID — наш DCID).
func isVersionNegotiation(p, ourSCID, ourDCID []byte) bool {
	if len(p) < 7 || p[0]&0x80 == 0 || binary.BigEndian.Uint32(p[1:5]) != 0 {
		return false
	}
	dl := int(p[5])
	if len(p) < 6+dl+1 {
		return false
	}
	if !bytes.Equal(p[6:6+dl], ourSCID) {
		return false
	}
	sl := int(p[6+dl])
	if len(p) < 7+dl+sl {
		return false
	}
	return bytes.Equal(p[7+dl:7+dl+sl], ourDCID)
}

func probeOne(ctx context.Context, mode, ip, sni string, tries int, timeout time.Duration) ProbeResult {
	r := ProbeResult{IP: ip, Attempts: tries}
	var best time.Duration
	var lastErr error
	for i := 0; i < tries; i++ {
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			break
		}
		var el time.Duration
		var err error
		if mode == ModeQUIC {
			el, err = quicVNProbe(ctx, ip, timeout)
		} else {
			var tlsOK bool
			el, tlsOK, err = probeTCPOnce(ctx, ip, sni, timeout, !r.TLS)
			if err == nil && tlsOK {
				r.TLS = true
			}
		}
		if err != nil {
			lastErr = err
			continue
		}
		r.Success++
		if !r.OK || el < best {
			best = el
		}
		r.OK = true
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

// ProbeEndpoints проверяет адреса параллельно (до 8). Сортировка: больше
// успешных попыток — выше, затем меньше задержка, неотвечающие — в конце.
func ProbeEndpoints(ctx context.Context, mode string, ips []string, sni string, tries int, timeout time.Duration) []ProbeResult {
	if sni == "" {
		sni = defaultProbeSNI
	}
	if tries < 1 {
		tries = defaultTries
	}
	if tries > maxProbeTries {
		tries = maxProbeTries
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
			res[i] = probeOne(ctx, mode, ip, sni, tries, timeout)
		}(i, ip)
	}
	wg.Wait()
	sortProbeResults(res)
	return res
}

// sortProbeResults: больше успешных попыток — выше, затем меньше задержка.
func sortProbeResults(res []ProbeResult) {
	sort.SliceStable(res, func(a, b int) bool {
		if res[a].Success != res[b].Success {
			return res[a].Success > res[b].Success
		}
		if res[a].OK && res[b].OK {
			return res[a].RttMs < res[b].RttMs
		}
		return false
	})
}
