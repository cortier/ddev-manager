package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Request struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Method    string `json:"method"`
	SurfaceID string `json:"surfaceId"`
	Action    string `json:"action"`
	ServiceID string `json:"serviceId"`
	ProfileID string `json:"profileId"`
	Proof     string `json:"proof"`
	Config    Config `json:"config"`
}
type Response struct {
	Version int            `json:"version"`
	ID      string         `json:"id"`
	Result  any            `json:"result,omitempty"`
	Error   *ProtocolError `json:"error,omitempty"`
}
type ProtocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func dispatch(l Layout, r Request) (any, error) {
	if r.Version != 1 {
		return nil, errors.New("Protocol version mismatch; update the extension and companion")
	}
	switch r.Method {
	case "register":
		body, _ := json.Marshal(map[string]string{"id": r.ProfileID, "proof": r.Proof})
		return ipc(l, "POST", "/register", string(body))
	case "diagnostics":
		c, e := readConfig(l)
		if e != nil {
			return nil, e
		}
		_, health := ipc(l, "GET", "/health", "")
		effective := c
		if effective.IDEPath == "" {
			effective.IDEPath = os.Getenv("IDE")
		}
		result := map[string]any{"version": 1, "platform": l.OS, "config": effective, "cleanupReady": health == nil}
		if health != nil {
			result["cleanupError"] = health.Error()
		}
		return result, nil
	case "configure":
		c := r.Config
		var e error
		c.DdevPath, e = executablePath(c.DdevPath, "ddev")
		if e != nil {
			return nil, e
		}
		c.GitPath, e = executablePath(c.GitPath, "git")
		if e != nil {
			return nil, e
		}
		if c.IDEPath != "" {
			c.IDEPath, e = absoluteExecutablePath(c.IDEPath, "IDE executable")
			if e != nil {
				return nil, e
			}
		}
		return c, writeJSON(filepath.Join(l.Data, "config.json"), c)
	}
	c, e := readConfig(l)
	if e != nil {
		return nil, e
	}
	m := &Manager{config: c, layout: l, run: runCommand, launch: launchCommand}
	duration := 45 * time.Second
	if r.Method == "action" {
		duration = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	switch r.Method {
	case "discover":
		return m.discover(ctx)
	case "services":
		p, e := m.project(ctx, r.SurfaceID)
		if e != nil {
			return nil, e
		}
		return m.services(ctx, p)
	case "action":
		return m.action(ctx, r.SurfaceID, r.Action, r.ServiceID)
	default:
		return nil, errors.New("Unknown companion request")
	}
}
func readFrame(reader io.Reader) ([]byte, error) {
	var header [4]byte
	if _, e := io.ReadFull(reader, header[:]); e != nil {
		return nil, e
	}
	size := binary.NativeEndian.Uint32(header[:])
	if size == 0 || size > 1<<20 {
		return nil, errors.New("Invalid native message size")
	}
	data := make([]byte, size)
	_, e := io.ReadFull(reader, data)
	return data, e
}
func writeFrame(writer io.Writer, value any) error {
	data, e := json.Marshal(value)
	if e != nil {
		return e
	}
	if len(data) > 1<<20 {
		return errors.New("Native response exceeds 1 MiB")
	}
	var header [4]byte
	binary.NativeEndian.PutUint32(header[:], uint32(len(data)))
	if _, e = writer.Write(header[:]); e != nil {
		return e
	}
	_, e = writer.Write(data)
	return e
}
func native(l Layout, reader io.Reader, writer io.Writer) error {
	var writing sync.Mutex
	var workers sync.WaitGroup
	slots := make(chan struct{}, 16)
	for {
		b, e := readFrame(reader)
		if e != nil {
			if e == io.EOF {
				workers.Wait()
				return nil
			}
			return e
		}
		var r Request
		if e = json.Unmarshal(b, &r); e != nil {
			return errors.New("Malformed native request")
		}
		slots <- struct{}{}
		workers.Add(1)
		go func(r Request) {
			defer workers.Done()
			defer func() { <-slots }()
			result, e := dispatch(l, r)
			response := Response{Version: 1, ID: r.ID, Result: result}
			if e != nil {
				response.Result = nil
				response.Error = &ProtocolError{Code: "COMMAND_FAILED", Message: e.Error()}
			}
			writing.Lock()
			defer writing.Unlock()
			if e = writeFrame(writer, response); e != nil {
				_ = writeFrame(writer, Response{Version: 1, ID: r.ID, Error: &ProtocolError{Code: "RESPONSE_TOO_LARGE", Message: e.Error()}})
			}
		}(r)
	}
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	l, e := currentLayout()
	if e != nil {
		return e
	}
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "install":
			f := flag.NewFlagSet("install", flag.ContinueOnError)
			d := f.String("ddev", "", "Absolute DDEV executable path")
			g := f.String("git", "", "Absolute Git executable path")
			if e = f.Parse(args[1:]); e != nil {
				return e
			}
			exe, e := os.Executable()
			if e != nil {
				return e
			}
			c := Config{DdevPath: *d, GitPath: *g}
			if old, e := readConfig(l); e == nil {
				if c.DdevPath == "" {
					c.DdevPath = old.DdevPath
				}
				if c.GitPath == "" {
					c.GitPath = old.GitPath
				}
			}
			if e = install(l, exe, c, systemCommand); e != nil {
				return e
			}
			deadline := time.Now().Add(8 * time.Second)
			for {
				if _, err := ipc(l, "GET", "/health", ""); err == nil {
					break
				}
				if time.Now().After(deadline) {
					return errors.New("Companion files installed but cleanup service is not ready; run doctor")
				}
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Println("DDEV Manager companion installed. Install the Firefox extension next.")
			return nil
		case "uninstall":
			if e = uninstall(l, systemCommand); e != nil {
				return e
			}
			fmt.Println("DDEV Manager companion removed. DDEV projects were not changed.")
			return nil
		case "serve":
			return serve(l)
		case "cleanup-helper":
			return cleanupHelper(l)
		case "doctor":
			c, e := readConfig(l)
			if e != nil {
				return e
			}
			fmt.Printf("Platform: %s\nDDEV: %s\nGit: %s\n", l.OS, c.DdevPath, c.GitPath)
			if _, e = executablePath(c.DdevPath, "ddev"); e != nil {
				return e
			}
			if _, e = executablePath(c.GitPath, "git"); e != nil {
				return e
			}
			if _, e = ipc(l, "GET", "/health", ""); e != nil {
				return e
			}
			fmt.Println("Cleanup service: connected")
			return nil
		case "discover":
			loadPath(l)
			v, e := dispatch(l, Request{Version: 1, Method: "discover"})
			if e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(v)
		case "--version":
			fmt.Println("DDEV Manager 0.1.0 (protocol 1)")
			return nil
		}
	}
	// Firefox launches the native host with manifest path and calling extension ID.
	if len(args) != 2 || args[1] != extensionID || !strings.HasSuffix(args[0], hostName+".json") {
		return errors.New("Use install, uninstall, doctor, discover, or --version; native mode is reserved for Firefox")
	}
	loadPath(l)
	return native(l, os.Stdin, os.Stdout)
}
func loadPath(l Layout) {
	if b, e := os.ReadFile(filepath.Join(l.Data, "path")); e == nil && len(b) > 0 {
		_ = os.Setenv("PATH", string(b))
	}
}
