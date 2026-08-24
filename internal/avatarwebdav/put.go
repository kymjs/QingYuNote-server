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

// PutStatusError WebDAV 返回非 2xx 时使用，便于 API 层映射为细分错误码。
type PutStatusError struct {
	StatusCode      int
	TargetURL       string
	ResponseHeaders string
	Body            string
}

func (e *PutStatusError) Error() string {
	if e == nil {
		return ""
	}
	details := make([]string, 0, 2)
	if e.TargetURL != "" {
		details = append(details, "target="+e.TargetURL)
	}
	if e.ResponseHeaders != "" {
		details = append(details, "response_headers="+e.ResponseHeaders)
	}
	if e.Body != "" {
		details = append(details, e.Body)
	}
	if len(details) == 0 {
		return fmt.Sprintf("put_status_%d", e.StatusCode)
	}
	return fmt.Sprintf("put_status_%d: %s", e.StatusCode, strings.Join(details, "; "))
}

// diagnosticTargetURL 保留请求目标，移除 URL 用户信息与查询参数，避免日志泄露凭据或令牌。
func diagnosticTargetURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	safe := *u
	safe.User = nil
	safe.RawQuery = ""
	safe.ForceQuery = false
	return safe.String()
}

// diagnosticResponseHeaders 只保留 WebDAV 排障所需、不会承载会话信息的响应头。
func diagnosticResponseHeaders(h http.Header) string {
	const maxValueLength = 512
	names := []string{"Server", "Allow", "DAV", "WWW-Authenticate", "Content-Type"}
	parts := make([]string, 0, len(names))
	for _, name := range names {
		value := strings.TrimSpace(h.Get(name))
		if value == "" {
			continue
		}
		if len(value) > maxValueLength {
			value = value[:maxValueLength] + "…"
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, ",")
}

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
	return &PutStatusError{
		StatusCode:      resp.StatusCode,
		TargetURL:       diagnosticTargetURL(u),
		ResponseHeaders: diagnosticResponseHeaders(resp.Header),
		Body:            strings.TrimSpace(string(b)),
	}
}
