package goldendb

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

type defaultHTTPClient struct {
	client *http.Client
}

func newDefaultHTTPClient(client *http.Client) *defaultHTTPClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &defaultHTTPClient{client: client}
}

func (c *defaultHTTPClient) Do(req *HTTPRequest) (*HTTPResponse, error) {
	httpReq, err := http.NewRequest(http.MethodGet, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	httpReq.Method = req.Method
	for key, value := range req.Header {
		httpReq.Header.Set(key, value)
	}
	if req.ContentType != "" {
		httpReq.Header.Set("Content-Type", req.ContentType)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}, nil
}

func (c *defaultHTTPClient) DoWithContext(ctx context.Context, req *HTTPRequest) (*HTTPResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for key, value := range req.Header {
		httpReq.Header.Set(key, value)
	}
	if req.ContentType != "" {
		httpReq.Header.Set("Content-Type", req.ContentType)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}, nil
}
