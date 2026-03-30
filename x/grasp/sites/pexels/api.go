package pexels

import (
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/KKKKKKKEM/flowkit/stages/download"
	"github.com/KKKKKKKEM/flowkit/stages/extract"
	"github.com/tidwall/gjson"
)

type APIParser struct {
}

func (p *APIParser) Name() string {
	return "pexels-api-parser"
}

func (p *APIParser) Handlers() []*extract.Parser {
	return []*extract.Parser{
		{
			Pattern:  regexp.MustCompile(`^https://api\.pexels\.com/v1/photos/(\d+)$`),
			Priority: 0,
			Hint:     "pexels image API parser",
			Parse:    p.ParseImageAPI,
		},
	}
}

func (p *APIParser) ParseImageAPI(ctx context.Context, task *extract.Task, opts *extract.Opts) ([]extract.Item, error) {

	httpClient := &download.HttpClient{}
	request, err := download.NewRequest("GET", task.URL, task.Headers)
	if err != nil {
		return nil, err
	}

	response, err := httpClient.Request(ctx, request, opts.ToDownloaderOpts())
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(response.Body)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	alt := gjson.GetBytes(body, "alt").String()
	width := gjson.GetBytes(body, "width").Int()
	height := gjson.GetBytes(body, "height").Int()
	url := gjson.GetBytes(body, "url").String()
	id := gjson.GetBytes(body, "id").Int()

	var items []extract.Item
	gjson.GetBytes(body, "src").ForEach(func(key, value gjson.Result) bool {
		items = append(items, extract.Item{
			Name:     fmt.Sprintf("%s (%s)", alt, key.String()),
			URI:      value.String(),
			IsDirect: true,
			Meta: map[string]any{
				"id":     id,
				"width":  width,
				"height": height,
				"url":    url,
			},
		})
		return true
	})

	return items, nil
}
