package og

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/config"
)

const (
	envDaemonURL    = "OG_DAEMON_URL"
	envDaemonSocket = "OG_DAEMON_SOCKET"
)

type Client struct {
	base string
	http *http.Client
}

func NewClientFromEnv() Client {
	base, client := daemonHTTPClient()
	return Client{base: base, http: client}
}

func (c Client) Call(path string, req Request) (Response, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		strings.TrimRight(c.base, "/")+path,
		bytes.NewReader(data),
	)
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("daemon call %s: %w", path, err)
	}
	defer httpResp.Body.Close()
	var resp Response
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("decode daemon response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 || !resp.OK {
		if resp.Error == "" {
			resp.Error = httpResp.Status
		}
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

func (c Client) Health() (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.base+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	return resp, nil
}

func SocketPath() string {
	if path := os.Getenv(envDaemonSocket); path != "" {
		return path
	}
	return filepath.Join(config.DefaultConfigDir(), "og.sock")
}

func daemonHTTPClient() (string, *http.Client) {
	if base := os.Getenv(envDaemonURL); base != "" {
		return strings.TrimRight(base, "/"), &http.Client{Timeout: 60 * time.Second}
	}
	socketPath := SocketPath()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return "http://og", &http.Client{Timeout: 60 * time.Second, Transport: transport}
}
