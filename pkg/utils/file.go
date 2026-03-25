package utils

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func FileExistsAt(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func IsDirPath(dest string) bool {
	if strings.HasSuffix(dest, "/") || strings.HasSuffix(dest, "\\") {
		return true
	}

	base := filepath.Base(dest)
	return filepath.Ext(base) == ""
}
func FilenameFromURL(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uuid.NewString()
	}
	base := filepath.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return uuid.NewString()
	}
	return base
}
