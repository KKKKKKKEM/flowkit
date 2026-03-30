package download

import (
	"context"
	"net/http"
	"net/url"
)

type HttpClient struct {
}

func (c *HttpClient) Name() string { return "http-client" }

func (c *HttpClient) Request(ctx context.Context, req *http.Request, opts *Opts) (*http.Response, error) {
	client := c.buildClient(opts)
	return client.Do(req.WithContext(ctx))
}

func (c *HttpClient) buildClient(opts *Opts) *http.Client {
	if opts == nil {
		opts = &Opts{}
	}
	transport := &http.Transport{}

	switch opts.Proxy {
	case "":
		transport.Proxy = nil
	case "env":
		transport.Proxy = http.ProxyFromEnvironment
	default:
		proxyURL, err := url.Parse(opts.Proxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
	}
}

func NewHTTPDownloader() *BaseHTTPDownloader {
	return &BaseHTTPDownloader{
		requester: &HttpClient{},
	}
}
