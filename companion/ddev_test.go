package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func envelope(v any) []byte { b, _ := json.Marshal(map[string]any{"raw": v}); return b }
func fixtureManager(t *testing.T) (*Manager, *Project, *[]string) {
	t.Helper()
	root := t.TempDir()
	l := layoutFor(t.TempDir(), "linux")
	os.MkdirAll(l.Data, 0700)
	p := &Project{Name: "example-api", Root: root, Status: "stopped", URL: "https://example.test"}
	calls := &[]string{}
	m := &Manager{config: Config{DdevPath: "ddev", GitPath: "git"}, layout: l}
	m.run = func(_ context.Context, dir, b string, args ...string) ([]byte, error) {
		key := strings.Join(args, " ")
		*calls = append(*calls, b+" "+key)
		switch key {
		case "list --json-output":
			return envelope([]Project{*p}), nil
		case "branch --show-current":
			return []byte("feat/example\n"), nil
		case "rev-parse --path-format=absolute --git-common-dir":
			return []byte(filepath.Join(root, "repo-api", ".git")), nil
		case "start":
			p.Status = "running"
			return nil, nil
		case "restart":
			return nil, nil
		case "stop":
			p.Status = "stopped"
			return nil, nil
		case "describe --json-output":
			return envelope(map[string]any{"services": map[string]any{"web": map[string]any{"https_url": p.URL}, "buggregator": map[string]any{"https_url": "https://example.test:8777"}, "webhook-site": map[string]any{"https_url": "https://example.test:8084"}, "ministack": map[string]any{"https_url": "https://example.test:3900"}, "redis": map[string]any{}}}), nil
		case "utility configyaml --full-yaml --omit-keys=web_environment":
			return []byte("These files loaded\n# Complete processed project configuration:\nweb_extra_exposed_ports:\n  - name: storybook\n    https_port: 6007\n  - name: reverb\n    https_port: 8080\n  - name: vite\n    https_port: 5173\n"), nil
		}
		return nil, errors.New("unexpected command " + key)
	}
	return m, p, calls
}
func TestDiscovery(t *testing.T) {
	m, p, _ := fixtureManager(t)
	got, e := m.discover(context.Background())
	if e != nil || len(got) != 1 || got[0].Branch != "feat/example" || got[0].Repository != "repo-api" {
		t.Fatalf("%+v %v", got, e)
	}
	p.Root = "/missing-ddev-fixture"
	got, e = m.discover(context.Background())
	if e != nil || got[0].Warning == "" {
		t.Fatalf("Missing directory not retained: %+v %v", got, e)
	}
}
func TestStartBeforeOpen(t *testing.T) {
	m, p, calls := fixtureManager(t)
	v, e := m.action(context.Background(), p.Name, "open", "")
	if e != nil || v.(map[string]string)["url"] != p.URL {
		t.Fatalf("%v %v", v, e)
	}
	if strings.Count(strings.Join(*calls, "\n"), "ddev start") != 1 {
		t.Fatal(*calls)
	}
	_, e = m.action(context.Background(), p.Name, "start", "")
	if e != nil || strings.Count(strings.Join(*calls, "\n"), "ddev start") != 1 {
		t.Fatal("Running start must be a no-op")
	}
}
func TestOpenIDEUsesSettingBeforeEnvironment(t *testing.T) {
	m, p, _ := fixtureManager(t)
	bin := t.TempDir()
	configured := filepath.Join(bin, "configured-ide")
	fallback := filepath.Join(bin, "environment-ide")
	os.WriteFile(configured, []byte("fixture"), 0700)
	os.WriteFile(fallback, []byte("fixture"), 0700)
	t.Setenv("IDE", fallback)
	m.config.IDEPath = configured
	var gotDir, gotBinary string
	var gotArgs []string
	original := m.run
	m.run = func(ctx context.Context, dir, binary string, args ...string) ([]byte, error) {
		if binary == configured || binary == fallback {
			gotDir, gotBinary, gotArgs = dir, binary, args
			return nil, nil
		}
		return original(ctx, dir, binary, args...)
	}
	if _, e := m.action(context.Background(), p.Name, "ide", ""); e != nil {
		t.Fatal(e)
	}
	if gotBinary != configured || gotDir != p.Root || len(gotArgs) != 1 || gotArgs[0] != p.Root {
		t.Fatalf("IDE invocation: dir=%q binary=%q args=%q", gotDir, gotBinary, gotArgs)
	}
	m.config.IDEPath = ""
	if _, e := m.action(context.Background(), p.Name, "ide", ""); e != nil || gotBinary != fallback {
		t.Fatalf("$IDE fallback: binary=%q error=%v", gotBinary, e)
	}
}

func TestOpenIDERequiresConfiguredExecutable(t *testing.T) {
	m, p, _ := fixtureManager(t)
	t.Setenv("IDE", "")
	if _, e := m.action(context.Background(), p.Name, "ide", ""); e == nil {
		t.Fatal("Missing IDE configuration accepted")
	}
}
func TestPausedRestartStarts(t *testing.T) {
	m, p, calls := fixtureManager(t)
	p.Status = "paused"
	_, e := m.action(context.Background(), p.Name, "restart", "")
	if e != nil || !strings.Contains(strings.Join(*calls, "\n"), "ddev start") {
		t.Fatal(e, *calls)
	}
}
func TestFailedStartDoesNotResolveURL(t *testing.T) {
	m, p, _ := fixtureManager(t)
	original := m.run
	m.run = func(c context.Context, d, b string, a ...string) ([]byte, error) {
		if a[0] == "start" {
			return nil, errors.New("startup failed")
		}
		return original(c, d, b, a...)
	}
	if v, e := m.action(context.Background(), p.Name, "open", ""); e == nil || v != nil {
		t.Fatal(v, e)
	}
}
func TestServiceDiscovery(t *testing.T) {
	m, p, _ := fixtureManager(t)
	services, e := m.services(context.Background(), *p)
	if e != nil || len(services) != 3 {
		t.Fatal(services, e)
	}
	if services[1].Name != "Storybook" || services[1].URL != "https://example.test:6007" {
		t.Fatal(services)
	}
	for _, service := range services {
		if service.ID == "ministack" || service.ID == "reverb" || service.ID == "vite" {
			t.Fatalf("Internal service exposed in browser menu: %+v", service)
		}
	}
	_, e = m.action(context.Background(), p.Name, "open", "unknown")
	if e == nil {
		t.Fatal("Unknown service accepted")
	}
}
func TestCustomURLAndFallback(t *testing.T) {
	m, p, _ := fixtureManager(t)
	os.MkdirAll(filepath.Join(p.Root, ".ddev/commands/host"), 0700)
	os.WriteFile(filepath.Join(p.Root, ".ddev/commands/host/url"), []byte("fixture"), 0600)
	original := m.run
	m.run = func(c context.Context, d, b string, a ...string) ([]byte, error) {
		if a[0] == "url" {
			return []byte("https://custom.test\nhttps://second.test"), nil
		}
		return original(c, d, b, a...)
	}
	u, e := m.primary(context.Background(), *p)
	if e != nil || u != "https://custom.test" {
		t.Fatal(u, e)
	}
	m.run = original
	u, e = m.primary(context.Background(), *p)
	if e != nil || u != p.URL {
		t.Fatal(u, e)
	}
}
func TestRejectUnknownProjectAndAction(t *testing.T) {
	m, p, _ := fixtureManager(t)
	if _, e := m.action(context.Background(), "../../other", "start", ""); e == nil {
		t.Fatal("Unknown project accepted")
	}
	if _, e := m.action(context.Background(), p.Name, "exec", ""); e == nil {
		t.Fatal("Arbitrary command accepted")
	}
}
func TestOperationSerialization(t *testing.T) {
	m, _, _ := fixtureManager(t)
	other := &Manager{layout: m.layout}
	var wg sync.WaitGroup
	var mu sync.Mutex
	active, max := 0, 0
	for _, manager := range []*Manager{m, other, m} {
		wg.Add(1)
		go func(mm *Manager) {
			defer wg.Done()
			_, e := mm.withOperation(func() (any, error) {
				mu.Lock()
				active++
				if active > max {
					max = active
				}
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				active--
				mu.Unlock()
				return nil, nil
			})
			if e != nil {
				t.Error(e)
			}
		}(manager)
	}
	wg.Wait()
	if max != 1 {
		t.Fatalf("Concurrent operation count %d", max)
	}
}
func TestRawJSONLogging(t *testing.T) {
	var p []Project
	e := rawJSON(append([]byte("{\"level\":\"info\",\"msg\":\"hello\"}\n"), envelope([]Project{{Name: "test"}})...), &p)
	if e != nil || p[0].Name != "test" {
		t.Fatal(p, e)
	}
}
