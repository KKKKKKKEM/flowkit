package http

import (
	"context"
	"net/http"
	"net/url"

	"github.com/KKKKKKKEM/grasp/pkg/downloader"
)

type SimpleHTTPClient struct {
}

func (c *SimpleHTTPClient) Name() string { return "http-client" }

func (c *SimpleHTTPClient) Request(ctx context.Context, req *http.Request, opts *downloader.Opts) (*http.Response, error) {
	client := c.buildClient(opts)
	return client.Do(req.WithContext(ctx))
}

func (c *SimpleHTTPClient) buildClient(opts *downloader.Opts) *http.Client {
	if opts == nil {
		opts = &downloader.Opts{}
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

func NewSimpleHTTPDownloader() *BaseHTTPDownloader {
	return &BaseHTTPDownloader{
		requester: &SimpleHTTPClient{},
	}
}
