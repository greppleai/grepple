package anchor

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEmptyAnchorRequestUsesJSONArrays(t *testing.T) {
	request, _, err := buildRequest(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"protocol_version":1,"files":[]}` {
		t.Fatalf("empty request = %s", encoded)
	}
}

func TestNativeLookupUsesBuiltInHashline(t *testing.T) {
	request, paths, err := buildRequest([]File{{Path: "/tmp/source", DisplayPath: "source.txt", Content: "alpha\nbeta\n", Lines: []int{1, 2, 3}}})
	if err != nil {
		t.Fatal(err)
	}
	lookup, err := nativeLookup(request, paths)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[int]string{1: "VAS", 2: "nF3", 3: "Asx"}
	if !reflect.DeepEqual(lookup["source.txt"], expected) {
		t.Fatalf("lookup=%#v expected=%#v", lookup, expected)
	}
}

func TestResponseRejectsMissingAndUnsafeAnchors(t *testing.T) {
	request := protocolRequest{ProtocolVersion: 1, Files: []protocolRequestFile{{Path: "/tmp/a", SHA256: "digest", Lines: []int{1}}}}
	paths := map[string]string{"/tmp/a": "a"}
	for _, value := range []string{"", "bad│anchor", "bad\nanchor"} {
		response := protocolResponse{ProtocolVersion: 1, Files: []protocolResponseFile{{Path: "/tmp/a", SHA256: "digest", Anchors: []protocolLine{{Line: 1, Anchor: value}}}}}
		if _, err := validateResponse(request, response, paths); err == nil {
			t.Fatalf("unsafe anchor %q was accepted", value)
		}
	}
}

func TestResponseRejectsDuplicateAnchorsWithinFile(t *testing.T) {
	request := protocolRequest{ProtocolVersion: 1, Files: []protocolRequestFile{{Path: "/tmp/a", SHA256: "digest", Lines: []int{1, 2}}}}
	response := protocolResponse{ProtocolVersion: 1, Files: []protocolResponseFile{{
		Path: "/tmp/a", SHA256: "digest", Anchors: []protocolLine{{Line: 1, Anchor: "same"}, {Line: 2, Anchor: "same"}},
	}}}
	if _, err := validateResponse(request, response, map[string]string{"/tmp/a": "a"}); err == nil {
		t.Fatal("duplicate anchors were accepted")
	}
}
