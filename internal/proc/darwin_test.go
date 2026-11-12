//go:build darwin

package proc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JtheGunner/portwhy/internal/execx"
)

func TestDarwinReader_ReadsOwnProcess(t *testing.T) {
	r := NewDarwinReader(execx.NewRunner(5 * time.Second))

	p, err := r.Read(context.Background(), os.Getpid())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if p.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", p.PID, os.Getpid())
	}
	if p.PPID != os.Getppid() {
		t.Errorf("PPID = %d, want %d", p.PPID, os.Getppid())
	}
	if p.Name == "" {
		t.Error("Name is empty")
	}
	if age := time.Since(p.Started); age < 0 || age > time.Hour {
		t.Errorf("Started = %v, want a recent time", p.Started)
	}
}
