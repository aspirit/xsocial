// Package server provides common server utilities for HTTP services.
// It simplifies the setup and graceful shutdown of HTTP servers.
package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	httputil "github.com/aspirit/xsocial/pkg/http"
)

// Logger interface defines what a logger should implement.
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// ServiceDeps interface defines what a service dependency should implement.
type ServiceDeps interface {
	// GetLogger returns the logger instance
	GetLogger() Logger

	// Close closes all service-specific resources
	Close() error
}

// ServerConfig holds server configuration including address and timeouts.
type ServerConfig struct {
	// Service ID (e.g., "xs-api", "xs-worker")
	ID string `mapstructure:"id"`

	// Server address to listen on (e.g., ":8080")
	Addr string `mapstructure:"addr"`

	// Timeout configurations
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`

	// TrustedProxies lists the IPs/CIDRs of reverse proxies allowed to set
	// client-IP headers (X-Forwarded-For). When empty, no proxy is trusted and
	// ClientIP() falls back to the direct connection address, which cannot be
	// spoofed. In production set this to the ingress gateway range only.
	TrustedProxies []string `mapstructure:"trusted_proxies"`

	// CORS configuration
	CORS *CORSConfig `mapstructure:"cors"`
}

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	// Enabled controls whether CORS middleware is active
	Enabled bool `mapstructure:"enabled"`

	// AllowedOrigins is a list of origins a cross-domain request can be executed from
	// This is the only environment-specific configuration
	AllowOrigins []string `mapstructure:"allow_origins"`

	// AllowCredentials indicates whether the request can include user credentials like cookies
	AllowCredentials bool `mapstructure:"allow_credentials"`

	// AllowWildcardSubdomains indicates whether to allow wildcard subdomains
	AllowWildcardSubdomains bool `mapstructure:"allow_wildcard_subdomains"`
}

// GetCORSMethods returns the standard allowed HTTP methods.
func GetCORSMethods() []string {
	return []string{
		"GET",
		"POST",
		"PUT",
		"PATCH",
		"DELETE",
		"OPTIONS",
		"HEAD",
	}
}

// GetCORSHeaders returns the standard allowed headers.
func GetCORSHeaders() []string {
	return []string{
		httputil.HeaderAuthorization,
		httputil.HeaderContentType,
		"Accept",
		"Origin",
		"X-Requested-With",
		"X-CSRF-Token",
		"X-Request-ID",
		httputil.HeaderDeviceID,
		httputil.HeaderAcceptLanguage,
		"Cache-Control",
		"Pragma",
		"traceparent",
		"tracestate",
	}
}

// GetCORSExposeHeaders returns the standard exposed headers.
func GetCORSExposeHeaders() []string {
	return []string{
		"Content-Length",
		"Content-Type",
		"X-Request-ID",
		"X-Total-Count",
		"X-Page-Count",
		"X-Rate-Limit-Remaining",
	}
}

// GetCORSMaxAge returns the standard max age for preflight requests.
func GetCORSMaxAge() int {
	return 86400 // 24 hours
}

// FillDefaults sets default values for empty fields.
func (s *ServerConfig) FillDefaults() {
	if s.Addr == "" {
		s.Addr = ":8080"
	}
	if s.ReadTimeout == 0 {
		s.ReadTimeout = 5 * time.Second
	}
	if s.WriteTimeout == 0 {
		s.WriteTimeout = 10 * time.Second
	}
	if s.IdleTimeout == 0 {
		s.IdleTimeout = 30 * time.Second
	}
	if s.ShutdownTimeout == 0 {
		s.ShutdownTimeout = 10 * time.Second
	}
	if s.CORS != nil {
		s.CORS.FillDefaults()
	}
}

// FillDefaults sets default values for CORS configuration.
func (c *CORSConfig) FillDefaults() {
	if !c.Enabled {
		c.Enabled = true
	}
	if len(c.AllowOrigins) == 0 {
		// Use default development origins
		c.AllowOrigins = []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"http://127.0.0.1:3000",
		}
	}
	// Other values are now handled by the middleware using the standard functions
}

// Start starts an HTTP server with graceful shutdown support.
func Start(
	engine *gin.Engine,
	config *ServerConfig,
	logger Logger,
) error {
	if config == nil {
		config = &ServerConfig{
			ID:   "default-service",
			Addr: ":8080",
		}
	}

	// Always ensure defaults are set
	config.FillDefaults()

	// Create context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Create HTTP server with timeout configurations
	srv := &http.Server{
		Addr:         config.Addr,
		Handler:      engine,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	// Log server configuration
	logger.Info("Starting server with configuration",
		zap.String("address", config.Addr),
		zap.Duration("read_timeout", config.ReadTimeout),
		zap.Duration("write_timeout", config.WriteTimeout),
		zap.Duration("idle_timeout", config.IdleTimeout),
		zap.Duration("shutdown_timeout", config.ShutdownTimeout),
	)

	// Start server in a goroutine
	srvErr := make(chan error, 1)
	go func() {
		logger.Info("Starting server", zap.String("address", config.Addr))
		srvErr <- srv.ListenAndServe()
	}()

	// Wait for interruption or server error
	select {
	case err := <-srvErr:
		logger.Error("Server failed to start", zap.Error(err))

		return err
	case <-ctx.Done():
		logger.Info("Received shutdown signal")
		stop()
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	// Shutdown HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown failed", zap.Error(err))

		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	logger.Info("Server shutdown complete")

	return nil
}

// StartWithDeps is a convenience function that combines
// server startup with dependency management.
func StartWithDeps(
	engine *gin.Engine,
	config *ServerConfig,
	deps ServiceDeps,
) error {
	// Start server
	if err := Start(engine, config, deps.GetLogger()); err != nil {
		return err
	}

	// Close dependencies
	if err := deps.Close(); err != nil {
		return fmt.Errorf("failed to close service dependencies: %w", err)
	}

	return nil
}
