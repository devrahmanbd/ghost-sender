package api

import (
    "context"
    "crypto/tls"
    "fmt"
    "net/http"
    "time"

    "email-campaign-system/internal/api/websocket"
    "email-campaign-system/internal/config"
    "email-campaign-system/pkg/logger"
)

type Server struct {
    httpServer *http.Server
    router     *Router
    hub        *websocket.Hub
    config     *config.AppConfig
    log        logger.Logger
}

type ServerConfig struct {
    Host              string
    Port              int
    ReadTimeout       time.Duration
    WriteTimeout      time.Duration
    IdleTimeout       time.Duration
    MaxHeaderBytes    int
    ShutdownTimeout   time.Duration
    EnableTLS         bool
    TLSCertFile       string
    TLSKeyFile        string
    TLSMinVersion     uint16
    EnableHTTP2       bool
}

func NewServer(
    router *Router,
    hub *websocket.Hub,
    cfg *config.AppConfig,
    log logger.Logger,
) *Server {
    serverCfg := extractServerConfig(cfg)

    addr := fmt.Sprintf("%s:%d", serverCfg.Host, serverCfg.Port)

    httpServer := &http.Server{
        Addr:           addr,
        Handler:        router,
        ReadTimeout:    serverCfg.ReadTimeout,
        WriteTimeout:   serverCfg.WriteTimeout,
        IdleTimeout:    serverCfg.IdleTimeout,
        MaxHeaderBytes: serverCfg.MaxHeaderBytes,
    }

    if serverCfg.EnableTLS {
        tlsConfig := &tls.Config{
            MinVersion:               serverCfg.TLSMinVersion,
            PreferServerCipherSuites: true,
            CurvePreferences: []tls.CurveID{
                tls.CurveP256,
                tls.X25519,
            },
            CipherSuites: []uint16{
                tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
                tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
                tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
                tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
            },
        }

        if serverCfg.EnableHTTP2 {
            tlsConfig.NextProtos = []string{"h2", "http/1.1"}
        }

        httpServer.TLSConfig = tlsConfig
    }

    return &Server{
        httpServer: httpServer,
        router:     router,
        hub:        hub,
        config:     cfg,
        log:        log,
    }
}

func (s *Server) Start(ctx context.Context) error {
    s.hub.Start(ctx)

    serverCfg := extractServerConfig(s.config)

    s.log.Info("starting http server",
        logger.String("address", s.httpServer.Addr),
        logger.Bool("tls_enabled", serverCfg.EnableTLS),
        logger.Bool("http2_enabled", serverCfg.EnableHTTP2),
    )

    errChan := make(chan error, 1)

    go func() {
        var err error
        if serverCfg.EnableTLS {
            err = s.httpServer.ListenAndServeTLS(
                serverCfg.TLSCertFile,
                serverCfg.TLSKeyFile,
            )
        } else {
            err = s.httpServer.ListenAndServe()
        }

        if err != nil && err != http.ErrServerClosed {
            errChan <- err
        }
    }()

    select {
    case err := <-errChan:
        return fmt.Errorf("server failed to start: %w", err)
    case <-ctx.Done():
        return s.Shutdown(context.Background())
    case <-time.After(100 * time.Millisecond):
        s.log.Info("http server started successfully",
            logger.String("address", s.httpServer.Addr),
        )
        return nil
    }
}

func (s *Server) Shutdown(ctx context.Context) error {
    serverCfg := extractServerConfig(s.config)

    shutdownCtx, cancel := context.WithTimeout(ctx, serverCfg.ShutdownTimeout)
    defer cancel()

    s.log.Info("shutting down http server",
        logger.Duration("timeout", serverCfg.ShutdownTimeout),
    )

    s.hub.Stop()

    if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
        s.log.Error("error during server shutdown", 
            logger.Error(err),
        )
        return fmt.Errorf("server shutdown failed: %w", err)
    }

    s.log.Info("http server shutdown completed")
    return nil
}

func (s *Server) Close() error {
    s.hub.Stop()
    return s.httpServer.Close()
}

func (s *Server) Addr() string {
    return s.httpServer.Addr
}

func (s *Server) ListenAndServe() error {
    serverCfg := extractServerConfig(s.config)

    s.log.Info("starting http server",
        logger.String("address", s.httpServer.Addr),
        logger.Bool("tls_enabled", serverCfg.EnableTLS),
    )

    if serverCfg.EnableTLS {
        return s.httpServer.ListenAndServeTLS(
            serverCfg.TLSCertFile,
            serverCfg.TLSKeyFile,
        )
    }

    return s.httpServer.ListenAndServe()
}

func extractServerConfig(cfg *config.AppConfig) ServerConfig {
    serverCfg := ServerConfig{
        Host:            "0.0.0.0",
        Port:            8080,
        ReadTimeout:     15 * time.Second,
        WriteTimeout:    15 * time.Second,
        IdleTimeout:     60 * time.Second,
        MaxHeaderBytes:  1 << 20,
        ShutdownTimeout: 30 * time.Second,
        EnableTLS:       false,
        TLSMinVersion:   tls.VersionTLS12,
        EnableHTTP2:     true,
    }

    if cfg.Server.Host != "" {
        serverCfg.Host = cfg.Server.Host
    }
    if cfg.Server.Port > 0 {
        serverCfg.Port = cfg.Server.Port
    }
    if cfg.Server.ReadTimeout > 0 {
        serverCfg.ReadTimeout = cfg.Server.ReadTimeout
    }
    if cfg.Server.WriteTimeout > 0 {
        serverCfg.WriteTimeout = cfg.Server.WriteTimeout
    }
    if cfg.Server.IdleTimeout > 0 {
        serverCfg.IdleTimeout = cfg.Server.IdleTimeout
    }
    if cfg.Server.ShutdownTimeout > 0 {
        serverCfg.ShutdownTimeout = cfg.Server.ShutdownTimeout
    }
    if cfg.Server.MaxHeaderBytes > 0 {
        serverCfg.MaxHeaderBytes = cfg.Server.MaxHeaderBytes
    }

    // Map from AppConfig's flat TLS fields
    serverCfg.EnableTLS = cfg.Server.TLSEnabled
    serverCfg.TLSCertFile = cfg.Server.TLSCertFile
    serverCfg.TLSKeyFile = cfg.Server.TLSKeyFile

    return serverCfg
}
