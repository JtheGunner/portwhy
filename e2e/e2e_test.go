//go:build e2e

// Package e2e runs the built portwhy binary against real listeners.
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const listenEnv = "PORTWHY_E2E_LISTEN"

var binary string

// TestMain doubles as the listener helper: when listenEnv is set, the test
// binary acts as a TCP listener instead of running tests.
func TestMain(m *testing.M) {
	if os.Getenv(listenEnv) != "" {
		runListener()
		return
	}
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "portwhy-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer os.RemoveAll(dir)

	binary = filepath.Join(dir, "portwhy")
	build := exec.Command("go", "build", "-o", binary, "../cmd/portwhy")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build portwhy:", err)
		return 2
	}
	return m.Run()
}

// runListener listens on an OS-chosen loopback port, prints the port and
// blocks until it is killed.
func runListener() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(l.Addr().(*net.TCPAddr).Port)
	for {
		c, err := l.Accept()
		if err != nil {
			os.Exit(2)
		}
		c.Close()
	}
}

// startListener re-executes the test binary as a TCP listener and returns its
// port and PID. The child is always killed on cleanup.
func startListener(t *testing.T) (port int, pid int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), listenEnv+"=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Reap the child as soon as it exits: an unreaped zombie would still
	// look alive to portwhy's post-kill check.
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-reaped
	})
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("read listener port: %v", err)
	}
	port, err = strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parse listener port %q: %v", line, err)
	}
	return port, cmd.Process.Pid
}

// freePort returns a port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// portwhy runs the binary and returns stdout, stderr and the exit code.
func portwhy(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		code = ee.ExitCode()
	default:
		t.Fatalf("run portwhy: %v", err)
	}
	return o.String(), e.String(), code
}

type jsonOutput struct {
	Version int `json:"version"`
	Ports   []struct {
		Port     int `json:"port"`
		Findings []struct {
			PID int `json:"pid"`
		} `json:"findings"`
	} `json:"ports"`
}

func explainJSON(t *testing.T, port int) (jsonOutput, int) {
	t.Helper()
	stdout, stderr, code := portwhy(t, strconv.Itoa(port), "--json")
	var out jsonOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode JSON: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return out, code
}

func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func canListen(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func TestE2E_ExplainsRealListener(t *testing.T) {
	port, pid := startListener(t)
	out, code := explainJSON(t, port)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if len(out.Ports) != 1 || len(out.Ports[0].Findings) == 0 {
		t.Fatalf("no findings for port %d: %+v", port, out)
	}
	if got := out.Ports[0].Findings[0].PID; got != pid {
		t.Errorf("findings[0].pid = %d, want %d", got, pid)
	}
}

func TestE2E_KillRealListener(t *testing.T) {
	port, _ := startListener(t)
	_, stderr, code := portwhy(t, strconv.Itoa(port), "-k", "-y")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, stderr)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if canListen(port) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("port %d is still busy after kill", port)
}

func TestE2E_FreePortExit1(t *testing.T) {
	_, _, code := portwhy(t, strconv.Itoa(freePort(t)))
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestE2E_DryRunLeavesProcessAlive(t *testing.T) {
	port, pid := startListener(t)
	_, stderr, code := portwhy(t, strconv.Itoa(port), "-k", "-y", "--dry-run")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, stderr)
	}
	if !alive(pid) {
		t.Fatalf("process %d was stopped by --dry-run", pid)
	}
	if canListen(port) {
		t.Errorf("port %d became free during --dry-run", port)
	}
}

func TestE2E_JSONHasVersion1(t *testing.T) {
	port, _ := startListener(t)
	out, _ := explainJSON(t, port)
	if out.Version != 1 {
		t.Errorf("version = %d, want 1", out.Version)
	}
}

func TestE2E_CompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			stdout, stderr, code := portwhy(t, "completion", shell)
			if code != 0 || stdout == "" {
				t.Fatalf("exit code = %d, stdout empty = %v\nstderr: %s", code, stdout == "", stderr)
			}
		})
	}
}
