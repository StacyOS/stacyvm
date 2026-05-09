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
	HTTPClient *http.Client
}

func (c Client) Heartbeat(ctx context.Context, params workerproto.HeartbeatParams) error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("control plane URL is required")
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		return fmt.Errorf("worker id is required")
	}
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("worker token is required")
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
	req.Header.Set("X-Worker-Token", c.Token)

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
