// Command snip runs the Snip link shortener.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // the container image has no zoneinfo

	"github.com/claretnnamocha/snip/internal/store"
	"github.com/claretnnamocha/snip/internal/web"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("snip stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := env("PORT", "5110")
	loc, err := time.LoadLocation(env("SNIP_TIMEZONE", "Africa/Lagos"))
	if err != nil {
		return err
	}

	st, err := store.Open(ctx, env("SNIP_DB", ":memory:"))
	if err != nil {
		return err
	}
	defer st.Close()

	if env("SNIP_SEED", "true") == "true" {
		if err := st.Seed(ctx, time.Now(), loc); err != nil {
			return err
		}
	}

	handler, err := web.New(st, web.Config{
		BaseURL:  env("SNIP_BASE_URL", "http://localhost:"+port),
		Location: loc,
		Logger:   logger,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	logger.Info("snip listening", "addr", "http://localhost:"+port, "timezone", loc.String())
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
