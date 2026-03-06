package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jessevdk/go-flags"

	"facebook-account-parser/internal/config"
	"facebook-account-parser/internal/db"
	"facebook-account-parser/internal/facebook"
	"facebook-account-parser/internal/logger"
	internalsync "facebook-account-parser/internal/sync"
	"facebook-account-parser/internal/token"
	"facebook-account-parser/internal/web"
)

var version = "dev"

type Options struct {
	Config  string `short:"c" long:"config" env:"CONFIG" default:"config.yml" description:"path to config file"`
	Debug   bool   `long:"dbg" env:"DEBUG" description:"enable debug logging with colors and caller info"`
	Version bool   `short:"V" long:"version" description:"show version info"`
}

func main() {
	var opts Options
	p := flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	if _, err := p.Parse(); err != nil {
		if fe, ok := err.(*flags.Error); ok && fe.Type == flags.ErrHelp {
			fmt.Println(err)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf("fb-spend-tracker %s\n", version)
		os.Exit(0)
	}

	setupLog(opts.Debug)

	cfg, err := config.Load(opts.Config)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	encKey, err := cfg.EncryptionKeyBytes()
	if err != nil {
		slog.Error("invalid encryption key", "err", err)
		os.Exit(1)
	}

	sqlDB, err := db.Open(cfg.DB.Path)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}

	store := db.NewStore(sqlDB)
	fbClient := facebook.NewClient(cfg.Facebook.APIVersion)
	tokenMgr := token.New(store, fbClient.Limiter(), encKey)
	pipeline := internalsync.New(store, fbClient, tokenMgr, cfg.Sync.BackfillDays)

	scheduler, err := internalsync.NewScheduler(pipeline, cfg.Sync.Schedule)
	if err != nil {
		slog.Error("failed to create scheduler", "err", err)
		os.Exit(1)
	}
	scheduler.Start()
	defer scheduler.Stop()

	webHandler, err := web.New(store, tokenMgr, pipeline)
	if err != nil {
		slog.Error("failed to create web handler", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	webHandler.Register(mux)

	slog.Info("server starting", "addr", cfg.Server.Addr, "version", version)
	if err := http.ListenAndServe(cfg.Server.Addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func setupLog(dbg bool) {
	if dbg {
		slog.SetDefault(slog.New(logger.NewDebugHandler(os.Stdout)))
		slog.Debug("debug mode ON")
		return
	}
	slog.SetDefault(slog.New(logger.NewHandler(os.Stdout, slog.LevelInfo)))
}
