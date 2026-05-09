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

type RPCClient struct {
	BaseURL    string
	WorkerID   string
	Token      string
	HTTPClient *http.Client
}

func (c RPCClient) Spawn(ctx context.Context, reqID string, lease workerproto.LeaseToken, params workerproto.SpawnParams) (workerproto.SpawnResult, error) {
	var result workerproto.SpawnResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodSpawn,
		WorkerID: c.WorkerID,
		Lease:    &lease,
		Params:   mustRawMessage(params),
	})
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c RPCClient) Status(ctx context.Context, reqID string, params workerproto.StatusParams) (workerproto.StatusResult, error) {
	var result workerproto.StatusResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodStatus,
		WorkerID: c.WorkerID,
		Params:   mustRawMessage(params),
	})
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c RPCClient) Destroy(ctx context.Context, reqID string, lease workerproto.LeaseToken, params workerproto.DestroyParams) error {
	_, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodDestroy,
		WorkerID: c.WorkerID,
		Lease:    &lease,
		Params:   mustRawMessage(params),
	})
	return err
}

func (c RPCClient) call(ctx context.Context, request workerproto.Request) (workerproto.Response, error) {
	var zero workerproto.Response
	if strings.TrimSpace(c.BaseURL) == "" {
		return zero, fmt.Errorf("worker RPC URL is required")
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		return zero, fmt.Errorf("worker id is required")
	}
	if strings.TrimSpace(c.Token) == "" {
		return zero, fmt.Errorf("worker token is required")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return zero, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/rpc", bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Worker-ID", c.WorkerID)
	httpReq.Header.Set("X-Worker-Token", c.Token)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return zero, err
	}
	defer httpResp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	var response workerproto.Response
	if len(data) > 0 {
		if err := json.Unmarshal(data, &response); err != nil {
			return zero, fmt.Errorf("decode worker RPC response: %w", err)
		}
	}
	if httpResp.StatusCode >= 300 {
		if response.Error != "" {
			return zero, fmt.Errorf("worker RPC failed: HTTP %d: %s", httpResp.StatusCode, response.Error)
		}
		return zero, fmt.Errorf("worker RPC failed: HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(data)))
	}
	if response.Error != "" {
		return zero, fmt.Errorf("worker RPC failed: %s", response.Error)
	}
	return response, nil
}

func mustRawMessage(value interface{}) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
