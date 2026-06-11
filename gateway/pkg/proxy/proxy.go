package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Stats tracks proxy statistics.
type Stats struct {
	TotalRequests   atomic.Int64
	ActiveRequests  atomic.Int64
	FailedRequests  atomic.Int64
	TotalLatencyUs  atomic.Int64
	LastRequestTime atomic.Int64 // unix nano
}

// Proxy is the core reverse proxy engine with dedicated network binding.
type Proxy struct {
	cfg    *Config
	logger *slog.Logger
	Stats  *Stats
	srv    *http.Server
}

// Config holds the proxy configuration.
type Config struct {
	ListenAddr    string
	UpstreamURL   string
	RemoteGateway string // optional remote gateway (for 2-hop: local→remote→upstream)
	BindIP        string
	BindInterface string
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	MaxRetries    int
	RetryDelay    time.Duration
	UpstreamTO    time.Duration
	PreserveHost  bool
	AllowedHdrs   []string
	StripHdrs     []string
	SetHdrs       map[string]string
	AuthToken     string
	TLS           *TLSConfig
}

// TLSConfig for the proxy listener.
type TLSConfig struct {
	CertFile string
	KeyFile  string
}

// New creates a new Proxy.
func New(cfg *Config, logger *slog.Logger) *Proxy {
	return &Proxy{
		cfg:    cfg,
		logger: logger,
		Stats:  &Stats{},
	}
}

// Start begins listening and serving. Blocks until error.
func (p *Proxy) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", p.handleHealth)
	mux.HandleFunc("/stats", p.handleStats)
	mux.HandleFunc("/", p.handleProxy)

	p.srv = &http.Server{
		Addr:         p.cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  p.cfg.ReadTimeout,
		WriteTimeout: p.cfg.WriteTimeout,
		IdleTimeout:  p.cfg.IdleTimeout,
	}

	p.logger.Info("proxy starting",
		"listen", p.cfg.ListenAddr,
		"upstream", p.cfg.UpstreamURL,
		"bind_ip", p.cfg.BindIP,
		"bind_interface", p.cfg.BindInterface,
	)

	if p.cfg.TLS != nil {
		return p.srv.ListenAndServeTLS(p.cfg.TLS.CertFile, p.cfg.TLS.KeyFile)
	}
	return p.srv.ListenAndServe()
}

// Stop gracefully shuts down the proxy.
func (p *Proxy) Stop(ctx context.Context) error {
	p.logger.Info("proxy stopping")
	return p.srv.Shutdown(ctx)
}

// StatsSnapshot returns a copy of current stats.
func (p *Proxy) StatsSnapshot() map[string]interface{} {
	totalLat := p.Stats.TotalLatencyUs.Load()
	totalReq := p.Stats.TotalRequests.Load()
	avgLat := float64(0)
	if totalReq > 0 {
		avgLat = float64(totalLat) / float64(totalReq) / 1000.0 // ms
	}
	return map[string]interface{}{
		"total_requests":   totalReq,
		"active_requests":  p.Stats.ActiveRequests.Load(),
		"failed_requests":  p.Stats.FailedRequests.Load(),
		"avg_latency_ms":   avgLat,
		"last_request":     p.Stats.LastRequestTime.Load(),
	}
}

func (p *Proxy) handleHealth(w http.ResponseWriter, r *http.Request) {
	upstreamOK := p.checkUpstream(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if upstreamOK {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","upstream":"reachable"}`)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"degraded","upstream":"unreachable"}`)
	}
}

func (p *Proxy) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s := p.StatsSnapshot()
	fmt.Fprintf(w, `{"total_requests":%d,"active_requests":%d,"failed_requests":%d,"avg_latency_ms":%.2f}`,
		s["total_requests"], s["active_requests"], s["failed_requests"], s["avg_latency_ms"])
}

func (p *Proxy) checkUpstream(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	transport := p.buildTransport()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, p.cfg.UpstreamURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Auth check
	if p.cfg.AuthToken != "" {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + p.cfg.AuthToken
		if auth != expected {
			p.logger.Warn("auth failed", "remote", r.RemoteAddr)
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}

	p.Stats.TotalRequests.Add(1)
	p.Stats.ActiveRequests.Add(1)
	defer p.Stats.ActiveRequests.Add(-1)
	p.Stats.LastRequestTime.Store(time.Now().UnixNano())

	start := time.Now()
	defer func() {
		p.Stats.TotalLatencyUs.Add(time.Since(start).Microseconds())
	}()

	rp, err := p.buildReverseProxy()
	if err != nil {
		p.logger.Error("build reverse proxy", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		p.Stats.FailedRequests.Add(1)
		return
	}

	var lastErr error
	for attempt := 0; attempt <= p.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := p.cfg.RetryDelay * time.Duration(1<<(attempt-1))
			p.logger.Info("retrying",
				"attempt", attempt,
				"url", r.URL.String(),
				"backoff", backoff.String(),
			)
			time.Sleep(backoff)
		}

		rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		rp.ServeHTTP(rec, r)

		if rec.statusCode < 500 {
			p.logger.Info("request served",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.statusCode,
				"latency_us", time.Since(start).Microseconds(),
				"attempt", attempt+1,
			)
			return
		}
		lastErr = fmt.Errorf("upstream returned %d", rec.statusCode)
	}

	p.Stats.FailedRequests.Add(1)
	p.logger.Error("all retries exhausted",
		"method", r.Method,
		"path", r.URL.Path,
		"error", lastErr,
	)
	http.Error(w, `{"error":"upstream unavailable"}`, http.StatusBadGateway)
}

func (p *Proxy) buildReverseProxy() (*httputil.ReverseProxy, error) {
	target, err := url.Parse(p.cfg.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream URL: %w", err)
	}

	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = p.buildTransport()

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)

		for _, h := range p.cfg.StripHdrs {
			req.Header.Del(h)
		}
		for k, v := range p.cfg.SetHdrs {
			req.Header.Set(k, v)
		}
		if p.cfg.PreserveHost {
			req.Host = req.Header.Get("Host")
		}
		if len(p.cfg.AllowedHdrs) > 0 {
			allowed := make(map[string]bool)
			for _, h := range p.cfg.AllowedHdrs {
				allowed[strings.ToLower(h)] = true
			}
			for k := range req.Header {
				if !allowed[strings.ToLower(k)] {
					req.Header.Del(k)
				}
			}
		}
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		p.logger.Error("proxy error",
			"method", r.Method,
			"path", r.URL.Path,
			"error", err.Error(),
		)
		http.Error(w, `{"error":"proxy error"}`, http.StatusBadGateway)
	}

	return rp, nil
}

// buildTransport creates an HTTP transport with optional interface/IP binding.
func (p *Proxy) buildTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Apply interface/IP binding (platform-specific)
	if p.cfg.BindIP != "" || p.cfg.BindInterface != "" {
		applyBind(dialer, p.cfg.BindIP, p.cfg.BindInterface)
	}

	return &http.Transport{
		DialContext:           dialer.DialContext,
		TLSClientConfig:       &tls.Config{},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: p.cfg.UpstreamTO,
	}
}

// responseRecorder captures the status code for retry logic.
type responseRecorder struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.statusCode = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.statusCode = http.StatusOK
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}
