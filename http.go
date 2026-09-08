package main

import (
	"colorninja/internal/engine"
	"colorninja/internal/studio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type mediaHandler struct{ app *App }

func (handler mediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a := handler.app
	if a.studio == nil {
		http.Error(w, "Starting", 503)
		return
	}
	a.studio.ServeHTTP(w, r)
}

// The optional loopback host runs the same service as the desktop. It is for
// development and automated UI validation; ordinary desktop builds need no port.
func serveDevelopment(address, configPath string, assets fs.FS) error {
	host, _, e := net.SplitHostPort(address)
	if e != nil {
		return e
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("development server must bind to a loopback IP")
	}
	s := studio.New(context.Background(), configPath)
	defer s.Shutdown()
	mux := http.NewServeMux()
	mux.Handle("/media/", s)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "Invalid origin", 403)
			return
		}
		if r.Header.Get("X-ColorNinja") != "studio" {
			http.Error(w, "Invalid request", 403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var args []json.RawMessage
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&args); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		get := func(i int, v any) error {
			if i >= len(args) {
				return fmt.Errorf("missing argument")
			}
			return json.Unmarshal(args[i], v)
		}
		var result any
		var err error
		switch strings.TrimPrefix(r.URL.Path, "/api/") {
		case "Initialize":
			result, err = s.Initialize()
		case "UseDemo":
			result, err = s.UseDemo()
		case "LoadImage":
			var path string
			if err = get(0, &path); err == nil {
				result, err = s.LoadImage(path)
			}
		case "SetLibrary":
			var path string
			var filter engine.LibraryFilter
			if err = get(0, &path); err == nil {
				err = get(1, &filter)
			}
			if err == nil {
				result, err = s.SetLibrary(path, filter)
			}
		case "Process":
			var req studio.Request
			if err = get(0, &req); err == nil {
				result, err = s.Process(req)
			}
		case "Cancel":
			s.Cancel()
		case "SavePreset":
			var name string
			var o engine.Options
			if err = get(0, &name); err == nil {
				err = get(1, &o)
			}
			if err == nil {
				result, err = s.SavePreset(name, o)
			}
		case "DeletePreset":
			var name string
			if err = get(0, &name); err == nil {
				result, err = s.DeletePreset(name)
			}
		case "SaveProject":
			var req studio.Request
			var path string
			if err = get(0, &req); err == nil {
				err = get(1, &path)
			}
			if err == nil {
				err = s.SaveProject(path, req, false)
				result = path
			}
		case "OpenProject":
			var path string
			if err = get(0, &path); err == nil {
				result, err = s.OpenProject(path)
			}
		case "Export":
			var kind, path string
			var id, revision uint64
			if err = get(0, &kind); err == nil {
				err = get(1, &id)
			}
			if err == nil {
				err = get(2, &revision)
			}
			if err == nil {
				err = get(3, &path)
			}
			if err == nil {
				err = s.Export(kind, path, id, revision, false)
				result = path
			}
		default:
			err = fmt.Errorf("unknown method")
		}
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
		}
	})
	mux.Handle("/", http.FileServer(http.FS(assets)))
	listener, e := net.Listen("tcp", address)
	if e != nil {
		return e
	}
	fmt.Printf("ColorNinja browser QA: http://%s\nSettings: %s\n", listener.Addr(), filepath.Dir(configPath))
	_ = os.MkdirAll(filepath.Dir(configPath), 0755)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() {
			http.Error(w, "Invalid host", 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second}
	return server.Serve(listener)
}
