package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestRequestNavigationResolveUsesPublicEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/public/navigation/resolve" || request.Method != http.MethodPost {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		var decoded api.NavigationResolveRequest
		if err := json.NewDecoder(request.Body).Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.References) != 1 || decoded.References[0].Version != "v3.5.0" {
			t.Fatalf("request=%#v", decoded)
		}
		_ = json.NewEncoder(writer).Encode(api.NavigationResolveResponse{Results: []api.NavigationResolveResult{{ID: "ctx", Symbols: []api.RelatedSymbol{{Name: "Ctx", Confidence: "dependency-resolved"}}}}})
	}))
	defer server.Close()

	response, err := requestNavigationResolve(context.Background(), api.NavigationResolveRequest{References: []api.ExternalNavigationReference{{ID: "ctx", Version: "v3.5.0"}}}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].ID != "ctx" {
		t.Fatalf("response=%#v", response)
	}
}
