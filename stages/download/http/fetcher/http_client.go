package fetcher

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/KKKKKKKEM/flowkit/stages/download"
)

type HttpClient struct {
	mu         sync.Mutex
	transports map[string]*http.Transport
}

func (c *HttpClient) Name() string { return "http-client" }

func (c *HttpClient) Request(ctx context.Context, req *http.Request, opts *download.Opts) (*http.Response, error) {
	client := c.buildClient(opts)
	return client.Do(req.WithContext(ctx))
}

func (c *HttpClient) ensureTransports() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transports == nil {
		c.transports = make(map[string]*http.Transport)
	}
}

func (c *HttpClient) buildClient(opts *download.Opts) *http.Client {
	if opts == nil {
		opts = &download.Opts{}
	}
	transport := c.transportFor(opts.Proxy)
	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
	}
}

func (c *HttpClient) transportFor(proxy string) *http.Transport {
	c.ensureTransports()
	key := proxy
	if key == "" {
		key = "direct"
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if transport, ok := c.transports[key]; ok {
		return transport
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch proxy {
	case "":
		transport.Proxy = nil
	case "env":
		transport.Proxy = http.ProxyFromEnvironment
	default:
		if proxyURL, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			transport.Proxy = nil
		}
	}
	c.transports[key] = transport
	return transport
}

func NewRequest(method, rawURL string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}
