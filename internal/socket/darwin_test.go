//go:build darwin

package socket

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/execx"
)

const (
	lsofCommand       = "lsof -nP -iTCP -sTCP:LISTEN -iUDP -F pPn"
	netstatTCPCommand = "netstat -an -p tcp"
	netstatUDPCommand = "netstat -an -p udp"
)

func fakeRunner(lsofOut, tcpOut, udpOut string) *execx.Fake {
	return &execx.Fake{Responses: map[string][]byte{
		lsofCommand:       []byte(lsofOut),
		netstatTCPCommand: []byte(tcpOut),
		netstatUDPCommand: []byte(udpOut),
	}}
}

func TestDarwinCollector_NetstatOnlyListenerKeepsPIDZero(t *testing.T) {
	r := fakeRunner("", "tcp4       0      0  *.3000                 *.*                    LISTEN\n", "")

	got, err := NewDarwinCollector(r).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets: %v", err)
	}

	want := []Socket{sock(3000, TCP, "0.0.0.0", Listen, 0)}
	if !slices.Equal(got, want) {
		t.Errorf("Sockets() = %v, want %v", got, want)
	}
}

func TestDarwinCollector_DualStackIsOneSocket(t *testing.T) {
	lsofOut := "p1642\nf6\nPTCP\nn*:8123\nf7\nPTCP\nn[::]:8123\n"
	tcpOut := "tcp46      0      0  *.8123                 *.*                    LISTEN\n"

	got, err := NewDarwinCollector(fakeRunner(lsofOut, tcpOut, "")).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets: %v", err)
	}

	want := []Socket{sock(8123, TCP, "0.0.0.0", Listen, 1642)}
	if !slices.Equal(got, want) {
		t.Errorf("Sockets() = %v, want %v", got, want)
	}
}

func TestDarwinCollector_LsofEntryWinsOverNetstat(t *testing.T) {
	lsofOut := "p77\nPTCP\nn127.0.0.1:5000\n"
	tcpOut := "tcp4       0      0  127.0.0.1.5000         *.*                    LISTEN\n" +
		"tcp4       0      0  *.5001                 *.*                    LISTEN\n"

	got, err := NewDarwinCollector(fakeRunner(lsofOut, tcpOut, "")).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets: %v", err)
	}

	want := []Socket{
		sock(5000, TCP, "127.0.0.1", Listen, 77),
		sock(5001, TCP, "0.0.0.0", Listen, 0),
	}
	if !slices.Equal(got, want) {
		t.Errorf("Sockets() = %v, want %v", got, want)
	}
}

func TestDarwinCollector_TimeWaitIsKept(t *testing.T) {
	tcpOut := "tcp4       0      0  127.0.0.1.65066        127.0.0.1.5037         TIME_WAIT\n"

	got, err := NewDarwinCollector(fakeRunner("", tcpOut, "")).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets: %v", err)
	}

	want := []Socket{sock(65066, TCP, "127.0.0.1", TimeWait, 0)}
	if !slices.Equal(got, want) {
		t.Errorf("Sockets() = %v, want %v", got, want)
	}
}

func TestDarwinCollector_RealFixtures(t *testing.T) {
	r := fakeRunner(
		string(readFixture(t, "lsof_listen.txt")),
		string(readFixture(t, "netstat_tcp.txt")),
		string(readFixture(t, "netstat_udp.txt")),
	)

	got, err := NewDarwinCollector(r).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets: %v", err)
	}

	for _, want := range []Socket{
		sock(49154, TCP, "0.0.0.0", Listen, 1113),
		sock(3000, TCP, "0.0.0.0", Listen, 0),
		sock(22, TCP, "0.0.0.0", Listen, 0),
		sock(7679, TCP, "::1", Listen, 1420),
	} {
		if !containsSocket(got, want) {
			t.Errorf("missing %v in %v", want, got)
		}
	}
}

// exitError returns a real *exec.ExitError with the given status.
func exitError(t *testing.T, code string) error {
	t.Helper()
	_, err := execx.NewRunner(5*time.Second).Run(context.Background(), "sh", "-c", "exit "+code)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected an exit error, got %v", err)
	}
	return err
}

func TestDarwinCollector_LsofExitOneWithEmptyOutputIsNotAnError(t *testing.T) {
	r := fakeRunner("", "tcp4       0      0  *.3000                 *.*                    LISTEN\n", "")
	r.Errors = map[string]error{lsofCommand: exitError(t, "1")}

	got, err := NewDarwinCollector(r).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets: %v", err)
	}
	if want := []Socket{sock(3000, TCP, "0.0.0.0", Listen, 0)}; !slices.Equal(got, want) {
		t.Errorf("Sockets() = %v, want %v", got, want)
	}
}

func TestDarwinCollector_LsofExitOneKeepsPartialOutput(t *testing.T) {
	r := fakeRunner("p9\nPTCP\nn*:4000\n", "", "")
	r.Errors = map[string]error{lsofCommand: exitError(t, "1")}

	got, err := NewDarwinCollector(r).Sockets(context.Background())
	if err != nil {
		t.Fatalf("Sockets: %v", err)
	}
	if want := []Socket{sock(4000, TCP, "0.0.0.0", Listen, 9)}; !slices.Equal(got, want) {
		t.Errorf("Sockets() = %v, want %v", got, want)
	}
}

func TestDarwinCollector_LsofMissingIsAnError(t *testing.T) {
	r := fakeRunner("", "", "")
	r.Errors = map[string]error{lsofCommand: exec.ErrNotFound}

	_, err := NewDarwinCollector(r).Sockets(context.Background())
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("error = %v, want one wrapping exec.ErrNotFound", err)
	}
}

func TestDarwinCollector_LsofOtherExitCodeIsAnError(t *testing.T) {
	r := fakeRunner("", "", "")
	r.Errors = map[string]error{lsofCommand: exitError(t, "2")}

	if _, err := NewDarwinCollector(r).Sockets(context.Background()); err == nil {
		t.Fatal("Sockets returned nil error for lsof exit status 2")
	}
}

func TestDarwinCollector_NetstatFailureIsAnError(t *testing.T) {
	boom := errors.New("boom")
	r := fakeRunner("", "", "")
	r.Errors = map[string]error{netstatTCPCommand: boom}

	_, err := NewDarwinCollector(r).Sockets(context.Background())
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "netstat") {
		t.Fatalf("error = %v, want one wrapping boom and naming netstat", err)
	}
}

func TestDarwinCollector_MalformedLsofOutputIsAnError(t *testing.T) {
	if _, err := NewDarwinCollector(fakeRunner("n*:80\n", "", "")).Sockets(context.Background()); err == nil {
		t.Fatal("Sockets returned nil error for malformed lsof output")
	}
}
