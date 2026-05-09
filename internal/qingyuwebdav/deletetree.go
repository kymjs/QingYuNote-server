package qingyuwebdav

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var hrefTag = regexp.MustCompile(`(?i)<(?:[a-z0-9]+:)?href[^>]*>([^<]+)</(?:[a-z0-9]+:)?href>`)

const propfindBody = `<?xml version="1.0" encoding="utf-8"?><propfind xmlns="DAV:"><allprop/></propfind>`

// DeleteNotesTree 删除 WebDAV 上 notes_dir 指向的整个目录树（如 /42/）。
func DeleteNotesTree(ctx context.Context, baseURL, webdavUser, webdavPass, notesDir string) error {
	root := joinNotesURL(baseURL, notesDir)
	if root == "" {
		return errors.New("empty_notes_url")
	}
	cli := &http.Client{Timeout: 3 * time.Minute}
	return deleteCollectionRecursive(ctx, cli, root, webdavUser, webdavPass)
}

func joinNotesURL(baseURL, notesDir string) string {
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	rel := strings.TrimSpace(notesDir)
	if rel == "" || b == "" {
		return ""
	}
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	return b + rel
}

func deleteCollectionRecursive(ctx context.Context, cli *http.Client, collectionURL, user, pass string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	hrefs, err := propfindDepth1(cli, collectionURL, user, pass)
	if err != nil {
		return err
	}

	base, err := url.Parse(collectionURL)
	if err != nil {
		return err
	}

	childURLs := directChildren(base, hrefs)
	for _, child := range childURLs {
		if strings.HasSuffix(child, "/") {
			if err := deleteCollectionRecursive(ctx, cli, child, user, pass); err != nil {
				return err
			}
		} else {
			if err := doDelete(ctx, cli, child, user, pass); err != nil {
				return err
			}
		}
	}
	return doDelete(ctx, cli, collectionURL, user, pass)
}

func directChildren(base *url.URL, rawHrefs []string) []string {
	pp := strings.TrimSuffix(base.Path, "/")
	out := make([]string, 0, len(rawHrefs))
	seen := map[string]struct{}{}
	for _, raw := range rawHrefs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}
		u := resolveHref(base, ref)
		if u == "" {
			continue
		}
		pu, err := url.Parse(u)
		if err != nil {
			continue
		}
		cp := strings.TrimSuffix(pu.Path, "/")
		if cp == pp {
			continue
		}
		if !strings.HasPrefix(cp, pp+"/") {
			continue
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(cp, pp), "/")
		if rest == "" {
			continue
		}
		if strings.Contains(strings.TrimSuffix(rest, "/"), "/") {
			continue
		}
		normalized := u
		if strings.HasSuffix(ref, "/") && !strings.HasSuffix(normalized, "/") {
			normalized += "/"
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func resolveHref(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	ref := href
	if !strings.HasPrefix(ref, "/") {
		ref = strings.TrimSuffix(base.Path, "/") + "/" + ref
	}
	u := &url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		Path:   ref,
	}
	return u.String()
}

func propfindDepth1(cli *http.Client, rawURL, user, pass string) ([]string, error) {
	req, err := http.NewRequest("PROPFIND", rawURL, strings.NewReader(propfindBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")
	req.SetBasicAuth(user, pass)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("propfind_failed")
	}
	var hrefs []string
	for _, sm := range hrefTag.FindAllStringSubmatch(string(b), -1) {
		if len(sm) > 1 {
			hrefs = append(hrefs, sm[1])
		}
	}
	return hrefs, nil
}

func doDelete(ctx context.Context, cli *http.Client, rawURL, user, pass string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rawURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return errors.New("delete_failed")
}
