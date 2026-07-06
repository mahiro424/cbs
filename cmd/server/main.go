package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/httpapi"
	"github.com/mahiro424/cbs/internal/storage"
)

func main() {
	configPath := flag.String("config", "conf/app.conf", "??????")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		logger.Warn("?????????????", "path", *configPath, "error", err)
		cfg = config.Default()
	}

	redisStatus := storage.CheckRedis(context.Background(), cfg)
	if redisStatus.Available {
		logger.Info("Redis ?????", "address", redisStatus.Address, "database", redisStatus.Database)
	} else {
		logger.Warn("Redis ???????? mock-first ????", "address", redisStatus.Address, "database", redisStatus.Database, "message", redisStatus.Message)
	}

	srv := &http.Server{Addr: cfg.ListenAddress(), Handler: httpapi.NewServer(cfg), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("????", "listen", cfg.ListenAddress(), "routes", len(httpapi.AllRoutes()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("??????", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("??????", "error", err)
	}
}
