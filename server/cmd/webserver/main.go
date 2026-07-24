// Command webserver is the framework-free HTTP entrypoint for the public
// (internet) deployment. It wires the exact same internal/* handlers that the
// internal nuwa build registers in common/server/httpserv, but on a plain
// net/http server — no nuwa/lego/disf/odin, so the binary runs on any host with
// no internal-network dependency.
//
// Config (all via env, same names the cmd/* jobs already use):
//   AIHOT_DATABASE_URL   postgres DSN (required for live data; server still
//                        boots without it and reports /healthz down)
//   AIHOT_HTTP_ADDR      listen address, default ":8991"
//   AIHOT_LLM_*          optional; enables POST /items/{id}/retranslate
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aihot-server/internal/cluster"
	"aihot-server/internal/daily"
	"aihot-server/internal/db"
	"aihot-server/internal/detailpage"
	"aihot-server/internal/health"
	"aihot-server/internal/items"
	"aihot-server/internal/llm"
	"aihot-server/internal/pipeline"
	"aihot-server/internal/publicapi"
	"aihot-server/internal/terms"
	"aihot-server/internal/version"

	"github.com/jackc/pgx/v5/pgxpool"
)

// downPinger backs /healthz when no DB pool could be built, so health reports
// "down" instead of panicking on a nil pool.
type downPinger struct{ err error }

func (d downPinger) Ping(context.Context) error { return d.err }

// newDBPool builds a single shared pgx pool from AIHOT_DATABASE_URL. pgxpool.New
// is lazy, so a live pool comes back even when the DB is momentarily
// unreachable; an unset/malformed DSN yields (nil, err) and the server still
// boots (routes return 500 at request time; /healthz reports down).
func newDBPool() (*pgxpool.Pool, error) {
	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("AIHOT_DATABASE_URL not set")
	}
	return db.NewPool(context.Background(), dsn)
}

func healthPinger(pool *pgxpool.Pool, err error) health.Pinger {
	if err != nil {
		log.Printf("[aihot] db pool unavailable: %v; /healthz will report down", err)
		return downPinger{err: err}
	}
	return pool
}

// recover500 turns a panic in any handler into a 500 instead of killing the
// process — the nuwa build got this from middleware.RecoveryWithConfig.
func recover500(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[aihot] panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func buildMux() http.Handler {
	mux := http.NewServeMux()

	// Single pgx pool shared by /healthz and every data route (mirrors the nuwa
	// build: avoids duplicate connections).
	pool, poolErr := newDBPool()
	itemsStore := items.New(pool) // nil pool -> route still registers, 500s at request time

	mux.Handle("/api/public/version", version.NewHandler())
	mux.Handle("/healthz", health.NewHandler(healthPinger(pool, poolErr)))
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("pong"))
	})

	mux.Handle("/api/public/items", publicapi.NewItemsHandler(itemsStore, time.Now))

	// SSR detail page GET /items/{id}; POST /items/{id}/retranslate when an LLM
	// key is configured.
	detailHandler := detailpage.NewHandler(itemsStore)
	if retryClient, err := llm.NewRetryTranslateClientFromEnv(); err == nil {
		retryTr := pipeline.NewTranslator(retryClient, retryClient.Model())
		detailHandler = detailHandler.WithRetranslate(retryTr, itemsStore)
	} else {
		log.Printf("[aihot] retranslate disabled: %v", err)
	}
	mux.Handle("/items/", detailHandler)

	// Daily. EnsureSchema is best-effort: a failure only logs, the server still
	// boots and the handler 500s at request time. Exact /api/public/daily wins
	// over the /daily/ subtree in ServeMux (longest-pattern match).
	dailyStore := daily.NewStore(pool)
	ensure("daily", pool, dailyStore.EnsureSchema)
	mux.Handle("/api/public/daily", publicapi.NewLatestDailyHandler(dailyStore))
	mux.Handle("/api/public/daily/", publicapi.NewDailyByDateHandler(dailyStore))
	mux.Handle("/api/public/dailies", publicapi.NewDailiesHandler(dailyStore))

	// Hot topics.
	hotStore := cluster.NewStore(pool)
	ensure("cluster", pool, hotStore.EnsureSchema)
	mux.Handle("/api/public/hot-topics", publicapi.NewHotTopicsHandler(hotStore))

	// Knowledge graph / word cloud.
	termsStore := terms.NewStore(pool)
	ensure("terms", pool, termsStore.EnsureSchema)
	mux.Handle("/api/public/graph/cloud", publicapi.NewGraphCloudHandler(termsStore, time.Now))
	mux.Handle("/api/public/graph/term/", publicapi.NewGraphTermHandler(termsStore, time.Now))

	return recover500(mux)
}

// ensure runs a best-effort EnsureSchema, skipping when the pool is nil.
func ensure(name string, pool *pgxpool.Pool, fn func(context.Context) error) {
	if pool == nil {
		return
	}
	if err := fn(context.Background()); err != nil {
		log.Printf("[aihot] %s EnsureSchema failed: %v", name, err)
	}
}

func main() {
	addr := os.Getenv("AIHOT_HTTP_ADDR")
	if addr == "" {
		addr = ":8991"
	}

	srv := &http.Server{
		Addr:        addr,
		Handler:     buildMux(),
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM (systemd stop / Ctrl-C).
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("[aihot] webserver listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("[aihot] server error: %v", err)
	}
	log.Printf("[aihot] webserver stopped")
}
