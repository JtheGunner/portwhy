package render

import (
	"cmp"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/JtheGunner/portwhy/internal/detect"
	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/socket"
)

// minRunLength is the number of consecutive ports that are written as a range.
const minRunLength = 3

// overview is the list-mode view of a set of reports: busy sockets grouped by
// the service or process that owns them.
type overview struct {
	Groups []group
	// Hidden are the findings whose owner is not visible.
	Hidden []report.Finding
}

// group is everything one service (or one plain process name) holds.
type group struct {
	Source detect.Source
	Title  string
	Rows   []row
}

// row is one line of a group: ports that share protocol, address and the set
// of owning PIDs.
type row struct {
	Protocol socket.Protocol
	Addr     netip.Addr
	Ports    []uint16
	PIDs     []int
	// Name is the process name of the lowest PID.
	Name string
	// Exposed is true when the address accepts connections from the network.
	Exposed bool
}

type groupKey struct {
	source detect.Source
	title  string
}

// buildOverview groups the findings of reports. It performs no I/O and its
// result does not depend on the input order.
func buildOverview(reports []report.PortReport) overview {
	type socketKey struct {
		group groupKey
		port  uint16
		proto socket.Protocol
		addr  netip.Addr
	}
	owners := map[socketKey]map[int]string{}
	var ov overview
	for _, r := range reports {
		for _, f := range r.Findings {
			if f.Process == nil {
				ov.Hidden = append(ov.Hidden, f)
				continue
			}
			k := socketKey{groupKey{f.Explanation.Source, f.Explanation.Title}, f.Socket.Port, f.Socket.Protocol, f.Socket.Addr}
			if owners[k] == nil {
				owners[k] = map[int]string{}
			}
			owners[k][f.Process.PID] = f.Process.Name
		}
	}

	type rowKey struct {
		group groupKey
		proto socket.Protocol
		addr  netip.Addr
		pids  string
	}
	rows := map[rowKey]*row{}
	for k, byPID := range owners {
		pids := slices.Sorted(func(yield func(int) bool) {
			for pid := range byPID {
				if !yield(pid) {
					return
				}
			}
		})
		rk := rowKey{k.group, k.proto, k.addr, joinInts(pids)}
		r, ok := rows[rk]
		if !ok {
			r = &row{Protocol: k.proto, Addr: k.addr, PIDs: pids, Name: byPID[pids[0]], Exposed: k.addr.IsUnspecified()}
			rows[rk] = r
		}
		r.Ports = append(r.Ports, k.port)
	}

	byGroup := map[groupKey]*group{}
	for rk, r := range rows {
		slices.Sort(r.Ports)
		g, ok := byGroup[rk.group]
		if !ok {
			g = &group{Source: rk.group.source, Title: rk.group.title}
			byGroup[rk.group] = g
		}
		g.Rows = append(g.Rows, *r)
	}
	for _, g := range byGroup {
		slices.SortFunc(g.Rows, compareRows)
		ov.Groups = append(ov.Groups, *g)
	}
	slices.SortFunc(ov.Groups, func(a, b group) int {
		return cmp.Or(cmp.Compare(a.Rows[0].Ports[0], b.Rows[0].Ports[0]), cmp.Compare(a.Title, b.Title), cmp.Compare(a.Source, b.Source))
	})
	slices.SortFunc(ov.Hidden, func(a, b report.Finding) int {
		return cmp.Or(cmp.Compare(a.Socket.Port, b.Socket.Port), cmp.Compare(a.Socket.Protocol, b.Socket.Protocol), a.Socket.Addr.Compare(b.Socket.Addr))
	})
	return ov
}

func compareRows(a, b row) int {
	return cmp.Or(
		cmp.Compare(a.Ports[0], b.Ports[0]),
		cmp.Compare(a.Protocol, b.Protocol),
		a.Addr.Compare(b.Addr),
		slices.Compare(a.PIDs, b.PIDs),
	)
}

func joinInts(ints []int) string {
	parts := make([]string, len(ints))
	for i, n := range ints {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// formatPorts writes ascending ports comma-separated, with runs of at least
// minRunLength consecutive ports as "first-last".
func formatPorts(ports []uint16) string {
	var parts []string
	for i := 0; i < len(ports); {
		j := i
		for j+1 < len(ports) && ports[j+1] == ports[j]+1 {
			j++
		}
		if j-i+1 >= minRunLength {
			parts = append(parts, strconv.Itoa(int(ports[i]))+"-"+strconv.Itoa(int(ports[j])))
		} else {
			for _, p := range ports[i : j+1] {
				parts = append(parts, strconv.Itoa(int(p)))
			}
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}
