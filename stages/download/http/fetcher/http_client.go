package fetcher

import (
	"context"
	"net/http"
	"net/url"

	"github.com/KKKKKKKEM/flowkit/stages/download"
)

type HttpClient struct{}

func (c *HttpClient) Name() string { return "http-client" }

func (c *HttpClient) Request(ctx context.Context, req *http.Request, opts *download.Opts) (*http.Response, error) {
	client := c.buildClient(opts)
	return client.Do(req.WithContext(ctx))
}

func (c *HttpClient) buildClient(opts *download.Opts) *http.Client {
	if opts == nil {
		opts = &download.Opts{}
	}
	transport := &http.Transport{}

	switch opts.Proxy {
	case "":
		transport.Proxy = nil
	case "env":
		transport.Proxy = http.ProxyFromEnvironment
	default:
		if proxyURL, err := url.Parse(opts.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
	}
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
