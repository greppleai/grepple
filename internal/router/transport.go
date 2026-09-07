package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func httpDo(o routerOptions, method, target string, body any, githubToken string) (int, map[string]any, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, target, rd)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	if githubToken != "" {
		req.Header.Set("x-github-token", githubToken)
	}
	client := http.Client{Timeout: o.timeout}
	resp, e := client.Do(req)
	if e != nil {
		return 502, map[string]any{"error": e.Error()}, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]any{}
	}
	return resp.StatusCode, m, b
}
