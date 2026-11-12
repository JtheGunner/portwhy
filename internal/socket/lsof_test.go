package socket

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "darwin", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func containsSocket(sockets []Socket, want Socket) bool {
	return slices.Contains(sockets, want)
}

func TestParseLsof_ListenIPv4(t *testing.T) {
	got, err := parseLsof(readFixture(t, "lsof_listen.txt"))
	if err != nil {
		t.Fatalf("parseLsof: %v", err)
	}

	for _, want := range []Socket{
		sock(49179, TCP, "127.0.0.1", Listen, 1381),
		sock(41343, TCP, "127.0.0.1", Listen, 1382),
		sock(63665, UDP, "0.0.0.0", Listen, 1154),
		sock(52911, UDP, "0.0.0.0", Listen, 1215),
	} {
		if !containsSocket(got, want) {
			t.Errorf("missing %v in %v", want, got)
		}
	}
}

func TestParseLsof_IPv6Wildcard(t *testing.T) {
	got, err := parseLsof([]byte("p77\nf5\nPTCP\nn[::]:9000\nf6\nPTCP\nn*:9000\n"))
	if err != nil {
		t.Fatalf("parseLsof: %v", err)
	}

	want := []Socket{
		sock(9000, TCP, "::", Listen, 77),
		sock(9000, TCP, "0.0.0.0", Listen, 77),
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseLsof() = %v, want %v", got, want)
	}
}

func TestParseLsof_IPv6Loopback(t *testing.T) {
	got, err := parseLsof(readFixture(t, "lsof_listen.txt"))
	if err != nil {
		t.Fatalf("parseLsof: %v", err)
	}
	if want := sock(7679, TCP, "::1", Listen, 1420); !containsSocket(got, want) {
		t.Errorf("missing %v in %v", want, got)
	}
}

func TestParseLsof_SkipsUnboundAndConnectedEntries(t *testing.T) {
	got, err := parseLsof(readFixture(t, "lsof_listen.txt"))
	if err != nil {
		t.Fatalf("parseLsof: %v", err)
	}

	for _, s := range got {
		if s.Port == 0 || s.Port == 443 || s.Port == 49808 {
			t.Errorf("unexpected socket %v", s)
		}
	}
}

func TestParseLsof_EmptyInput(t *testing.T) {
	got, err := parseLsof(nil)
	if err != nil || len(got) != 0 {
		t.Errorf("parseLsof(nil) = %v, %v; want no sockets and no error", got, err)
	}
}

func TestParseLsof_MalformedInput(t *testing.T) {
	cases := map[string]string{
		"address before pid":  "PTCP\nn*:80\n",
		"address before type": "p1\nn*:80\n",
		"bad pid":             "pabc\nPTCP\nn*:80\n",
		"bad port":            "p1\nPTCP\nn*:http\n",
		"bad address":         "p1\nPTCP\nnnot-an-ip:80\n",
		"unknown protocol":    "p1\nPSCTP\nn*:80\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := parseLsof([]byte(in)); err == nil {
				t.Errorf("parseLsof() = %v, want an error", got)
			}
		})
	}
}
