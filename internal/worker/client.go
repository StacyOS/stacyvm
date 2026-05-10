package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/workerproto"
)

type Client struct {
	BaseURL    string
	WorkerID   string
	Token      string
	TokenFunc  func() (string, error)
	HTTPClient *http.Client
}

func (c Client) Heartbeat(ctx context.Context, params workerproto.HeartbeatParams) error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("control plane URL is required")
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		return fmt.Errorf("worker id is required")
	}
	token, err := c.authToken()
	if err != nil {
		return err
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/api/v1/worker/" + c.WorkerID + "/heartbeat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Worker-ID", c.WorkerID)
	req.Header.Set("X-Worker-Token", token)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("worker heartbeat failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func (c Client) RenewLease(ctx context.Context, resourceID, ttl string) (workerproto.LeaseToken, error) {
	var zero workerproto.LeaseToken
	if strings.TrimSpace(c.BaseURL) == "" {
		return zero, fmt.Errorf("control plane URL is required")
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		return zero, fmt.Errorf("worker id is required")
	}
	token, err := c.authToken()
	if err != nil {
		return zero, err
	}
	if strings.TrimSpace(resourceID) == "" {
		return zero, fmt.Errorf("lease resource id is required")
	}
	body, err := json.Marshal(workerproto.RenewLeaseParams{ResourceID: resourceID, TTL: ttl})
	if err != nil {
		return zero, err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/api/v1/worker/" + c.WorkerID + "/leases/" + resourceID + "/renew"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Worker-ID", c.WorkerID)
	req.Header.Set("X-Worker-Token", token)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return zero, fmt.Errorf("worker lease renewal failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var result workerproto.RenewLeaseResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, err
	}
	return result.Lease, nil
}

// NewIssuerTokenFunc returns a TokenFunc that fetches a short-lived signed worker
// token from the control-plane's centralized issuer endpoint. This lets workers
// authenticate without needing direct access to auth.worker_signing_key.
//
// bootstrapKey is an admin API key used solely to call POST /api/v1/admin/worker-tokens.
// ttl is the desired token lifetime (max 15 minutes).
func NewIssuerTokenFunc(controlPlaneURL, workerID, bootstrapAdminKey, ttl string) func() (string, error) {
	if ttl == "" {
		ttl = "5m"
	}
	return func() (string, error) {
		base := strings.TrimRight(controlPlaneURL, "/")
		url := base + "/api/v1/admin/worker-tokens"
		body, _ := json.Marshal(map[string]string{
			"worker_id": workerID,
			"ttl":       ttl,
			"audience":  "worker:control-plane",
		})
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("issuer: building request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-API-Key", bootstrapAdminKey)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("issuer: calling control plane: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return "", fmt.Errorf("issuer: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}
		var result struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("issuer: decoding response: %w", err)
		}
		if strings.TrimSpace(result.Token) == "" {
			return "", fmt.Errorf("issuer: response contained no token")
		}
		return result.Token, nil
	}
}

func (c Client) authToken() (string, error) {
	if c.TokenFunc != nil {
		token, err := c.TokenFunc()
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(token) == "" {
			return "", fmt.Errorf("worker token is required")
		}
		return token, nil
	}
	if strings.TrimSpace(c.Token) == "" {
		return "", fmt.Errorf("worker token is required")
	}
	return c.Token, nil
}
