package avatarwebdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PutFile 使用 HTTP PUT 将文件上传到 WebDAV 路径 baseURL/filename。
func PutFile(ctx context.Context, baseURL, webdavUser, webdavPass, filename string, body io.Reader, contentType string, size int64) error {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return errors.New("empty_base_url")
	}
	fn := strings.TrimSpace(filename)
	if fn == "" || strings.Contains(fn, "/") || strings.Contains(fn, "\\") {
		return errors.New("bad_filename")
	}
	baseU, err := url.Parse(base)
	if err != nil || baseU.Scheme == "" || baseU.Host == "" {
		return fmt.Errorf("parse_base: %w", err)
	}
	u := baseU.JoinPath(fn)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), body)
	if err != nil {
		return err
	}
	if ct := strings.TrimSpace(contentType); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if size >= 0 {
		req.ContentLength = size
	}
	req.SetBasicAuth(strings.TrimSpace(webdavUser), webdavPass)

	cli := &http.Client{Timeout: 3 * time.Minute}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("put_status_%d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
}
