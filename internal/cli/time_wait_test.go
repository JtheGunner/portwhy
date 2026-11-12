package cli

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"github.com/JtheGunner/portwhy/internal/socket"
)

func timeWaitSocket(port uint16) socket.Socket {
	return socket.Socket{Port: port, Protocol: socket.TCP, Addr: netip.MustParseAddr("127.0.0.1"), State: socket.TimeWait}
}

func TestRun_TimeWaitOnlyExit1Text(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{timeWaitSocket(3000)}})

	got := Run(context.Background(), d, []string{"3000"}, Flags{})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	out := strings.ToLower(stdout.String())
	if !strings.Contains(out, "free") || !strings.Contains(out, "time_wait") {
		t.Errorf("stdout = %q, want free with a TIME_WAIT hint", stdout.String())
	}
}

func TestRun_TimeWaitOnlyExit1JSON(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{timeWaitSocket(3000)}})

	got := Run(context.Background(), d, []string{"3000"}, Flags{JSON: true})

	if got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	out := stdout.String()
	for _, want := range []string{`"version": 1`, `"state": "free"`, `"timeWaitOnly": true`, `"advice"`, "TIME_WAIT"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want it to contain %q", out, want)
		}
	}
}

func TestRun_TimeWaitPlusListenerExit0(t *testing.T) {
	d, stdout, _ := testDeps(fakeSockets{sockets: []socket.Socket{timeWaitSocket(3000), listener(3000, 42)}})

	got := Run(context.Background(), d, []string{"3000"}, Flags{JSON: true})

	if got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if strings.Contains(stdout.String(), `"timeWaitOnly"`) {
		t.Errorf("stdout = %q, want no timeWaitOnly for a port with a listener", stdout.String())
	}
}
