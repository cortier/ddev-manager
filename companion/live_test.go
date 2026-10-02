package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Explicitly opt in: creates, starts, restarts, stops, and deletes a disposable DDEV project.
func TestLiveDDEVLifecycle(t *testing.T) {
	if os.Getenv("DDEV_MANAGER_LIVE") != "1" {
		t.Skip("Set DDEV_MANAGER_LIVE=1 to exercise a disposable DDEV project")
	}
	ddev, e := exec.LookPath("ddev")
	if e != nil {
		t.Fatal(e)
	}
	git, e := exec.LookPath("git")
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "public"), 0700)
	os.WriteFile(filepath.Join(root, "public", "index.html"), []byte("DDEV Manager disposable test"), 0600)
	name := fmt.Sprintf("ddev-manager-smoke-%d", time.Now().Unix())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, e = runCommand(ctx, root, ddev, "config", "--project-name="+name, "--project-type=generic", "--docroot=public", "--omit-containers=db,ddev-ssh-agent", "--php-version=8.5", "--use-dns-when-possible=false"); e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, e := runCommand(cleanupCtx, root, ddev, "delete", "--omit-snapshot", "--yes"); e != nil {
			t.Errorf("Disposable project cleanup failed (%s): %v", name, e)
		}
	}()
	l := layoutFor(t.TempDir(), "linux")
	os.MkdirAll(l.Data, 0700)
	m := &Manager{config: Config{DdevPath: ddev, GitPath: git}, layout: l, run: runCommand}
	for _, action := range []string{"open", "restart", "stop", "start", "stop"} {
		t.Log("Testing", action)
		result, e := m.action(ctx, name, action, "")
		if e != nil {
			t.Fatal(action, e)
		}
		if action == "open" && !validURL(result.(map[string]string)["url"]) {
			t.Fatal("No launch URL")
		}
		p, e := m.project(ctx, name)
		if e != nil {
			t.Fatal(e)
		}
		expected := "running"
		if action == "stop" {
			expected = "stopped"
		}
		if p.Status != expected {
			t.Fatalf("After %s: %s", action, p.Status)
		}
	}
}
