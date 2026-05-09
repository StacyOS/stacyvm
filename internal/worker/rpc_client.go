package worker

import (
	"bufio"
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
	TokenFunc  func() (string, error)
	HTTPClient *http.Client
	RPCTLS     TLSConfig
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

func (c RPCClient) Exec(ctx context.Context, reqID string, params workerproto.ExecParams) (workerproto.ExecResult, error) {
	var result workerproto.ExecResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodExec,
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

func (c RPCClient) ExecStream(ctx context.Context, reqID string, params workerproto.ExecParams) (workerproto.ExecStreamResult, error) {
	var result workerproto.ExecStreamResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodExecStream,
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

func (c RPCClient) ExecStreamLive(ctx context.Context, reqID string, params workerproto.ExecParams) (<-chan workerproto.StreamChunk, <-chan error, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, nil, fmt.Errorf("worker RPC URL is required")
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		return nil, nil, fmt.Errorf("worker id is required")
	}
	token, err := c.authToken()
	if err != nil {
		return nil, nil, err
	}
	body, err := json.Marshal(workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodExecStream,
		WorkerID: c.WorkerID,
		Params:   mustRawMessage(params),
	})
	if err != nil {
		return nil, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/x-ndjson")
	httpReq.Header.Set("X-Worker-Stream", "ndjson")
	httpReq.Header.Set("X-Worker-ID", c.WorkerID)
	httpReq.Header.Set("X-Worker-Token", token)

	client, err := c.httpClient(&http.Client{})
	if err != nil {
		return nil, nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var response workerproto.Response
		if len(data) > 0 {
			_ = json.Unmarshal(data, &response)
		}
		if response.Error != "" {
			return nil, nil, fmt.Errorf("worker RPC failed: HTTP %d: %s", resp.StatusCode, response.Error)
		}
		return nil, nil, fmt.Errorf("worker RPC failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	chunks := make(chan workerproto.StreamChunk, 64)
	errs := make(chan error, 1)
	go func() {
		defer resp.Body.Close()
		defer close(chunks)
		defer close(errs)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			var response workerproto.Response
			if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
				errs <- fmt.Errorf("decode worker stream response: %w", err)
				return
			}
			if response.Error != "" {
				errs <- fmt.Errorf("worker RPC stream failed: %s", response.Error)
				return
			}
			if len(response.Result) == 0 {
				continue
			}
			var chunk workerproto.StreamChunk
			if err := json.Unmarshal(response.Result, &chunk); err != nil {
				errs <- fmt.Errorf("decode worker stream chunk: %w", err)
				return
			}
			select {
			case chunks <- chunk:
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errs <- err
		}
	}()
	return chunks, errs, nil
}

func (c RPCClient) FileWrite(ctx context.Context, reqID string, params workerproto.FileParams) error {
	_, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileWrite,
		WorkerID: c.WorkerID,
		Params:   mustRawMessage(params),
	})
	return err
}

func (c RPCClient) FileRead(ctx context.Context, reqID string, params workerproto.FileParams) (workerproto.FileReadResult, error) {
	var result workerproto.FileReadResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileRead,
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

func (c RPCClient) FileList(ctx context.Context, reqID string, params workerproto.FileParams) (workerproto.FileListResult, error) {
	var result workerproto.FileListResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileList,
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

func (c RPCClient) FileDelete(ctx context.Context, reqID string, params workerproto.FileParams) error {
	_, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileDelete,
		WorkerID: c.WorkerID,
		Params:   mustRawMessage(params),
	})
	return err
}

func (c RPCClient) FileMove(ctx context.Context, reqID string, params workerproto.FileParams) error {
	_, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileMove,
		WorkerID: c.WorkerID,
		Params:   mustRawMessage(params),
	})
	return err
}

func (c RPCClient) FileChmod(ctx context.Context, reqID string, params workerproto.FileParams) error {
	_, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileChmod,
		WorkerID: c.WorkerID,
		Params:   mustRawMessage(params),
	})
	return err
}

func (c RPCClient) FileStat(ctx context.Context, reqID string, params workerproto.FileParams) (workerproto.FileStatResult, error) {
	var result workerproto.FileStatResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileStat,
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

func (c RPCClient) FileGlob(ctx context.Context, reqID string, params workerproto.FileParams) (workerproto.FileGlobResult, error) {
	var result workerproto.FileGlobResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodFileGlob,
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

func (c RPCClient) Logs(ctx context.Context, reqID string, params workerproto.LogsParams) (workerproto.LogsResult, error) {
	var result workerproto.LogsResult
	resp, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodLogs,
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

func (c RPCClient) Shutdown(ctx context.Context, reqID string) error {
	_, err := c.call(ctx, workerproto.Request{
		ID:       reqID,
		Method:   workerproto.MethodShutdown,
		WorkerID: c.WorkerID,
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
	token, err := c.authToken()
	if err != nil {
		return zero, err
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
	httpReq.Header.Set("X-Worker-Token", token)

	client, err := c.httpClient(&http.Client{Timeout: 30 * time.Second})
	if err != nil {
		return zero, err
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

func (c RPCClient) httpClient(defaultClient *http.Client) (*http.Client, error) {
	if c.HTTPClient != nil {
		return c.HTTPClient, nil
	}
	return c.RPCTLS.HTTPClient(defaultClient)
}

func (c RPCClient) authToken() (string, error) {
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

func mustRawMessage(value interface{}) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
