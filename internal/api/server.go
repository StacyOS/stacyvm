package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/api/routes"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/StacyOs/stacyvm/docs"
)

type ServerConfig struct {
	Addr                  string
	APIKey                string
	AdminAPIKey           string
	AdminFallbackDisabled bool
	AdminAuditRetention   time.Duration
	WorkerToken           string
	WorkerTokens          map[string]string
	WorkerSigningKey      string
	WorkerSigningKeys     []string
	WorkerRevokedTokenIDs []string
	Version               string
	RateLimit             middleware.RateLimitConfig
	WorkerHeartbeat       time.Duration
	OIDC                  middleware.OIDCConfig
}

type Server struct {
	httpServer      *http.Server
	logger          zerolog.Logger
	workerHeartbeat *localWorkerHeartbeat
}

//	@title			StacyVM API
//	@version		1.0
//	@description	StacyVM microVM sandbox orchestrator API

//	@host		localhost:7423
//	@BasePath	/api/v1

//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-API-Key

func NewServer(cfg ServerConfig, registry *providers.Registry, manager *orchestrator.Manager, events *orchestrator.EventBus, templates *orchestrator.TemplateRegistry, pool *orchestrator.PoolManager, st store.Store, envBuild routes.BuildStarter, logger zerolog.Logger) *Server {
	r := chi.NewRouter()
	heartbeatInterval := cfg.WorkerHeartbeat
	if heartbeatInterval == 0 {
		heartbeatInterval = 30 * time.Second
	}
	workerHeartbeat := newLocalWorkerHeartbeat(registry, manager, st, logger, heartbeatInterval)
	workerHeartbeat.register(context.Background())

	// Global middleware (applies to all routes including swagger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(logger))

	// Swagger UI — public, no auth required
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	workerRoutes := routes.NewWorkerRoutes(st)
	r.Route("/api/v1/worker", func(r chi.Router) {
		r.Use(middleware.WorkerAuthWithConfig(middleware.WorkerAuthConfig{
			SharedToken:     cfg.WorkerToken,
			WorkerTokens:    cfg.WorkerTokens,
			SigningKey:      cfg.WorkerSigningKey,
			SigningKeys:     cfg.WorkerSigningKeys,
			RevokedTokenIDs: cfg.WorkerRevokedTokenIDs,
		}))
		r.With(middleware.RequireScope(middleware.ScopeWorkerHeartbeat)).Post("/{workerID}/heartbeat", workerRoutes.Heartbeat)
		r.With(middleware.RequireScope(middleware.ScopeWorkerLease)).Post("/{workerID}/leases/{resourceID}/renew", workerRoutes.RenewLease)
	})

	// API routes — with auth and CORS
	r.Group(func(r chi.Router) {
		// OIDC Bearer token auth (runs before API key auth; falls through on no Bearer token).
		if cfg.OIDC.Issuer != "" || cfg.OIDC.JWKSUrl != "" || cfg.OIDC.PublicKeyPEM != "" {
			r.Use(middleware.OIDCAuth(cfg.OIDC))
		}
		if cfg.APIKey != "" || cfg.AdminAPIKey != "" {
			r.Use(middleware.AuthAny(cfg.APIKey, cfg.AdminAPIKey))
		}

		// CORS
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Admin-API-Key, X-Request-ID, X-User-ID, X-Tenant-ID, Authorization")
				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}
				next.ServeHTTP(w, r)
			})
		})

		var rateLimiter *middleware.RateLimiter
		if cfg.RateLimit.Enabled {
			rateLimiter = middleware.NewRateLimiter(cfg.RateLimit)
			r.Use(rateLimiter.Middleware)
		}

		// Routes
		sandboxRoutes := routes.NewSandboxRoutes(manager)
		providerRoutes := routes.NewProviderRoutes(registry, manager)
		templateRoutes := routes.NewTemplateRoutes(templates, manager)
		snapshotRoutes := routes.NewSnapshotRoutes(registry)
		systemRoutes := routes.NewSystemRoutes(registry, manager, events, st, cfg.Version, rateLimiter)
		environmentRoutes := routes.NewEnvironmentRoutes(st, envBuild)
		quotaRoutes := routes.NewQuotaRoutes(manager)
		adminAuditRoutes := routes.NewAdminAuditRoutes(st)
		tenantRoutes := routes.NewTenantRoutes(st)
		tokenIssuerRoutes := routes.NewWorkerTokenIssuerRoutes(cfg.WorkerSigningKey)
		authConfigured := cfg.APIKey != "" || cfg.AdminAPIKey != "" || cfg.OIDC.Issuer != "" || cfg.OIDC.JWKSUrl != "" || cfg.OIDC.PublicKeyPEM != ""
		r.Route("/api/v1", func(r chi.Router) {
			r.Mount("/sandboxes", sandboxRoutes.RoutesWithScopeEnforcement(authConfigured))
			r.Mount("/providers", providerRoutes.Routes())
			r.Mount("/templates", templateRoutes.Routes())
			r.Mount("/snapshots", snapshotRoutes.Routes())
			r.Mount("/environments", environmentRoutes.Routes())
			r.Mount("/quotas", quotaRoutes.Routes())
			r.Mount("/workers", workerRoutes.ReadOnlyRoutes())
			r.Get("/pool/status", sandboxRoutes.VMPoolStatus)
			r.Route("/admin", func(r chi.Router) {
				r.Use(middleware.AdminAuth(cfg.AdminAPIKey, cfg.APIKey, !cfg.AdminFallbackDisabled))
				if cfg.APIKey != "" || cfg.AdminAPIKey != "" {
					r.Use(middleware.RequireScope(middleware.ScopeAdmin))
				}
				r.Use(middleware.AdminAudit(st, logger, cfg.AdminAuditRetention))
				r.Get("/audit", adminAuditRoutes.List)
				r.Mount("/providers", providerRoutes.Routes())
				r.Mount("/quotas", quotaRoutes.Routes())
				r.Mount("/workers", workerRoutes.Routes())
				r.Mount("/tenants", tenantRoutes.Routes())
				r.Post("/worker-tokens", tokenIssuerRoutes.IssueToken)
				r.Get("/diagnostics", systemRoutes.Diagnostics)
				r.Get("/metrics", systemRoutes.Metrics)
				r.Get("/metrics/prometheus", systemRoutes.PrometheusMetrics)
			})
			r.Mount("/", systemRoutes.Routes())
		})
	})

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      r,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		logger:          logger,
		workerHeartbeat: workerHeartbeat,
	}
}

type localWorkerHeartbeat struct {
	registry *providers.Registry
	manager  *orchestrator.Manager
	store    store.Store
	logger   zerolog.Logger
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func newLocalWorkerHeartbeat(registry *providers.Registry, manager *orchestrator.Manager, st store.Store, logger zerolog.Logger, interval time.Duration) *localWorkerHeartbeat {
	return &localWorkerHeartbeat{
		registry: registry,
		manager:  manager,
		store:    st,
		logger:   logger,
		interval: interval,
	}
}

func (h *localWorkerHeartbeat) start() {
	if h == nil || h.store == nil || h.interval <= 0 {
		return
	}
	h.mu.Lock()
	if h.cancel != nil {
		h.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	h.cancel = cancel
	h.done = done
	h.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.register(ctx)
			}
		}
	}()
}

func (h *localWorkerHeartbeat) stop() {
	if h == nil {
		return
	}
	h.mu.Lock()
	cancel := h.cancel
	done := h.done
	h.cancel = nil
	h.done = nil
	h.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (h *localWorkerHeartbeat) register(ctx context.Context) {
	if h == nil || h.store == nil {
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "local"
	}
	capabilities := []string{"api", "single_node", "spawn", "exec", "files"}
	providerNames := h.registry.List()
	providersJSON, _ := json.Marshal(providerNames)
	capabilitiesJSON, _ := json.Marshal(capabilities)
	capacityJSON, _ := json.Marshal(h.manager.Limits())
	if err := h.store.SaveWorker(ctx, &store.WorkerRecord{
		ID:            "local",
		Hostname:      hostname,
		Status:        "online",
		Providers:     string(providersJSON),
		Capabilities:  string(capabilitiesJSON),
		Capacity:      string(capacityJSON),
		LastHeartbeat: time.Now().UTC(),
	}); err != nil {
		h.logger.Warn().Err(err).Msg("failed to register local worker")
	}
}

func (s *Server) Start() error {
	s.logger.Info().Str("addr", s.httpServer.Addr).Msg("starting HTTP server")
	s.workerHeartbeat.start()
	err := s.httpServer.ListenAndServe()
	if err != nil {
		s.workerHeartbeat.stop()
	}
	return err
}

func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("shutting down HTTP server")
	s.workerHeartbeat.stop()
	return s.httpServer.Shutdown(ctx)
}
