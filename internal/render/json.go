package render

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/JtheGunner/portwhy/internal/advise"
	"github.com/JtheGunner/portwhy/internal/report"
)

// schemaVersion is bumped whenever the JSON shape changes.
const schemaVersion = 1

type jsonDocument struct {
	Version int        `json:"version"`
	Ports   []jsonPort `json:"ports"`
}

type jsonPort struct {
	Port         uint16        `json:"port"`
	State        string        `json:"state"`
	TimeWaitOnly bool          `json:"timeWaitOnly,omitempty"`
	Findings     []jsonFinding `json:"findings"`
}

type jsonFinding struct {
	Protocol  string         `json:"protocol"`
	Address   string         `json:"address"`
	PID       int            `json:"pid"`
	Process   *jsonProcess   `json:"process"`
	Source    string         `json:"source"`
	Title     string         `json:"title"`
	Details   []jsonDetail   `json:"details"`
	Hints     []string       `json:"hints"`
	Advice    []jsonAdvice   `json:"advice"`
	Terminate *jsonTerminate `json:"terminate,omitempty"`
}

type jsonProcess struct {
	PID       int            `json:"pid"`
	PPID      int            `json:"ppid"`
	Name      string         `json:"name"`
	Cmdline   []string       `json:"cmdline"`
	User      string         `json:"user"`
	Started   string         `json:"started,omitempty"`
	Cwd       string         `json:"cwd,omitempty"`
	Ancestors []jsonAncestor `json:"ancestors"`
}

type jsonAncestor struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

type jsonDetail struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type jsonAdvice struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type jsonTerminate struct {
	Command string `json:"command"`
}

// JSON writes the reports as one indented JSON document.
func JSON(w io.Writer, reports []report.PortReport) error {
	doc := jsonDocument{Version: schemaVersion, Ports: make([]jsonPort, 0, len(reports))}
	for _, r := range reports {
		doc.Ports = append(doc.Ports, toJSONPort(r))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("write json report: %w", err)
	}
	return nil
}

func toJSONPort(r report.PortReport) jsonPort {
	p := jsonPort{Port: r.Port, State: "free", TimeWaitOnly: r.TimeWaitOnly, Findings: make([]jsonFinding, 0, len(r.Findings))}
	if len(r.Findings) > 0 {
		p.State = "busy"
	}
	for _, f := range r.Findings {
		p.Findings = append(p.Findings, toJSONFinding(f))
	}
	return p
}

func toJSONFinding(f report.Finding) jsonFinding {
	out := jsonFinding{
		Protocol: string(f.Socket.Protocol),
		Address:  f.Socket.Addr.String(),
		PID:      f.Socket.PID,
		Source:   string(f.Explanation.Source),
		Title:    f.Explanation.Title,
		Details:  make([]jsonDetail, 0, len(f.Explanation.Details)),
		Hints:    nonNil(f.Explanation.Hints),
		Advice:   make([]jsonAdvice, 0, len(f.Advice)),
	}
	for _, d := range f.Explanation.Details {
		out.Details = append(out.Details, jsonDetail{Key: d.Key, Value: d.Value})
	}
	for _, a := range f.Advice {
		level := "info"
		if a.Level == advise.Warn {
			level = "warn"
		}
		out.Advice = append(out.Advice, jsonAdvice{Level: level, Text: a.Text})
	}
	if f.Explanation.Terminator != nil {
		out.Terminate = &jsonTerminate{Command: f.Explanation.Terminator.Describe()}
	}
	if p := f.Process; p != nil {
		jp := &jsonProcess{
			PID: p.PID, PPID: p.PPID, Name: p.Name, Cmdline: nonNil(p.Cmdline), User: p.User,
			Cwd: p.Cwd, Ancestors: make([]jsonAncestor, 0, len(p.Ancestors)),
		}
		if !p.Started.IsZero() {
			jp.Started = p.Started.Format(time.RFC3339)
		}
		for _, a := range p.Ancestors {
			jp.Ancestors = append(jp.Ancestors, jsonAncestor{PID: a.PID, Name: a.Name})
		}
		out.Process = jp
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
