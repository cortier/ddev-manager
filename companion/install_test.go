package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedLinuxService(t *testing.T) {
	l := layoutFor(t.TempDir(), "linux")
	e := install(l, "missing", Config{}, func(string, ...string) error { return errors.New("no user bus") })
	if e == nil || !strings.Contains(e.Error(), "systemd") {
		t.Fatal(e)
	}
	if _, e = os.Stat(l.Data); !os.IsNotExist(e) {
		t.Fatal("Installer wrote files before preflight")
	}
}
func TestInstallerLayouts(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			l := layoutFor(t.TempDir(), platform)
			bin := t.TempDir()
			for _, name := range []string{"ddev", "git", "source"} {
				os.WriteFile(filepath.Join(bin, name), []byte("fixture"), 0700)
			}
			calls := []string{}
			run := func(s string, a ...string) error { calls = append(calls, s+" "+strings.Join(a, " ")); return nil }
			c := Config{DdevPath: filepath.Join(bin, "ddev"), GitPath: filepath.Join(bin, "git")}
			if e := install(l, filepath.Join(bin, "source"), c, run); e != nil {
				t.Fatal(e)
			}
			if b, e := os.ReadFile(l.Native); e != nil || !bytes.Contains(b, []byte(extensionID)) {
				t.Fatal(e)
			}
			if platform == "darwin" && !strings.Contains(startupContent(l), "Launch") {
				if !strings.Contains(strings.Join(calls, "\n"), "launchctl bootstrap") {
					t.Fatal(calls)
				}
			}
			saved, _ := os.ReadFile(filepath.Join(l.Data, "ipc.secret"))
			if e := install(l, filepath.Join(bin, "source"), c, run); e != nil {
				t.Fatal(e)
			}
			again, _ := os.ReadFile(filepath.Join(l.Data, "ipc.secret"))
			if !bytes.Equal(saved, again) {
				t.Fatal("Secret changed on upgrade")
			}
			if e := uninstall(l, run); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestUninstallRejectsForeignManifest(t *testing.T) {
	l := layoutFor(t.TempDir(), "linux")
	writeJSON(l.Native, map[string]string{"path": "/other/program"})
	if e := uninstall(l, func(string, ...string) error { return nil }); e == nil {
		t.Fatal("Foreign manifest deleted")
	}
}
func TestFrames(t *testing.T) {
	var b bytes.Buffer
	if e := writeFrame(&b, map[string]string{"id": "test"}); e != nil {
		t.Fatal(e)
	}
	data, e := readFrame(&b)
	if e != nil || !bytes.Contains(data, []byte("test")) {
		t.Fatal(string(data), e)
	}
	var h [4]byte
	binary.NativeEndian.PutUint32(h[:], 2<<20)
	if _, e = readFrame(bytes.NewReader(h[:])); e == nil {
		t.Fatal("Oversized frame accepted")
	}
	if _, e = readFrame(bytes.NewReader([]byte{1, 0})); e == nil {
		t.Fatal("Truncated frame accepted")
	}
}
func TestProtocolRejectsUnknownVersionAndMethod(t *testing.T) {
	l := layoutFor(t.TempDir(), "linux")
	if _, e := dispatch(l, Request{Version: 2}); e == nil {
		t.Fatal("Version accepted")
	}
	writeJSON(filepath.Join(l.Data, "config.json"), Config{})
	if _, e := dispatch(l, Request{Version: 1, Method: "shell"}); e == nil {
		t.Fatal("Unknown method accepted")
	}
}

func TestInterruptedCleanupCanRetry(t *testing.T) {
	l := layoutFor(t.TempDir(), "linux")
	os.MkdirAll(l.Data, 0700)
	os.WriteFile(l.Binary, []byte("owned"), 0700)
	run := func(_ string, args ...string) error {
		if len(args) > 1 && args[1] == "disable" {
			return errors.New("failed to stop")
		}
		return nil
	}
	if e := uninstall(l, run); e == nil {
		t.Fatal("Reported success while service remained active")
	}
	if _, e := os.Stat(l.Binary); e != nil {
		t.Fatal("Removed binary before service stopped")
	}
	if e := uninstall(l, func(string, ...string) error { return nil }); e != nil {
		t.Fatal(e)
	}
}
func TestForeignStartupIsNotStopped(t *testing.T) {
	l := layoutFor(t.TempDir(), "linux")
	writePrivate(l.Startup, []byte("ExecStart=/other/program"))
	called := false
	if e := uninstall(l, func(string, ...string) error { called = true; return nil }); e == nil || called {
		t.Fatal("Touched foreign startup registration")
	}
}
