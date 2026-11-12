package advise

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

func listen(port uint16, addr string, pid int) socket.Socket {
	return socket.Socket{Port: port, Protocol: socket.TCP, Addr: netip.MustParseAddr(addr), State: socket.Listen, PID: pid}
}

func find(advice []Advice, substr string) *Advice {
	for i := range advice {
		if strings.Contains(advice[i].Text, substr) {
			return &advice[i]
		}
	}
	return nil
}

func TestAirPlay_OnlyDarwinPort5000WithControlCenter(t *testing.T) {
	cc := &proc.Process{PID: 400, Name: "ControlCenter"}
	other := &proc.Process{PID: 401, Name: "node"}
	tests := []struct {
		name string
		in   Input
		want bool
	}{
		{"darwin 5000", Input{GOOS: "darwin", Socket: listen(5000, "127.0.0.1", 400), Process: cc}, true},
		{"darwin 7000", Input{GOOS: "darwin", Socket: listen(7000, "127.0.0.1", 400), Process: cc}, true},
		{"linux 5000", Input{GOOS: "linux", Socket: listen(5000, "127.0.0.1", 400), Process: cc}, false},
		{"unset GOOS", Input{Socket: listen(5000, "127.0.0.1", 400), Process: cc}, false},
		{"darwin other port", Input{GOOS: "darwin", Socket: listen(5001, "127.0.0.1", 400), Process: cc}, false},
		{"darwin other process", Input{GOOS: "darwin", Socket: listen(5000, "127.0.0.1", 401), Process: other}, false},
		{"darwin no process", Input{GOOS: "darwin", Socket: listen(5000, "127.0.0.1", 0)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := find(Advise(tt.in), "AirPlay Receiver")
			if (a != nil) != tt.want {
				t.Fatalf("AirPlay advice present = %v, want %v", a != nil, tt.want)
			}
			if a != nil && a.Level != Warn {
				t.Errorf("level = %v, want Warn", a.Level)
			}
		})
	}
}

func TestExposedBind_Wildcard(t *testing.T) {
	for _, addr := range []string{"0.0.0.0", "::"} {
		got := Advise(Input{Socket: listen(3000, addr, 10), Process: &proc.Process{PID: 10}})
		a := find(got, "Reachable from the network")
		if a == nil || a.Level != Warn {
			t.Errorf("%s: advice = %+v, want Warn exposed-bind", addr, got)
		}
	}
}

func TestExposedBind_LoopbackNoAdvice(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "::1"} {
		got := Advise(Input{Socket: listen(3000, addr, 10), Process: &proc.Process{PID: 10}})
		if len(got) != 0 {
			t.Errorf("%s: advice = %+v, want none", addr, got)
		}
	}
}

func TestTimeWaitOnly(t *testing.T) {
	sk := socket.Socket{Port: 3000, Protocol: socket.TCP, Addr: netip.MustParseAddr("127.0.0.1"), State: socket.TimeWait}
	got := Advise(Input{Socket: sk})
	if len(got) != 1 || got[0].Level != Info || !strings.Contains(got[0].Text, "TIME_WAIT") {
		t.Fatalf("advice = %+v, want one Info TIME_WAIT hint", got)
	}
}

func TestListeningSocketGetsNoTimeWaitAdvice(t *testing.T) {
	got := Advise(Input{Socket: listen(3000, "127.0.0.1", 10), Process: &proc.Process{PID: 10}})
	if a := find(got, "TIME_WAIT"); a != nil {
		t.Errorf("unexpected %+v", a)
	}
}

func TestOwnerNotVisible(t *testing.T) {
	want := "Owner not visible. Run with sudo for details."
	for name, in := range map[string]Input{
		"pid 0":       {Socket: listen(3000, "127.0.0.1", 0), Process: &proc.Process{PID: 1}},
		"nil process": {Socket: listen(3000, "127.0.0.1", 55)},
	} {
		got := Advise(in)
		if len(got) != 1 || got[0].Level != Info || got[0].Text != want {
			t.Errorf("%s: advice = %+v, want %q", name, got, want)
		}
	}
}
