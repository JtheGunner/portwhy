package report

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"testing"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

type fakeCollector struct {
	sockets []socket.Socket
	err     error
}

func (f fakeCollector) Sockets(context.Context) ([]socket.Socket, error) {
	return f.sockets, f.err
}

type fakeReader struct {
	procs map[int]proc.Process
	err   error
	reads []int
}

func (f *fakeReader) Read(_ context.Context, pid int) (proc.Process, error) {
	f.reads = append(f.reads, pid)
	if f.err != nil {
		return proc.Process{}, f.err
	}
	p, ok := f.procs[pid]
	if !ok {
		return proc.Process{}, proc.ErrNotFound
	}
	return p, nil
}

func sock(port uint16, state socket.State, addr string, pid int) socket.Socket {
	return socket.Socket{Port: port, Protocol: socket.TCP, Addr: netip.MustParseAddr(addr), State: state, PID: pid}
}

func service(sockets []socket.Socket, r *fakeReader) Service {
	return Service{Sockets: fakeCollector{sockets: sockets}, Procs: r, Detector: detect.Generic()}
}

func TestService_FreePortHasNoFindings(t *testing.T) {
	svc := service([]socket.Socket{sock(8080, socket.Listen, "0.0.0.0", 1)}, &fakeReader{})

	got, err := svc.Explain(context.Background(), []uint16{3000})

	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Port != 3000 || len(got[0].Findings) != 0 || got[0].TimeWaitOnly {
		t.Errorf("Explain() = %+v, want free port 3000", got)
	}
}

func TestService_AllListeningWhenPortsNil(t *testing.T) {
	r := &fakeReader{procs: map[int]proc.Process{
		1: {PID: 1, Name: "a"},
		2: {PID: 2, Name: "b"},
	}}
	svc := service([]socket.Socket{
		sock(9000, socket.Listen, "0.0.0.0", 2),
		sock(80, socket.Listen, "0.0.0.0", 1),
		sock(5000, socket.TimeWait, "127.0.0.1", 0),
	}, r)

	got, err := svc.Explain(context.Background(), nil)

	if err != nil {
		t.Fatal(err)
	}
	var ports []uint16
	for _, pr := range got {
		ports = append(ports, pr.Port)
	}
	if !slices.Equal(ports, []uint16{80, 9000}) {
		t.Fatalf("ports = %v, want [80 9000]", ports)
	}
	if f := got[0].Findings[0]; f.Process == nil || f.Process.Name != "a" || f.Explanation.Title != "a" {
		t.Errorf("finding = %+v", f)
	}
}

func TestService_ProcessVanishedDegradesToNotVisible(t *testing.T) {
	svc := service([]socket.Socket{sock(3000, socket.Listen, "0.0.0.0", 99)}, &fakeReader{err: proc.ErrNotFound})

	got, err := svc.Explain(context.Background(), []uint16{3000})

	if err != nil {
		t.Fatal(err)
	}
	f := got[0].Findings[0]
	if f.Process != nil {
		t.Errorf("Process = %+v, want nil", f.Process)
	}
	if f.Explanation.Title != "owner not visible" {
		t.Errorf("Title = %q", f.Explanation.Title)
	}
}

func TestService_PermissionDeniedDegrades(t *testing.T) {
	svc := service([]socket.Socket{sock(22, socket.Listen, "0.0.0.0", 1)}, &fakeReader{err: fmt.Errorf("read 1: %w", proc.ErrPermission)})

	got, err := svc.Explain(context.Background(), []uint16{22})

	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Findings) != 1 || got[0].Findings[0].Process != nil {
		t.Errorf("Explain() = %+v, want one finding without process", got)
	}
}

func TestService_UnexpectedReaderErrorDegrades(t *testing.T) {
	svc := service([]socket.Socket{sock(22, socket.Listen, "0.0.0.0", 1)}, &fakeReader{err: errors.New("boom")})

	got, err := svc.Explain(context.Background(), []uint16{22})

	if err != nil {
		t.Fatalf("Explain() error = %v, want degrade", err)
	}
	if got[0].Findings[0].Process != nil {
		t.Error("Process != nil, want nil")
	}
}

func TestService_CancelledContextAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := service([]socket.Socket{sock(22, socket.Listen, "0.0.0.0", 1)}, &fakeReader{err: ctx.Err()})

	if _, err := svc.Explain(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Explain() error = %v, want context.Canceled", err)
	}
}

func TestService_PIDZeroSkipsProcessRead(t *testing.T) {
	r := &fakeReader{}
	svc := service([]socket.Socket{sock(443, socket.Listen, "0.0.0.0", 0)}, r)

	got, err := svc.Explain(context.Background(), []uint16{443})

	if err != nil {
		t.Fatal(err)
	}
	if len(r.reads) != 0 {
		t.Errorf("reads = %v, want none", r.reads)
	}
	if len(got[0].Findings) != 1 || got[0].Findings[0].Process != nil {
		t.Errorf("Explain() = %+v", got)
	}
}

func TestService_TimeWaitOnly(t *testing.T) {
	r := &fakeReader{}
	svc := service([]socket.Socket{sock(3000, socket.TimeWait, "127.0.0.1", 0)}, r)

	got, err := svc.Explain(context.Background(), []uint16{3000})

	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Findings) != 0 || !got[0].TimeWaitOnly {
		t.Errorf("Explain() = %+v, want TimeWaitOnly without findings", got)
	}
	if len(r.reads) != 0 {
		t.Errorf("reads = %v, want none", r.reads)
	}
}

func TestService_ListenBeatsTimeWait(t *testing.T) {
	svc := service([]socket.Socket{
		sock(3000, socket.Listen, "0.0.0.0", 0),
		sock(3000, socket.TimeWait, "127.0.0.1", 0),
	}, &fakeReader{})

	got, _ := svc.Explain(context.Background(), []uint16{3000})

	if len(got[0].Findings) != 1 || got[0].TimeWaitOnly {
		t.Errorf("Explain() = %+v", got)
	}
}

func TestService_CollectorErrorIsReturned(t *testing.T) {
	boom := errors.New("boom")
	svc := Service{Sockets: fakeCollector{err: boom}, Procs: &fakeReader{}}

	if _, err := svc.Explain(context.Background(), nil); !errors.Is(err, boom) {
		t.Errorf("Explain() error = %v, want %v", err, boom)
	}
}

func TestService_PortsReportedAscendingWithoutDuplicates(t *testing.T) {
	svc := service(nil, &fakeReader{})

	got, _ := svc.Explain(context.Background(), []uint16{9000, 80, 9000})

	if len(got) != 2 || got[0].Port != 80 || got[1].Port != 9000 {
		t.Errorf("Explain() = %+v, want ports 80 and 9000", got)
	}
}

func TestService_SeveralSocketsForOnePortYieldSeveralFindings(t *testing.T) {
	r := &fakeReader{procs: map[int]proc.Process{1: {Name: "a"}, 2: {Name: "b"}}}
	svc := service([]socket.Socket{
		sock(8000, socket.Listen, "127.0.0.1", 1),
		sock(8000, socket.Listen, "10.0.0.5", 2),
	}, r)

	got, _ := svc.Explain(context.Background(), []uint16{8000})

	if len(got[0].Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(got[0].Findings))
	}
}

func TestService_NilDetectorFallsBackToGeneric(t *testing.T) {
	svc := Service{
		Sockets: fakeCollector{sockets: []socket.Socket{sock(1, socket.Listen, "0.0.0.0", 7)}},
		Procs:   &fakeReader{procs: map[int]proc.Process{7: {Name: "x"}}},
	}

	got, _ := svc.Explain(context.Background(), nil)

	if got[0].Findings[0].Explanation.Title != "x" {
		t.Errorf("Title = %q, want x", got[0].Findings[0].Explanation.Title)
	}
}
