// Command watchwhat serves the WatchWhat household tracker.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chriscasey/watchwhat/internal/catalog"
	"github.com/chriscasey/watchwhat/internal/config"
	"github.com/chriscasey/watchwhat/internal/db"
	"github.com/chriscasey/watchwhat/internal/server"
	"github.com/chriscasey/watchwhat/internal/tmdb"
	"github.com/chriscasey/watchwhat/web"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Getenv("WATCHWHAT_ADDR")))
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel()}))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func logLevel() slog.Level {
	if os.Getenv("WATCHWHAT_DEBUG") != "" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := db.Migrate(ctx, database); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	assets, err := web.NewManifest(web.Static)
	if err != nil {
		return err
	}
	now := func() time.Time { return time.Now().In(cfg.Location) }
	app := server.New(cfg, database, tmdb.New(cfg.TMDBToken, cfg.TMDBLanguage), log, now, assets)

	refresher := &catalog.Refresher{Svc: app.Catalog, Log: log}
	refreshDone := make(chan struct{})
	go func() { refresher.Run(ctx); close(refreshDone) }()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "db", cfg.DBPath, "tz", cfg.Location.String())
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		stop()
		<-refreshDone
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutCtx)
	<-refreshDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck GETs /healthz on the local listener; distroless has no curl.
func healthcheck(addr string) int {
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
