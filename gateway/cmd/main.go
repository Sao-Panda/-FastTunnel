package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reasonix/api-tunnel/config"
	"github.com/reasonix/api-tunnel/pkg/logger"
	"github.com/reasonix/api-tunnel/pkg/proxy"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Init logger
	log, err := logger.New(cfg.Log.Level, cfg.Log.Format, cfg.Log.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	log.Info("api-tunnel gateway starting", "version", "1.0.0")

	// Build proxy config
	proxyCfg := &proxy.Config{
		ListenAddr:    cfg.Gateway.ListenAddr,
		UpstreamURL:   cfg.Upstream.URL,
		BindIP:        cfg.Gateway.BindIP,
		BindInterface: cfg.Gateway.BindInterface,
		ReadTimeout:   cfg.Gateway.ReadTimeout,
		WriteTimeout:  cfg.Gateway.WriteTimeout,
		IdleTimeout:   cfg.Gateway.IdleTimeout,
		MaxRetries:    cfg.Gateway.MaxRetries,
		RetryDelay:    cfg.Gateway.RetryDelay,
		UpstreamTO:    cfg.Upstream.Timeout,
		PreserveHost:  cfg.Upstream.PreserveHost,
		AllowedHdrs:   cfg.Upstream.AllowedHeaders,
		StripHdrs:     cfg.Upstream.StripHeaders,
		SetHdrs:       cfg.Upstream.SetHeaders,
		AuthToken:     cfg.Gateway.AuthToken,
	}
	if cfg.Gateway.TLS != nil {
		proxyCfg.TLS = &proxy.TLSConfig{
			CertFile: cfg.Gateway.TLS.CertFile,
			KeyFile:  cfg.Gateway.TLS.KeyFile,
		}
	}

	// Create and start proxy
	p := proxy.New(proxyCfg, log.Logger)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Info("received signal, shutting down", "signal", sig.String())
		cancel()
	}()

	go func() {
		if err := p.Start(); err != nil {
			log.Error("proxy failed", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := p.Stop(shutdownCtx); err != nil {
		log.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	log.Info("gateway stopped cleanly")
}
