// Package report assembles sockets, processes, explanations and advice into
// per-port reports.
package report

import (
	"context"
	"fmt"
	"slices"

	"github.com/JtheGunner/portwhy/internal/advise"
	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/proc"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// Finding is one listening socket together with its explanation. Process is
// nil when the owner is not visible, vanished or could not be read.
type Finding struct {
	Socket      socket.Socket
	Process     *proc.Process
	Explanation detect.Explanation
	Advice      []advise.Advice
}

// PortReport is the result for one port. A port without findings is free;
// TimeWaitOnly is set when only lingering time_wait sockets exist.
type PortReport struct {
	Port         uint16
	Findings     []Finding
	TimeWaitOnly bool
}

// Service builds port reports. A nil Detector falls back to detect.Generic.
type Service struct {
	Sockets  socket.Collector
	Procs    proc.Reader
	Detector detect.Detector
}

// Explain reports on the given ports in ascending order. A nil ports slice
// means every port with a listening socket.
func (s Service) Explain(ctx context.Context, ports []uint16) ([]PortReport, error) {
	collected, err := s.Sockets.Sockets(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect sockets: %w", err)
	}
	all := socket.Dedupe(collected)

	if ports == nil {
		for _, sk := range all {
			if sk.State == socket.Listen {
				ports = append(ports, sk.Port)
			}
		}
	}
	ports = slices.Clone(ports)
	slices.Sort(ports)
	ports = slices.Compact(ports)

	detector := s.Detector
	if detector == nil {
		detector = detect.Generic()
	}

	reports := make([]PortReport, 0, len(ports))
	for _, port := range ports {
		r := PortReport{Port: port}
		sawTimeWait := false
		for _, sk := range all {
			if sk.Port != port {
				continue
			}
			switch sk.State {
			case socket.TimeWait:
				sawTimeWait = true
			case socket.Listen:
				f, err := s.finding(ctx, detector, sk, all)
				if err != nil {
					return nil, err
				}
				r.Findings = append(r.Findings, f)
			}
		}
		r.TimeWaitOnly = len(r.Findings) == 0 && sawTimeWait
		reports = append(reports, r)
	}
	return reports, nil
}

func (s Service) finding(ctx context.Context, d detect.Detector, sk socket.Socket, all []socket.Socket) (Finding, error) {
	p, err := s.readProcess(ctx, sk.PID)
	if err != nil {
		return Finding{}, err
	}

	f := Finding{Socket: sk, Process: p}
	if e, ok := d.Detect(ctx, sk, p); ok && e != nil {
		f.Explanation = *e
	}
	f.Advice = advise.Advise(advise.Input{
		Socket:  sk,
		Process: p,
		Source:  f.Explanation.Source,
		All:     all,
	})
	return f, nil
}

// readProcess returns nil for a process that cannot be inspected. PID 0 means
// the owner is not visible, so nothing is read. A vanished process
// (proc.ErrNotFound), a permission problem (proc.ErrPermission) and any other
// reader error all degrade to nil, because a report that
// merely lacks process details is more useful than none and the socket itself
// was collected successfully. Only context cancellation aborts, since every
// further read would fail the same way.
func (s Service) readProcess(ctx context.Context, pid int) (*proc.Process, error) {
	if pid == 0 {
		return nil, nil
	}
	p, err := s.Procs.Read(ctx, pid)
	if err == nil {
		return &p, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	return nil, nil
}
