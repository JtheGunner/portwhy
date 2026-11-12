package render

import (
	"cmp"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/JtheGunner/portwhy/internal/report"
	"github.com/JtheGunner/portwhy/internal/textutil"
)

// plainEmpty stands in for a column without a value, such as the name of a
// process that is not visible, so every line keeps the same number of columns.
const plainEmpty = "-"

// plainRow is one line of the plain output.
type plainRow struct {
	port   uint16
	proto  string
	addr   string
	pid    int
	source string
	name   string
	ip     netip.Addr
}

// Plain writes one tab-separated line per listening socket: port, protocol,
// address, PID, source, name. There is no header, grouping, colour or
// shortening. Sockets are ordered by port, protocol, address and PID. A socket whose
// owner is not visible gets its own line with PID 0 and the name "-". The
// column order is a public interface.
func Plain(w io.Writer, reports []report.PortReport) error {
	var rows []plainRow
	for _, r := range reports {
		for _, f := range r.Findings {
			row := plainRow{
				port:   f.Socket.Port,
				proto:  plainField(string(f.Socket.Protocol)),
				addr:   f.Socket.Addr.String(),
				pid:    f.Socket.PID,
				source: plainField(string(f.Explanation.Source)),
				name:   plainEmpty,
				ip:     f.Socket.Addr,
			}
			if f.Process != nil {
				row.name = plainField(f.Process.Name)
			}
			rows = append(rows, row)
		}
	}
	slices.SortStableFunc(rows, func(a, b plainRow) int {
		return cmp.Or(
			cmp.Compare(a.port, b.port),
			cmp.Compare(a.proto, b.proto),
			a.ip.Compare(b.ip),
			cmp.Compare(a.pid, b.pid),
		)
	})

	var b strings.Builder
	for _, row := range rows {
		b.WriteString(strings.Join([]string{
			strconv.Itoa(int(row.port)), row.proto, row.addr, strconv.Itoa(row.pid), row.source, row.name,
		}, "\t"))
		b.WriteByte('\n')
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write plain report: %w", err)
	}
	return nil
}

// plainField makes a value safe as a single tab-separated column: tabs and
// line breaks become spaces, other control characters (such as ESC) become '?',
// and an empty value becomes "-".
func plainField(s string) string {
	s = textutil.Clean(strings.Join(strings.Fields(s), " "))
	if s == "" {
		return plainEmpty
	}
	return s
}
