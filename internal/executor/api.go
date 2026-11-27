package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tyr-zhang/agent-password/internal/store"
)

var safeResponseHeaders = map[string]bool{
	"Content-Type": true, "Content-Length": true, "Date": true,
	"Server": true, "Cache-Control": true, "ETag": true,
}

func (e *Executor) runAPI(ctx context.Context, cred *store.Credential, args map[string]any) (map[string]any, error) {
	if cred.Kind != store.KindAPIKey {
		return nil, errors.New("executor: api requires an apikey credential")
	}
	s := cred.Secret
	if s.BaseURL == "" {
		return nil, errors.New("executor: apikey credential has no base_url")
	}
	path := stringArg(args, "path")
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, errors.New("executor: path must be absolute and must not contain .. or //")
	}
	method := strings.ToUpper(stringArg(args, "method"))
	if method == "" {
		method = "GET"
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
	default:
		return nil, fmt.Errorf("executor: unsupported HTTP method %s", method)
	}
	base, err := url.Parse(s.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, errors.New("executor: invalid base_url")
	}
	if base.Scheme != "https" && base.Scheme != "http" {
		return nil, errors.New("executor: base_url must be http or https")
	}
	target := strings.TrimRight(base.String(), "/") + path
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(stringArg(args, "body")))
	if err != nil {
		return nil, err
	}
	headerName := s.HeaderName
	if headerName == "" {
		headerName = "Authorization"
	}
	lowerHeader := strings.ToLower(headerName)
	for k, v := range argsHeaders(args) {
		lk := strings.ToLower(k)
		if lk == lowerHeader || lk == "authorization" || lk == "cookie" || lk == "host" || lk == "proxy-authorization" {
			return nil, fmt.Errorf("executor: header %s may not be overridden", k)
		}
		req.Header.Set(k, fmt.Sprint(v))
	}
	req.Header.Set(headerName, s.Token)
	client := &http.Client{
		Timeout: e.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !boolArg(args, "allow_redirect") {
				return http.ErrUseLastResponse
			}
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				return errors.New("executor: cross-origin redirect blocked")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executor: HTTP request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxOutputBytes+1))
	if err != nil {
		return nil, err
	}
	text, truncated := capString(string(e.redactor.RedactBytes(body)))
	headers := map[string]string{}
	for k, vals := range resp.Header {
		if safeResponseHeaders[k] {
			headers[k] = e.redactor.Redact(strings.Join(vals, ", "))
		}
	}
	return map[string]any{
		"status": resp.StatusCode, "headers": headers, "body": text, "truncated": truncated,
	}, nil
}

func argsHeaders(args map[string]any) map[string]any {
	v, ok := args["headers"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return v
}

