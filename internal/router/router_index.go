package router

import (
	"encoding/json"
	"grepple/internal/api"
	"net/http"
	"net/url"
)

type indexedRepo struct {
	api.RepoInfo
	Shard string `json:"shard"`
}

func listIndexed(o routerOptions) []indexedRepo {
	ch := make(chan []indexedRepo, len(o.backends))
	for _, b := range o.backends {
		go func(base string) {
			resp, e := (&http.Client{Timeout: o.timeout}).Get(base + "/index")
			if e != nil {
				ch <- nil
				return
			}
			defer resp.Body.Close()
			var x api.IndexListResponse
			if resp.StatusCode < 300 {
				json.NewDecoder(resp.Body).Decode(&x)
			}
			out := make([]indexedRepo, len(x.Repos))
			for i, r := range x.Repos {
				out[i] = indexedRepo{r, base}
			}
			ch <- out
		}(b)
	}
	var out []indexedRepo
	for range o.backends {
		out = append(out, (<-ch)...)
	}
	return out
}

func indexShard(o routerOptions, shard, method string, ref api.RepoRef) (int, map[string]any) {
	target := shard + "/index"
	if method == "GET" {
		target += "?repository=" + url.QueryEscape(ref.Repo)
	}
	// Clone/pull operations need a GitHub token; resolve the App installation
	// token for the repo's org, falling back to the static PAT.
	token := ""
	if method == http.MethodPost || method == http.MethodPut {
		token = o.auth.tokenForRepo(ref.Repo)
	}
	status, m, _ := httpDo(o, method, target, func() any {
		if method == "GET" {
			return nil
		}
		return ref
	}(), token)
	return status, m
}
