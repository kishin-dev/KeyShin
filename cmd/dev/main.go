// Command dev runs KeyShin locally the same way Vercel serves it:
// static files from public/ with clean URLs, and the Go handlers from api/
// under /api/*.
//
//	go run ./cmd/dev         # uses your Supabase project from .env
//	go run ./cmd/dev -fake   # uses an in-memory fake Supabase, no setup needed
//
// It reads settings from a .env file in the project root if one exists.
// This command is only for local development; Vercel never builds it.
package main

import (
	"bufio"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	handler "github.com/kishin-dev/keyshin/api"
	"github.com/kishin-dev/keyshin/lib/supafake"
)

func main() {
	fake := flag.Bool("fake", false, "use an in-memory fake Supabase (login: admin / password)")
	flag.Parse()

	loadDotEnv(".env")
	if os.Getenv("KEYSHIN_DEV") == "" {
		os.Setenv("KEYSHIN_DEV", "1") // allow the cookies over plain http://localhost
	}
	if *fake {
		f := supafake.New()
		defer f.Close()
		f.AddAdmin("admin", f.AddUser("admin@example.com", "password"))
		os.Setenv("SUPABASE_URL", f.URL)
		os.Setenv("SUPABASE_PUBLISHABLE_KEY", supafake.PublishableKey)
		os.Setenv("SUPABASE_SECRET_KEY", supafake.SecretKey)
		log.Print("Using fake Supabase. Sign in with admin / password")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", handler.Login)
	mux.HandleFunc("/api/logout", handler.Logout)
	mux.HandleFunc("/api/session", handler.Session)
	mux.HandleFunc("/api/stats", handler.Stats)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	mux.Handle("/", cleanURLs("public"))

	addr := "localhost:" + envOr("PORT", "3000")
	log.Printf("KeyShin dev server on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, securityHeaders(mux)))
}

// cleanURLs mimics Vercel's "cleanUrls": /dashboard serves dashboard.html,
// and /dashboard.html redirects to /dashboard.
func cleanURLs(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasSuffix(p, ".html") {
			target := strings.TrimSuffix(p, ".html")
			if target == "/index" {
				target = "/"
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		if p != "/" && filepath.Ext(p) == "" {
			if _, err := os.Stat(filepath.Join(dir, p+".html")); err == nil {
				http.ServeFile(w, r, filepath.Join(dir, p+".html"))
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

// securityHeaders matches the headers set in vercel.json.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// loadDotEnv sets KEY=VALUE pairs from path without overriding variables
// that are already set in the environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
