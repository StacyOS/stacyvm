package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OIDCConfig holds OIDC/JWT verification settings.
// Supports RS256 (RSA), ES256/ES384/ES512 (ECDSA) via JWKS or static PEM key.
type OIDCConfig struct {
	Issuer         string
	Audience       string
	JWKSUrl        string
	PublicKeyPEM   string
	GroupsClaim    string
	TenantClaim    string
	AdminGroups    []string
	OperatorGroups []string
	ViewerGroups   []string
	Now            func() time.Time
}

type JWTClaims struct {
	Subject  string   `json:"sub"`
	Issuer   string   `json:"iss"`
	Audience aud      `json:"aud"`
	Expiry   int64    `json:"exp"`
	IssuedAt int64    `json:"iat"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Groups   []string `json:"-"` // extracted via GroupsClaim
	TenantID string   `json:"-"` // extracted via TenantClaim
	Extra    map[string]json.RawMessage
}

// aud handles both string and []string audience in JWT.
type aud []string

func (a *aud) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*a = []string{s}
		return nil
	}
	var ss []string
	if err := json.Unmarshal(b, &ss); err != nil {
		return err
	}
	*a = ss
	return nil
}

// publicKey holds either an RSA or EC public key from a JWKS or PEM.
type publicKey struct {
	rsa *rsa.PublicKey
	ec  *ecdsa.PublicKey
}

type jwksCache struct {
	mu      sync.RWMutex
	keys    map[string]publicKey
	fetched time.Time
	ttl     time.Duration
	url     string
}

func newJWKSCache(url string) *jwksCache {
	return &jwksCache{url: url, ttl: 5 * time.Minute, keys: map[string]publicKey{}}
}

func (c *jwksCache) get(kid string, now time.Time) (publicKey, error) {
	c.mu.RLock()
	if now.Before(c.fetched.Add(c.ttl)) {
		key, ok := c.keys[kid]
		c.mu.RUnlock()
		if ok {
			return key, nil
		}
		return publicKey{}, fmt.Errorf("jwks: unknown kid %q", kid)
	}
	c.mu.RUnlock()

	keys, err := fetchJWKS(c.url)
	if err != nil {
		return publicKey{}, err
	}

	c.mu.Lock()
	c.keys = keys
	c.fetched = now
	c.mu.Unlock()

	key, ok := keys[kid]
	if !ok {
		return publicKey{}, fmt.Errorf("jwks: unknown kid %q", kid)
	}
	return key, nil
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	// RSA fields
	N string `json:"n"`
	E string `json:"e"`
	// EC fields
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func fetchJWKS(url string) (map[string]publicKey, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching jwks: %w", err)
	}
	defer resp.Body.Close()

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decoding jwks: %w", err)
	}

	keys := make(map[string]publicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		switch k.Kty {
		case "RSA":
			pub, err := rsaPublicKeyFromJWK(k)
			if err != nil {
				continue
			}
			keys[k.Kid] = publicKey{rsa: pub}
		case "EC":
			pub, err := ecPublicKeyFromJWK(k)
			if err != nil {
				continue
			}
			keys[k.Kid] = publicKey{ec: pub}
		}
	}
	return keys, nil
}

func rsaPublicKeyFromJWK(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	return &rsa.PublicKey{N: n, E: e}, nil
}

func ecPublicKeyFromJWK(k jwk) (*ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	switch k.Crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported EC curve %q", k.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

func parsePublicKeyPEM(pemData string) (publicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return publicKey{}, errors.New("oidc: invalid PEM block")
	}
	switch block.Type {
	case "PUBLIC KEY":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return publicKey{}, err
		}
		switch k := key.(type) {
		case *rsa.PublicKey:
			return publicKey{rsa: k}, nil
		case *ecdsa.PublicKey:
			return publicKey{ec: k}, nil
		default:
			return publicKey{}, fmt.Errorf("oidc: unsupported public key type %T", key)
		}
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return publicKey{}, err
		}
		switch k := cert.PublicKey.(type) {
		case *rsa.PublicKey:
			return publicKey{rsa: k}, nil
		case *ecdsa.PublicKey:
			return publicKey{ec: k}, nil
		default:
			return publicKey{}, fmt.Errorf("oidc: unsupported certificate public key type %T", cert.PublicKey)
		}
	default:
		return publicKey{}, fmt.Errorf("oidc: unsupported PEM block type %q", block.Type)
	}
}

// OIDCAuth returns middleware that validates OIDC Bearer JWTs and injects
// AuthIdentity into the request context alongside the existing API-key path.
func OIDCAuth(cfg OIDCConfig) func(http.Handler) http.Handler {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	var staticKey publicKey
	if cfg.PublicKeyPEM != "" {
		k, err := parsePublicKeyPEM(cfg.PublicKeyPEM)
		if err == nil {
			staticKey = k
		}
	}

	var cache *jwksCache
	if cfg.JWKSUrl != "" {
		cache = newJWKSCache(cfg.JWKSUrl)
	}

	adminGroups := groupSet(cfg.AdminGroups)
	operatorGroups := groupSet(cfg.OperatorGroups)
	viewerGroups := groupSet(cfg.ViewerGroups)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only intercept Bearer tokens; fall through for X-API-Key.
			token := bearerToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := verifyJWT(token, cfg.Issuer, cfg.Audience, staticKey, cache, now())
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "UNAUTHORIZED",
					"message": "invalid or expired JWT: " + err.Error(),
				})
				return
			}

			// Extract groups from configured claim.
			if cfg.GroupsClaim != "" && cfg.GroupsClaim != "groups" {
				if raw, ok := claims.Extra[cfg.GroupsClaim]; ok {
					json.Unmarshal(raw, &claims.Groups)
				}
			}
			// Extract tenant from configured claim.
			if cfg.TenantClaim != "" {
				if raw, ok := claims.Extra[cfg.TenantClaim]; ok {
					var s string
					if err := json.Unmarshal(raw, &s); err == nil {
						claims.TenantID = s
					}
				}
			}

			role := oidcRole(claims.Groups, adminGroups, operatorGroups, viewerGroups)
			identity := AuthIdentity{
				Role:     role,
				Header:   "Authorization",
				Scopes:   scopesForRole(role),
				Subject:  claims.Subject,
				Email:    claims.Email,
				TenantID: claims.TenantID,
				Groups:   claims.Groups,
			}
			r = r.WithContext(WithAuthIdentity(r.Context(), identity))
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	v := r.Header.Get("Authorization")
	if len(v) > len(prefix) && strings.EqualFold(v[:len(prefix)], prefix) {
		candidate := strings.TrimSpace(v[len(prefix):])
		// Exclude signed worker tokens so they go through WorkerAuth, not OIDC.
		if !strings.HasPrefix(candidate, workerSignedTokenPrefix+".") {
			return candidate
		}
	}
	return ""
}

func verifyJWT(token, issuer, audience string, staticKey publicKey, cache *jwksCache, now time.Time) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed JWT")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("malformed JWT header")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("malformed JWT header")
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("malformed JWT signature")
	}

	signed := parts[0] + "." + parts[1]

	// Resolve the key to use for verification.
	var key publicKey
	if staticKey.rsa != nil || staticKey.ec != nil {
		key = staticKey
	} else if cache != nil {
		key, err = cache.get(header.Kid, now)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("no OIDC public key or JWKS URL configured")
	}

	// Verify signature based on algorithm.
	switch header.Alg {
	case "RS256":
		if key.rsa == nil {
			return nil, fmt.Errorf("algorithm RS256 requires an RSA key, got EC")
		}
		h := sha256.Sum256([]byte(signed))
		if err := rsa.VerifyPKCS1v15(key.rsa, 0, h[:], sigBytes); err != nil {
			return nil, errors.New("JWT signature verification failed")
		}
	case "ES256":
		if key.ec == nil {
			return nil, fmt.Errorf("algorithm ES256 requires an EC key, got RSA")
		}
		h := sha256.Sum256([]byte(signed))
		if !ecdsa.VerifyASN1(key.ec, h[:], sigBytes) {
			return nil, errors.New("JWT signature verification failed")
		}
	case "ES384":
		if key.ec == nil {
			return nil, fmt.Errorf("algorithm ES384 requires an EC key, got RSA")
		}
		h := sha512.Sum384([]byte(signed))
		if !ecdsa.VerifyASN1(key.ec, h[:], sigBytes) {
			return nil, errors.New("JWT signature verification failed")
		}
	case "ES512":
		if key.ec == nil {
			return nil, fmt.Errorf("algorithm ES512 requires an EC key, got RSA")
		}
		h := sha512.Sum512([]byte(signed))
		if !ecdsa.VerifyASN1(key.ec, h[:], sigBytes) {
			return nil, errors.New("JWT signature verification failed")
		}
	default:
		return nil, fmt.Errorf("unsupported JWT algorithm %q; supported: RS256, ES256, ES384, ES512", header.Alg)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("malformed JWT payload")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payloadBytes, &raw); err != nil {
		return nil, errors.New("malformed JWT payload")
	}

	claims := &JWTClaims{Extra: raw}
	for k, v := range raw {
		switch k {
		case "sub":
			json.Unmarshal(v, &claims.Subject)
		case "iss":
			json.Unmarshal(v, &claims.Issuer)
		case "aud":
			json.Unmarshal(v, &claims.Audience)
		case "exp":
			json.Unmarshal(v, &claims.Expiry)
		case "iat":
			json.Unmarshal(v, &claims.IssuedAt)
		case "email":
			json.Unmarshal(v, &claims.Email)
		case "name":
			json.Unmarshal(v, &claims.Name)
		case "groups":
			json.Unmarshal(v, &claims.Groups)
		}
	}

	if claims.Expiry <= 0 || !now.Before(time.Unix(claims.Expiry, 0)) {
		return nil, errors.New("JWT is expired")
	}
	if issuer != "" && claims.Issuer != issuer {
		return nil, fmt.Errorf("JWT issuer %q does not match expected %q", claims.Issuer, issuer)
	}
	if audience != "" {
		found := false
		for _, a := range claims.Audience {
			if a == audience {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("JWT audience does not contain %q", audience)
		}
	}

	return claims, nil
}

func groupSet(groups []string) map[string]struct{} {
	m := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		if g = strings.TrimSpace(g); g != "" {
			m[g] = struct{}{}
		}
	}
	return m
}

func oidcRole(groups []string, adminGroups, operatorGroups, viewerGroups map[string]struct{}) AuthRole {
	for _, g := range groups {
		if _, ok := adminGroups[g]; ok {
			return AuthRoleAdmin
		}
	}
	for _, g := range groups {
		if _, ok := operatorGroups[g]; ok {
			return AuthRoleOperator
		}
	}
	for _, g := range groups {
		if _, ok := viewerGroups[g]; ok {
			return AuthRoleViewer
		}
	}
	// Default to API role when authenticated but no group matches.
	return AuthRoleAPI
}

// TenantIDFromContext returns the tenant ID stored in context, if any.
func TenantIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(tenantIDContextKey{}).(string)
	return id
}

type tenantIDContextKey struct{}

// WithTenantID injects a tenant ID into context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDContextKey{}, tenantID)
}
