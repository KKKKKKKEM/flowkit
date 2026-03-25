package core

import (
	"net/url"
	"path/filepath"
)

func (t *DownloadTask) StringMeta(key string) (string, bool) {
	v, ok := t.Meta[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func (t *DownloadTask) Int64Meta(key string) (int64, bool) {
	v, ok := t.Meta[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

func (t *DownloadTask) FilenameFromURL() string {
	u, err := url.Parse(t.URL)
	if err != nil {
		return "download"
	}
	base := filepath.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return "download"
	}
	return base
}
