package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/worker"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func newWorkerCmd() *cobra.Command {
	var id string
	var controlPlaneURL string
	var token string
	var tokenFile string
	var signingKeyFile string
	var bootstrapAdminKey string
	var bootstrapAdminKeyFile string
	var bootstrapTokenTTL string
	var heartbeatInterval string
	var listenAddr string
	var previewDomain string
	var once bool
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start a StacyVM remote worker process",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if id == "" {
				id = cfg.Worker.ID
			}
			if id == "" {
				hostname, _ := os.Hostname()
				id = hostname
			}
			if controlPlaneURL == "" {
				controlPlaneURL = cfg.Worker.ControlPlaneURL
			}
			if token == "" {
				token = cfg.Auth.WorkerToken
			}
			var tokenFunc func() (string, error)
			if tokenFile != "" {
				if cmd.Flags().Changed("worker-token") {
					return fmt.Errorf("worker token must be set with either --worker-token or --worker-token-file, not both")
				}
				token = ""
				tokenFunc = fileWorkerTokenFunc(tokenFile)
			}
			signingKey := cfg.Auth.WorkerSigningKey
			if signingKeyFile != "" {
				signingKey, err = readSecretFile(signingKeyFile)
				if err != nil {
					return fmt.Errorf("worker signing key file: %w", err)
				}
			}
			if heartbeatInterval == "" {
				heartbeatInterval = cfg.Worker.HeartbeatInterval
			}
			if listenAddr == "" {
				listenAddr = cfg.Worker.ListenAddr
			}
			if previewDomain == "" {
				previewDomain = cfg.Worker.PreviewDomain
			}
			if previewDomain == "" {
				previewDomain = cfg.Server.PreviewDomain
			}
			interval, err := time.ParseDuration(heartbeatInterval)
			if err != nil {
				return fmt.Errorf("worker heartbeat interval: %w", err)
			}
			logger := newCommandLogger(cfg)
			registry := buildWorkerRegistry(cfg, logger, previewDomain)
			// Resolve bootstrap admin key from flag or file.
			if bootstrapAdminKeyFile != "" {
				bootstrapAdminKey, err = readSecretFile(bootstrapAdminKeyFile)
				if err != nil {
					return fmt.Errorf("bootstrap admin key file: %w", err)
				}
			}
			// Priority: explicit token > token file > bootstrap issuer > local signing key.
			if tokenFunc == nil {
				if bootstrapAdminKey != "" {
					tokenFunc = worker.NewIssuerTokenFunc(controlPlaneURL, id, bootstrapAdminKey, bootstrapTokenTTL)
				} else {
					tokenFunc = signedWorkerTokenFunc(id, signingKey)
					if token != "" {
						tokenFunc = nil
					}
				}
			}
			rt := worker.Runtime{
				Client: worker.Client{
					BaseURL:   strings.TrimRight(controlPlaneURL, "/"),
					WorkerID:  id,
					Token:     token,
					TokenFunc: tokenFunc,
				},
				HeartbeatInterval: interval,
				ListenAddr:        listenAddr,
				Logger:            logger,
				Providers:         enabledProviderNames(cfg),
				Capacity: map[string]interface{}{
					"max_sandboxes":           cfg.Defaults.MaxSandboxes,
					"max_sandboxes_per_owner": cfg.Defaults.MaxSandboxesPerOwner,
					"preview_domain":          previewDomain,
				},
				Registry:        registry,
				RPCTLS:          workerTLSConfig(cfg.Worker.RPCTLS),
				SigningKey:      signingKey,
				SigningKeys:     cfg.Auth.WorkerSigningKeys,
				RevokedTokenIDs: cfg.Auth.WorkerRevokedTokenIDs,
			}
			if once {
				return rt.RunOnce(cmd.Context())
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			logger.Info().Str("worker_id", id).Str("control_plane", controlPlaneURL).Msg("starting StacyVM worker")
			return rt.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "worker ID; defaults to worker.id or hostname")
	cmd.Flags().StringVar(&controlPlaneURL, "control-plane", "", "control plane URL; defaults to worker.control_plane_url")
	cmd.Flags().StringVar(&token, "worker-token", os.Getenv("STACYVM_AUTH_WORKER_TOKEN"), "worker bearer token; defaults to auth.worker_token")
	cmd.Flags().StringVar(&tokenFile, "worker-token-file", "", "file containing the worker bearer token")
	cmd.Flags().StringVar(&signingKeyFile, "worker-signing-key-file", "", "file containing the worker signing key used to derive short-lived signed tokens")
	cmd.Flags().StringVar(&bootstrapAdminKey, "bootstrap-admin-key", os.Getenv("STACYVM_WORKER_BOOTSTRAP_ADMIN_KEY"), "admin API key used to fetch signed worker tokens from the control-plane issuer")
	cmd.Flags().StringVar(&bootstrapAdminKeyFile, "bootstrap-admin-key-file", "", "file containing the admin API key for token issuance")
	cmd.Flags().StringVar(&bootstrapTokenTTL, "bootstrap-token-ttl", "5m", "TTL for tokens fetched from the control-plane issuer (max 15m)")
	cmd.Flags().StringVar(&heartbeatInterval, "heartbeat-interval", "", "worker heartbeat interval")
	cmd.Flags().StringVar(&listenAddr, "listen", "", "worker RPC listen address; defaults to worker.listen_addr")
	cmd.Flags().StringVar(&previewDomain, "preview-domain", "", "worker preview domain; defaults to worker.preview_domain or server.preview_domain")
	cmd.Flags().BoolVar(&once, "once", false, "send one heartbeat and exit")
	cmd.AddCommand(newWorkerTokenCmd())
	return cmd
}

func newWorkerTokenCmd() *cobra.Command {
	var signingKey string
	var signingKeyFile string
	var ttl string
	var scopes []string
	var audience string
	var tokenID string
	var notBefore string
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "token <worker-id>",
		Short: "Issue a signed worker token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if signingKey == "" {
				signingKey = cfg.Auth.WorkerSigningKey
			}
			if signingKeyFile != "" {
				if cmd.Flags().Changed("signing-key") {
					return fmt.Errorf("worker signing key must be set with either --signing-key or --signing-key-file, not both")
				}
				signingKey, err = readSecretFile(signingKeyFile)
				if err != nil {
					return fmt.Errorf("worker signing key file: %w", err)
				}
			}
			result, err := issueWorkerToken(workerTokenIssueOptions{
				WorkerID:   args[0],
				SigningKey: signingKey,
				TTL:        ttl,
				Scopes:     scopes,
				Audience:   audience,
				TokenID:    tokenID,
				NotBefore:  notBefore,
				Now:        time.Now,
			})
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(outputFormat)) {
			case "", "token":
				_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Token)
			case "json":
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				err = encoder.Encode(result)
			default:
				err = fmt.Errorf("worker token output format must be token or json")
			}
			return err
		},
	}
	cmd.Flags().StringVar(&signingKey, "signing-key", os.Getenv("STACYVM_AUTH_WORKER_SIGNING_KEY"), "worker signing key; defaults to auth.worker_signing_key")
	cmd.Flags().StringVar(&signingKeyFile, "signing-key-file", "", "file containing the worker signing key")
	cmd.Flags().StringVar(&ttl, "ttl", "5m", "token lifetime")
	cmd.Flags().StringVar(&audience, "audience", middleware.WorkerTokenAudienceControlPlane, "token audience: worker:control-plane or worker:rpc")
	cmd.Flags().StringVar(&tokenID, "token-id", "", "explicit token id for incident-response tracking; generated when empty")
	cmd.Flags().StringVar(&notBefore, "not-before", "0s", "delay before token becomes valid")
	cmd.Flags().StringVar(&outputFormat, "format", "token", "output format: token or json")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "worker scope to include; repeatable, defaults to all worker scopes")
	cmd.AddCommand(newWorkerTokenInspectCmd())
	cmd.AddCommand(newWorkerTokenVerifyCmd())
	cmd.AddCommand(newWorkerTokenRotationPlanCmd())
	return cmd
}

func newWorkerTokenInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <token>",
		Short: "Inspect signed worker token claims without verifying the signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := inspectWorkerToken(args[0])
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(result)
		},
	}
}

func newWorkerTokenVerifyCmd() *cobra.Command {
	var signingKey string
	var signingKeyFile string
	var verificationKeys []string
	var verificationKeyFiles []string
	var audience string
	var workerID string
	var revokedTokenIDs []string
	cmd := &cobra.Command{
		Use:   "verify <token>",
		Short: "Verify a signed worker token against signing keys and revocation settings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if signingKey == "" {
				signingKey = cfg.Auth.WorkerSigningKey
			}
			if signingKeyFile != "" {
				if cmd.Flags().Changed("signing-key") {
					return fmt.Errorf("worker signing key must be set with either --signing-key or --signing-key-file, not both")
				}
				signingKey, err = readSecretFile(signingKeyFile)
				if err != nil {
					return fmt.Errorf("worker signing key file: %w", err)
				}
			}
			for _, path := range verificationKeyFiles {
				key, err := readSecretFile(path)
				if err != nil {
					return fmt.Errorf("worker verification key file: %w", err)
				}
				verificationKeys = append(verificationKeys, key)
			}
			verificationKeys = append(append([]string{}, cfg.Auth.WorkerSigningKeys...), verificationKeys...)
			revokedTokenIDs = append(append([]string{}, cfg.Auth.WorkerRevokedTokenIDs...), revokedTokenIDs...)
			result, err := verifyWorkerToken(workerTokenVerifyOptions{
				Token:           args[0],
				SigningKey:      signingKey,
				VerificationKey: verificationKeys,
				Audience:        audience,
				WorkerID:        workerID,
				RevokedTokenIDs: revokedTokenIDs,
				Now:             time.Now,
			})
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(result)
		},
	}
	cmd.Flags().StringVar(&signingKey, "signing-key", os.Getenv("STACYVM_AUTH_WORKER_SIGNING_KEY"), "active worker signing key; defaults to auth.worker_signing_key")
	cmd.Flags().StringVar(&signingKeyFile, "signing-key-file", "", "file containing the active worker signing key")
	cmd.Flags().StringArrayVar(&verificationKeys, "verification-key", nil, "additional verification key accepted during rotation; repeatable")
	cmd.Flags().StringArrayVar(&verificationKeyFiles, "verification-key-file", nil, "file containing an additional verification key accepted during rotation; repeatable")
	cmd.Flags().StringVar(&audience, "audience", "", "expected token audience: worker:control-plane or worker:rpc")
	cmd.Flags().StringVar(&workerID, "worker-id", "", "expected worker ID")
	cmd.Flags().StringArrayVar(&revokedTokenIDs, "revoked-token-id", nil, "revoked token ID to reject; repeatable")
	return cmd
}

func newWorkerTokenRotationPlanCmd() *cobra.Command {
	var newKeyRef string
	var previousKeyRef string
	var ttl string
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "rotation-plan",
		Short: "Print a no-secret signed worker token rotation plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := workerTokenRotationPlan(workerTokenRotationPlanOptions{
				NewKeyRef:      newKeyRef,
				PreviousKeyRef: previousKeyRef,
				TTL:            ttl,
			})
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(outputFormat)) {
			case "", "text":
				_, err = fmt.Fprint(cmd.OutOrStdout(), result.Text)
			case "json":
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				err = encoder.Encode(result)
			default:
				err = fmt.Errorf("worker token rotation-plan output format must be text or json")
			}
			return err
		},
	}
	cmd.Flags().StringVar(&newKeyRef, "new-key-ref", "auth.worker_signing_key", "operator-visible reference for the new active signing key")
	cmd.Flags().StringVar(&previousKeyRef, "previous-key-ref", "auth.worker_signing_keys[0]", "operator-visible reference for the previous verification key")
	cmd.Flags().StringVar(&ttl, "ttl", "5m", "maximum signed worker token lifetime to wait before removing the previous key")
	cmd.Flags().StringVar(&outputFormat, "format", "text", "output format: text or json")
	return cmd
}

type workerTokenIssueOptions struct {
	WorkerID   string
	SigningKey string
	TTL        string
	Scopes     []string
	Audience   string
	TokenID    string
	NotBefore  string
	Now        func() time.Time
}

type workerTokenIssueResult struct {
	Token     string   `json:"token"`
	TokenID   string   `json:"token_id"`
	WorkerID  string   `json:"worker_id"`
	Audience  string   `json:"audience"`
	Scopes    []string `json:"scopes,omitempty"`
	IssuedAt  string   `json:"issued_at"`
	NotBefore string   `json:"not_before,omitempty"`
	ExpiresAt string   `json:"expires_at"`
}

type workerTokenInspectResult struct {
	SignatureVerified bool     `json:"signature_verified"`
	WorkerID          string   `json:"worker_id,omitempty"`
	TokenID           string   `json:"token_id,omitempty"`
	Audience          string   `json:"audience,omitempty"`
	Scopes            []string `json:"scopes,omitempty"`
	IssuedAt          string   `json:"issued_at,omitempty"`
	NotBefore         string   `json:"not_before,omitempty"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
}

type workerTokenVerifyOptions struct {
	Token           string
	SigningKey      string
	VerificationKey []string
	Audience        string
	WorkerID        string
	RevokedTokenIDs []string
	Now             func() time.Time
}

type workerTokenRotationPlanOptions struct {
	NewKeyRef      string
	PreviousKeyRef string
	TTL            string
}

type workerTokenRotationPlanResult struct {
	NewKeyRef      string   `json:"new_key_ref"`
	PreviousKeyRef string   `json:"previous_key_ref"`
	MaxTokenTTL    string   `json:"max_token_ttl"`
	Steps          []string `json:"steps"`
	ConfigSnippet  string   `json:"config_snippet"`
	Validation     []string `json:"validation"`
	Text           string   `json:"text,omitempty"`
}

func issueWorkerToken(opts workerTokenIssueOptions) (workerTokenIssueResult, error) {
	workerID := strings.TrimSpace(opts.WorkerID)
	signingKey := strings.TrimSpace(opts.SigningKey)
	if workerID == "" {
		return workerTokenIssueResult{}, fmt.Errorf("worker id is required")
	}
	if signingKey == "" {
		return workerTokenIssueResult{}, fmt.Errorf("worker signing key is required")
	}
	ttl, err := time.ParseDuration(opts.TTL)
	if err != nil {
		return workerTokenIssueResult{}, fmt.Errorf("worker token ttl: %w", err)
	}
	if ttl <= 0 {
		return workerTokenIssueResult{}, fmt.Errorf("worker token ttl must be positive")
	}
	if ttl > middleware.MaxWorkerTokenTTL {
		return workerTokenIssueResult{}, fmt.Errorf("worker token ttl must be <= %s", middleware.MaxWorkerTokenTTL)
	}
	audience := strings.TrimSpace(opts.Audience)
	if audience == "" {
		audience = middleware.WorkerTokenAudienceControlPlane
	}
	if audience != middleware.WorkerTokenAudienceControlPlane && audience != middleware.WorkerTokenAudienceRPC {
		return workerTokenIssueResult{}, fmt.Errorf("worker token audience must be %q or %q", middleware.WorkerTokenAudienceControlPlane, middleware.WorkerTokenAudienceRPC)
	}
	notBefore := strings.TrimSpace(opts.NotBefore)
	if notBefore == "" {
		notBefore = "0s"
	}
	notBeforeDelay, err := time.ParseDuration(notBefore)
	if err != nil {
		return workerTokenIssueResult{}, fmt.Errorf("worker token not-before: %w", err)
	}
	if notBeforeDelay < 0 {
		return workerTokenIssueResult{}, fmt.Errorf("worker token not-before must be non-negative")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	issuedAt := now().UTC()
	tokenID := strings.TrimSpace(opts.TokenID)
	if tokenID == "" {
		tokenID, err = middleware.NewWorkerTokenID()
		if err != nil {
			return workerTokenIssueResult{}, fmt.Errorf("worker token id: %w", err)
		}
	}
	notBeforeAt := issuedAt.Add(notBeforeDelay)
	claims := middleware.WorkerTokenClaims{
		WorkerID:  workerID,
		TokenID:   tokenID,
		Audience:  audience,
		Scopes:    opts.Scopes,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: issuedAt.Add(ttl).Unix(),
	}
	result := workerTokenIssueResult{
		TokenID:   tokenID,
		WorkerID:  workerID,
		Audience:  audience,
		Scopes:    opts.Scopes,
		IssuedAt:  issuedAt.Format(time.RFC3339),
		ExpiresAt: issuedAt.Add(ttl).Format(time.RFC3339),
	}
	if notBeforeDelay > 0 {
		claims.NotBefore = notBeforeAt.Unix()
		result.NotBefore = notBeforeAt.Format(time.RFC3339)
	}
	result.Token, err = middleware.SignWorkerToken(signingKey, claims)
	if err != nil {
		return workerTokenIssueResult{}, err
	}
	return result, nil
}

func inspectWorkerToken(token string) (workerTokenInspectResult, error) {
	claims, ok := middleware.DecodeWorkerTokenClaims(token)
	if !ok {
		return workerTokenInspectResult{}, fmt.Errorf("invalid signed worker token format")
	}
	return workerTokenInspectResult{
		SignatureVerified: false,
		WorkerID:          claims.WorkerID,
		TokenID:           claims.TokenID,
		Audience:          claims.Audience,
		Scopes:            claims.Scopes,
		IssuedAt:          unixTimeString(claims.IssuedAt),
		NotBefore:         unixTimeString(claims.NotBefore),
		ExpiresAt:         unixTimeString(claims.ExpiresAt),
	}, nil
}

func verifyWorkerToken(opts workerTokenVerifyOptions) (workerTokenInspectResult, error) {
	keys := cleanStrings(append([]string{opts.SigningKey}, opts.VerificationKey...))
	if len(keys) == 0 {
		return workerTokenInspectResult{}, fmt.Errorf("worker signing key is required")
	}
	audience := strings.TrimSpace(opts.Audience)
	if audience != "" && audience != middleware.WorkerTokenAudienceControlPlane && audience != middleware.WorkerTokenAudienceRPC {
		return workerTokenInspectResult{}, fmt.Errorf("worker token audience must be %q or %q", middleware.WorkerTokenAudienceControlPlane, middleware.WorkerTokenAudienceRPC)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	var claims middleware.WorkerTokenClaims
	var ok bool
	for _, key := range keys {
		claims, ok = middleware.VerifyWorkerTokenForAudience(key, opts.Token, audience, now().UTC())
		if ok {
			break
		}
	}
	if !ok {
		return workerTokenInspectResult{}, fmt.Errorf("invalid signed worker token")
	}
	if expectedWorkerID := strings.TrimSpace(opts.WorkerID); expectedWorkerID != "" && claims.WorkerID != expectedWorkerID {
		return workerTokenInspectResult{}, fmt.Errorf("worker token worker_id %q does not match expected worker %q", claims.WorkerID, expectedWorkerID)
	}
	revoked := map[string]struct{}{}
	for _, id := range cleanStrings(opts.RevokedTokenIDs) {
		revoked[id] = struct{}{}
	}
	if _, isRevoked := revoked[claims.TokenID]; claims.TokenID != "" && isRevoked {
		return workerTokenInspectResult{}, fmt.Errorf("worker token %q is revoked", claims.TokenID)
	}
	result, _ := inspectWorkerToken(opts.Token)
	result.SignatureVerified = true
	return result, nil
}

func cleanStrings(values []string) []string {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			clean = append(clean, value)
		}
	}
	return clean
}

func readSecretFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("secret file is empty")
	}
	return secret, nil
}

func fileWorkerTokenFunc(path string) func() (string, error) {
	return func() (string, error) {
		token, err := readSecretFile(path)
		if err != nil {
			return "", fmt.Errorf("worker token file: %w", err)
		}
		return token, nil
	}
}

func workerTokenRotationPlan(opts workerTokenRotationPlanOptions) (workerTokenRotationPlanResult, error) {
	newKeyRef := strings.TrimSpace(opts.NewKeyRef)
	previousKeyRef := strings.TrimSpace(opts.PreviousKeyRef)
	if newKeyRef == "" {
		return workerTokenRotationPlanResult{}, fmt.Errorf("new key reference is required")
	}
	if previousKeyRef == "" {
		return workerTokenRotationPlanResult{}, fmt.Errorf("previous key reference is required")
	}
	ttl := strings.TrimSpace(opts.TTL)
	if ttl == "" {
		ttl = "5m"
	}
	duration, err := time.ParseDuration(ttl)
	if err != nil {
		return workerTokenRotationPlanResult{}, fmt.Errorf("worker token rotation ttl: %w", err)
	}
	if duration <= 0 {
		return workerTokenRotationPlanResult{}, fmt.Errorf("worker token rotation ttl must be positive")
	}
	if duration > middleware.MaxWorkerTokenTTL {
		return workerTokenRotationPlanResult{}, fmt.Errorf("worker token rotation ttl must be <= %s", middleware.MaxWorkerTokenTTL)
	}
	steps := []string{
		fmt.Sprintf("Put the new active signing key at %s.", newKeyRef),
		fmt.Sprintf("Move the previous active signing key into %s.", previousKeyRef),
		"Restart or reload control-plane and worker processes so new tokens are minted with the new key.",
		fmt.Sprintf("Wait at least %s, plus clock skew, before removing the previous verification key.", duration),
		"Remove the previous verification key after old signed worker tokens have expired.",
	}
	configSnippet := fmt.Sprintf("auth:\n  worker_signing_key: \"<new key from %s>\"\n  worker_signing_keys:\n    - \"<previous key from %s>\"\n", newKeyRef, previousKeyRef)
	validation := []string{
		"stacyvm config lint --production",
		"stacyvm worker token <worker-id> --ttl " + duration.String() + " --format json",
		"stacyvm worker token verify '<signed-worker-token>' --worker-id <worker-id> --audience worker:control-plane",
	}
	result := workerTokenRotationPlanResult{
		NewKeyRef:      newKeyRef,
		PreviousKeyRef: previousKeyRef,
		MaxTokenTTL:    duration.String(),
		Steps:          steps,
		ConfigSnippet:  configSnippet,
		Validation:     validation,
	}
	result.Text = formatWorkerTokenRotationPlan(result)
	return result, nil
}

func formatWorkerTokenRotationPlan(result workerTokenRotationPlanResult) string {
	var b strings.Builder
	b.WriteString("Signed worker token rotation plan\n\n")
	b.WriteString("Key references:\n")
	b.WriteString(fmt.Sprintf("- new active key: %s\n", result.NewKeyRef))
	b.WriteString(fmt.Sprintf("- previous verification key: %s\n", result.PreviousKeyRef))
	b.WriteString(fmt.Sprintf("- maximum token TTL: %s\n\n", result.MaxTokenTTL))
	b.WriteString("Steps:\n")
	for i, step := range result.Steps {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
	}
	b.WriteString("\nConfig sketch:\n")
	b.WriteString(result.ConfigSnippet)
	b.WriteString("\nValidation:\n")
	for _, command := range result.Validation {
		b.WriteString("- " + command + "\n")
	}
	return b.String()
}

func unixTimeString(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func signedWorkerTokenFunc(workerID, signingKey string) func() (string, error) {
	signingKey = strings.TrimSpace(signingKey)
	workerID = strings.TrimSpace(workerID)
	if signingKey == "" || workerID == "" {
		return nil
	}
	return func() (string, error) {
		now := time.Now().UTC()
		tokenID, err := middleware.NewWorkerTokenID()
		if err != nil {
			return "", fmt.Errorf("worker token id: %w", err)
		}
		return middleware.SignWorkerToken(signingKey, middleware.WorkerTokenClaims{
			WorkerID:  workerID,
			TokenID:   tokenID,
			Audience:  middleware.WorkerTokenAudienceControlPlane,
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(5 * time.Minute).Unix(),
		})
	}
}

func workerTLSConfig(cfg config.WorkerRPCTLSConfig) worker.TLSConfig {
	return worker.TLSConfig{
		Enabled:            cfg.Enabled,
		ServerCertFile:     cfg.ServerCertFile,
		ServerKeyFile:      cfg.ServerKeyFile,
		ClientCAFile:       cfg.ClientCAFile,
		CAFile:             cfg.CAFile,
		ClientCertFile:     cfg.ClientCertFile,
		ClientKeyFile:      cfg.ClientKeyFile,
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
}

func newCommandLogger(cfg *config.Config) zerolog.Logger {
	var logger zerolog.Logger
	if cfg.Logging.Format == "pretty" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	if level, err := zerolog.ParseLevel(cfg.Logging.Level); err == nil {
		logger = logger.Level(level)
	}
	return logger
}

func enabledProviderNames(cfg *config.Config) []string {
	var providers []string
	if cfg.Providers.Mock.Enabled {
		providers = append(providers, "mock")
	}
	if cfg.Providers.Firecracker.Enabled {
		providers = append(providers, "firecracker")
	}
	if cfg.Providers.Docker.Enabled {
		providers = append(providers, "docker")
	}
	if cfg.Providers.E2B.Enabled {
		providers = append(providers, "e2b")
	}
	if cfg.Providers.Custom.Enabled {
		providers = append(providers, cfg.Providers.Custom.Name)
	}
	if cfg.Providers.PRoot.Enabled {
		providers = append(providers, "proot")
	}
	return providers
}

func buildWorkerRegistry(cfg *config.Config, logger zerolog.Logger, previewDomain string) *providers.Registry {
	registry := providers.NewRegistry()
	if cfg.Providers.Mock.Enabled {
		registry.Register(providers.NewMockProvider())
	}
	if cfg.Providers.Docker.Enabled {
		docker, err := providers.NewDockerProvider(providers.DockerProviderConfig{
			Socket:         cfg.Providers.Docker.Socket,
			Runtime:        cfg.Providers.Docker.Runtime,
			DefaultImage:   cfg.Providers.Docker.DefaultImage,
			NetworkMode:    cfg.Providers.Docker.NetworkMode,
			SeccompProfile: cfg.Providers.Docker.SeccompProfile,
			ReadOnlyRootfs: cfg.Providers.Docker.ReadOnlyRootfs,
			Memory:         cfg.Providers.Docker.Memory,
			CPUs:           cfg.Providers.Docker.CPUs,
			PidsLimit:      cfg.Providers.Docker.PidsLimit,
			User:           cfg.Providers.Docker.User,
			DroppedCaps:    cfg.Providers.Docker.DroppedCaps,
			AddedCaps:      cfg.Providers.Docker.AddedCaps,
			Tmpfs:          cfg.Providers.Docker.Tmpfs,
			PoolSecurity: providers.PoolSecurityProviderConfig{
				PerUserUID:           cfg.Providers.Docker.PoolSecurity.PerUserUID,
				PIDNamespace:         cfg.Providers.Docker.PoolSecurity.PIDNamespace,
				WorkspacePermissions: cfg.Providers.Docker.PoolSecurity.WorkspacePermissions,
				HidePID:              cfg.Providers.Docker.PoolSecurity.HidePID,
			},
			PreviewDomain: previewDomain,
		}, logger)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create worker docker provider")
		} else {
			registry.Register(docker)
		}
	}
	if len(registry.List()) > 0 {
		if err := registry.SetDefault(cfg.Providers.Default); err != nil {
			_ = registry.SetDefault(registry.List()[0])
		}
	}
	return registry
}
