package systemd

import "testing"

func TestUnitFromCgroup(t *testing.T) {
	tests := []struct {
		name string
		cg   string
		want cgroupUnit
		ok   bool
	}{
		{"system service", "/system.slice/postgresql.service", cgroupUnit{Name: "postgresql.service"}, true},
		{"template unit", "/system.slice/system-getty.slice/getty@tty1.service", cgroupUnit{Name: "getty@tty1.service"}, true},
		{"service with sub cgroup", "/system.slice/nginx.service/worker", cgroupUnit{Name: "nginx.service"}, true},
		{"user service", "/user.slice/user-1000.slice/user@1000.service/app.slice/foo.service", cgroupUnit{Name: "foo.service", User: true, UID: 1000}, true},
		{"user service nested in app slice", "/user.slice/user-1000.slice/user@1000.service/app.slice/app-x.slice/bar.service/sub", cgroupUnit{Name: "bar.service", User: true, UID: 1000}, true},
		{"escaped name", `/system.slice/foo\x2dbar.service`, cgroupUnit{Name: `foo\x2dbar.service`}, true},
		{"service inside container scope", "/system.slice/docker-abc123.scope/sshd.service", cgroupUnit{}, false},
		{"service deeper inside container scope", "/system.slice/docker-abc123.scope/sub/sshd.service", cgroupUnit{}, false},
		{"service below a plain directory", "/foo/system.slice/sshd.service", cgroupUnit{}, false},
		{"user manager spoofed inside session scope", "/user.slice/user-1000.slice/session-3.scope/user@1000.service/app.slice/foo.service", cgroupUnit{}, false},
		{"scope below user manager", "/user.slice/user-1000.slice/user@1000.service/app.slice/foo.scope/bar.service", cgroupUnit{}, false},
		{"first service wins over a spoofed inner one", "/system.slice/evil.service/x.slice/sshd.service", cgroupUnit{Name: "evil.service"}, true},
		{"service directly below the root", "/foo.service", cgroupUnit{Name: "foo.service"}, true},
		{"session scope", "/user.slice/user-1000.slice/session-3.scope", cgroupUnit{}, false},
		{"docker scope", "/system.slice/docker-abc123.scope", cgroupUnit{}, false},
		{"user manager init scope", "/user.slice/user-1000.slice/user@1000.service/init.scope", cgroupUnit{}, false},
		{"user manager without inner service", "/user.slice/user-1000.slice/user@1000.service", cgroupUnit{}, false},
		{"root cgroup", "/", cgroupUnit{}, false},
		{"empty", "", cgroupUnit{}, false},
		{"option-like name", "/system.slice/--now.service", cgroupUnit{}, false},
		{"name with space", "/system.slice/a b.service", cgroupUnit{}, false},
		{"name with control character", "/system.slice/a\x1b[2J.service", cgroupUnit{}, false},
		{"bare suffix", "/system.slice/.service", cgroupUnit{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := unitFromCgroup(tt.cg)
			if ok != tt.ok || got != tt.want {
				t.Errorf("unitFromCgroup(%q) = %+v, %v, want %+v, %v", tt.cg, got, ok, tt.want, tt.ok)
			}
		})
	}
}
