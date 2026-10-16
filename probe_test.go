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

func TestProbeEndpoints(t *testing.T) {
	probeAllowPrivate = true
	defer func() { probeAllowPrivate = false }()

	// Рабочий «endpoint»: TCP-порт 443 нельзя занять без root, поэтому проверяем
	// логику на недоступном адресе и на сортировке через probeOne напрямую.
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
	// подменяем порт зондирования на порт слушателя
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	old := probePortOverride
	probePortOverride = port
	defer func() { probePortOverride = old }()

	res := ProbeEndpoints(t.Context(), []string{"127.0.0.1"}, "", time.Second)
	if len(res) != 1 || !res[0].OK || res[0].RttMs <= 0 {
		t.Fatalf("ожидался рабочий endpoint: %+v", res)
	}

	// закрытый порт → ошибка, и такой результат идёт после рабочих
	probePortOverride = "1"
	res = ProbeEndpoints(t.Context(), []string{"127.0.0.1"}, "", time.Second)
	if len(res) != 1 || res[0].OK || res[0].Error == "" {
		t.Fatalf("ожидалась ошибка: %+v", res)
	}
}
