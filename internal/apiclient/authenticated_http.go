package apiclient

import (
	"fmt"
	"net/http"
)

// doRequest preserves ordinary redirect behavior without forwarding credentials
// to a different origin, including a different port on the same hostname.
func (client *apiClient) doRequest(request *http.Request) (*http.Response, error) {
	if request.Header.Get("Authorization") == "" {
		return client.httpClient.Do(request)
	}
	transport := *client.httpClient
	redirect := transport.CheckRedirect
	transport.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if next.URL.Scheme != request.URL.Scheme || next.URL.Host != request.URL.Host {
			return http.ErrUseLastResponse
		}
		if redirect != nil {
			return redirect(next, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return transport.Do(request)
}
