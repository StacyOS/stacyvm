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
