package download

import (
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
		return ""
	}
	base := filepath.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return ""
	}
	return base
}

// FilenameFromHeader 从响应头 Content-Disposition 解析文件名。
// 支持 filename= 和 filename*=（RFC 5987 编码）两种形式，无法解析时返回空字符串。
func FilenameFromHeader(h http.Header) string {
	cd := h.Get("Content-Disposition")
	if cd == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		return ""
	}
	// filename* 优先（RFC 5987，支持非 ASCII）
	if name, ok := params["filename*"]; ok && name != "" {
		return filepath.Base(name)
	}
	if name, ok := params["filename"]; ok && name != "" {
		return filepath.Base(name)
	}
	return ""
}

// ExtFromContentType 从 Content-Type 推断文件扩展名（含点，如 ".zip"）。
// 无法推断或扩展名为空时返回 ""。
func ExtFromContentType(ct string) string {
	if ct == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	exts, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	// mime 包返回的列表按字母序排序，优先选择更常见的扩展名。
	// 对于少数类型有多个扩展名的情况，取第一个（通常已足够）。
	return exts[0]
}
