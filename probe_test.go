package main

import (
	"net"
	"testing"
	"time"
)

func TestNormalizeProbeIPs(t *testing.T) {
	got, err := normalizeProbeIPs([]string{" 162.159.198.2 ", "162.159.198.2", "2606:4700:103::", ""})
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range [][]string{
		{"192.168.1.1"}, {"127.0.0.1"}, {"10.0.0.1"}, {"fe80::1"}, {"0.0.0.0"}, {"example.com"}, {"1.2.3.4:443"}, {},
	} {
		if _, err := normalizeProbeIPs(bad); err == nil {
			t.Errorf("должно отклоняться: %v", bad)
		}
	}
	many := make([]string, 40)
	for i := range many {
		many[i] = net.IPv4(8, 8, byte(i/250), byte(i%250+1)).String()
	}
	if _, err := normalizeProbeIPs(many); err == nil {
		t.Error("больше 32 адресов должно отклоняться")
	}
}

// fakeQUICServer отвечает на Initial-пакеты с неизвестной версией пакетом
// Version Negotiation (как настоящий QUIC-сервер). respond=false — молчит.
func fakeQUICServer(t *testing.T, respond bool) string {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if !respond || n < 1200 || buf[0]&0x80 == 0 {
				continue
			}
			dl := int(buf[5])
			dcid := buf[6 : 6+dl]
			sl := int(buf[6+dl])
			scid := buf[7+dl : 7+dl+sl]
			// VN: флаги, версия 0, DCID=наш SCID, SCID=наш DCID, список версий (v1)
			vn := []byte{0x80, 0, 0, 0, 0, byte(sl)}
			vn = append(vn, scid...)
			vn = append(vn, byte(dl))
			vn = append(vn, dcid...)
			vn = append(vn, 0, 0, 0, 1)
			_, _ = pc.WriteTo(vn, addr)
		}
	}()
	_, port, _ := net.SplitHostPort(pc.LocalAddr().String())
	return port
}

func withProbeTest(t *testing.T, port string) {
	probeAllowPrivate = true
	probePortOverride = port
	t.Cleanup(func() { probeAllowPrivate = false; probePortOverride = "" })
}

func TestProbeQUICVersionNegotiation(t *testing.T) {
	withProbeTest(t, fakeQUICServer(t, true))
	res := ProbeEndpoints(t.Context(), ModeQUIC, []string{"127.0.0.1"}, "", 3, time.Second)
	if len(res) != 1 || !res[0].OK || res[0].Success != 3 || res[0].RttMs <= 0 {
		t.Fatalf("ожидался ответивший QUIC-сервер 3/3: %+v", res)
	}
}

func TestProbeQUICSilentServer(t *testing.T) {
	withProbeTest(t, fakeQUICServer(t, false))
	res := ProbeEndpoints(t.Context(), ModeQUIC, []string{"127.0.0.1"}, "", 1, 300*time.Millisecond)
	if len(res) != 1 || res[0].OK || res[0].Error == "" {
		t.Fatalf("молчащий сервер должен давать ошибку: %+v", res)
	}
}

func TestIsVersionNegotiation(t *testing.T) {
	ourS, ourD := []byte{1, 2, 3, 4, 5, 6, 7, 8}, []byte{9, 9, 9, 9, 9, 9, 9, 9}
	good := append([]byte{0x80, 0, 0, 0, 0, 8}, ourS...)
	good = append(good, 8)
	good = append(good, ourD...)
	good = append(good, 0, 0, 0, 1)
	if !isVersionNegotiation(good, ourS, ourD) {
		t.Error("корректный VN не распознан")
	}
	if isVersionNegotiation(good, ourD, ourS) {
		t.Error("чужие connection id должны отклоняться")
	}
	short := good[:6]
	if isVersionNegotiation(short, ourS, ourD) || isVersionNegotiation(nil, ourS, ourD) {
		t.Error("обрезанные пакеты должны отклоняться")
	}
	notVN := append([]byte{0xC0, 0, 0, 0, 1}, good[5:]...) // версия 1, не 0
	if isVersionNegotiation(notVN, ourS, ourD) {
		t.Error("пакет с версией ≠ 0 не VN")
	}
}

func TestProbeTCPMode(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	withProbeTest(t, port)
	res := ProbeEndpoints(t.Context(), ModeH2, []string{"127.0.0.1"}, "", 3, time.Second)
	if len(res) != 1 || !res[0].OK || res[0].Success != 3 {
		t.Fatalf("ожидался рабочий endpoint: %+v", res)
	}
	probePortOverride = "1"
	res = ProbeEndpoints(t.Context(), ModeH2, []string{"127.0.0.1"}, "", 2, time.Second)
	if len(res) != 1 || res[0].OK || res[0].Error == "" || res[0].Success != 0 {
		t.Fatalf("ожидалась ошибка: %+v", res)
	}
}

func TestProbeSortsBySuccessThenRTT(t *testing.T) {
	// сортировка: больше успехов — выше; при равенстве — меньше RTT
	rs := []ProbeResult{
		{IP: "a", OK: true, Success: 1, RttMs: 5},
		{IP: "b", OK: false},
		{IP: "c", OK: true, Success: 3, RttMs: 20},
		{IP: "d", OK: true, Success: 3, RttMs: 10},
	}
	sortProbeResults(rs)
	if rs[0].IP != "d" || rs[1].IP != "c" || rs[2].IP != "a" || rs[3].IP != "b" {
		t.Fatalf("%+v", rs)
	}
}
