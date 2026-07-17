package insightopen

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var requestDebug bool

func SetDebug(on bool) { requestDebug = on }

var requestHeaders = map[string]string{
	"Content-Type": "application/json",
	"Accept":       "application/json",
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	noVerify   bool
	auth       Auth
}

func NewClient(api string, noVerify bool, auth Auth) (*Client, error) {
	baseURL, err := NormalizeAPIBase(api)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: noVerify}, //nolint:gosec
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
		noVerify: noVerify,
		auth:     auth,
	}, nil
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) RequestJSON(ctx context.Context, method, requestURL string, body any, out any) error {
	candidates := []string{requestURL}
	if strings.HasPrefix(requestURL, "https://") {
		candidates = append(candidates, "http://"+strings.TrimPrefix(requestURL, "https://"))
	}

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body failed: %w", err)
		}
	}

	var lastErr error
	for i, candidate := range candidates {
		err = c.doJSON(ctx, method, candidate, payload, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if i == 0 && shouldRetryPlainHTTP(candidate, err) {
			continue
		}
		return err
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("request failed")
}

func (c *Client) doJSON(ctx context.Context, method, requestURL string, payload []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request failed: %w", err)
	}
	for key, value := range requestHeaders {
		req.Header.Set(key, value)
	}
	if c.auth.Username != "" {
		req.Header.Set("username", c.auth.Username)
	}
	if c.auth.PasswordB64 != "" {
		req.Header.Set("password", c.auth.PasswordB64)
	}

	if requestDebug {
		if len(payload) > 0 {
			log.Printf("[debug] %s %s body: %s", method, requestURL, string(payload))
		} else {
			log.Printf("[debug] %s %s (no body)", method, requestURL)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		if requestDebug {
			log.Printf("[debug] response: HTTP %d: %s", resp.StatusCode, string(raw))
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	if out == nil || len(raw) == 0 {
		return nil
	}
	if requestDebug {
		log.Printf("[debug] response body: %s", string(raw))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode json response failed: %w", err)
	}
	return nil
}

func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	return c.RequestJSON(ctx, http.MethodGet, requestURL, nil, out)
}

func (c *Client) PostJSON(ctx context.Context, path string, body any, out any) error {
	return c.RequestJSON(ctx, http.MethodPost, c.baseURL+path, body, out)
}

func shouldRetryPlainHTTP(candidate string, err error) bool {
	if !strings.HasPrefix(candidate, "https://") {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected eof while reading") ||
		strings.Contains(message, "wrong version number") ||
		strings.Contains(message, "unknown protocol")
}
