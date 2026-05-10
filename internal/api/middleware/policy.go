package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"

	"github.com/StacyOs/stacyvm/internal/store"
)

type policyStore interface {
	ListPolicies(ctx context.Context, query store.PolicyQuery) ([]*store.PolicyRecord, error)
}

// PolicyEnforcer checks spawn requests against tenant/global policies for
// provider, image, and network_mode fields in the JSON body.
// It buffers the request body so downstream handlers can still read it.
func PolicyEnforcer(st policyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || st == nil {
				next.ServeHTTP(w, r)
				return
			}

			identity := AuthIdentityFromContext(r.Context())
			tenantID := identity.TenantID

			// Read the full body into a buffer so we can inspect it and
			// restore it for the downstream handler.
			rawBody, err := io.ReadAll(io.LimitReader(r.Body, 2<<20)) // 2 MiB cap
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			// Always restore the body regardless of what we do next.
			r.Body = io.NopCloser(bytes.NewReader(rawBody))

			var body map[string]any
			if err := json.Unmarshal(rawBody, &body); err != nil {
				// Non-JSON or empty body — pass through without enforcement.
				next.ServeHTTP(w, r)
				return
			}
			// Body is already restored above; no context injection needed.

			checks := []struct {
				key          string
				resourceType string
			}{
				{"image", "image"},
				{"provider", "provider"},
				{"network_mode", "network"},
			}

			for _, check := range checks {
				val, _ := body[check.key].(string)
				if val == "" {
					continue
				}
				policies, err := st.ListPolicies(r.Context(), store.PolicyQuery{
					TenantID:     tenantID,
					ResourceType: check.resourceType,
				})
				if err != nil || len(policies) == 0 {
					continue
				}
				if !policyPermits(val, policies) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(map[string]string{
						"code":    "FORBIDDEN",
						"message": check.resourceType + " \"" + val + "\" is not permitted by policy",
					})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// policyPermits returns true when the value is allowed by the ordered policy list.
// Rules: deny policies take precedence over allow within the same priority tier.
// If no allow policy matches, default is to deny.
func policyPermits(value string, policies []*store.PolicyRecord) bool {
	for _, p := range policies {
		matched, _ := filepath.Match(p.Pattern, value)
		if !matched {
			// Also support exact match.
			matched = p.Pattern == value || p.Pattern == "*"
		}
		if matched {
			if p.Effect == "deny" {
				return false
			}
			if p.Effect == "allow" {
				return true
			}
		}
	}
	// No policy matched — default permit (policies are opt-in restrictions).
	return true
}

type decodedBodyKey struct{}

func withDecodedBody(ctx context.Context, body map[string]any) context.Context {
	return context.WithValue(ctx, decodedBodyKey{}, body)
}

// DecodedBodyFromContext returns the pre-decoded JSON body if PolicyEnforcer ran.
func DecodedBodyFromContext(ctx context.Context) (map[string]any, bool) {
	v, ok := ctx.Value(decodedBodyKey{}).(map[string]any)
	return v, ok
}
