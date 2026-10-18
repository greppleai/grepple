package initcommand

import (
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func manifestV1() []directorymeta.FileV1 {
	return []directorymeta.FileV1{{Name: "first.go", Checksum: strings.Repeat("a", 64)}, {Name: "second.go", Checksum: strings.Repeat("b", 64)}}
}

func TestParseGeneratedV1EnforcesManifestAndPreservesReviewedTags(t *testing.T) {
	manifest := manifestV1()
	prior := directorymeta.DirectoryV1{Files: []directorymeta.FileV1{{Name: "first.go", Checksum: manifest[0].Checksum, Description: "Reviewed.", Purpose: "Implementation", DomainAreas: []string{"Reviewed"}}}}
	response := "```yaml\nfiles:\n- name: second.go\n  checksum: invented\n  description: Tests source.\n  purpose: Testing\n  keywords: [c++]\n- name: first.go\n  checksum: invented\n  description: Implements source.\n  purpose: Custom domain work\n  domainAreas: [Parser]\n```"
	got, err := parseGeneratedV1(response, manifest, prior)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files[0].Name != "first.go" || got.Files[0].Checksum != manifest[0].Checksum || len(got.Files[0].DomainAreas) != 2 || got.Files[0].Purpose != "custom-domain-work" || got.Files[1].Checksum != manifest[1].Checksum || got.Files[1].Keywords[0] != "c++" {
		t.Fatalf("merged generation: %+v", got)
	}
	manifest[0].Checksum = strings.Repeat("c", 64)
	changed, err := parseGeneratedV1(response, manifest, prior)
	if err != nil || len(changed.Files[0].DomainAreas) != 1 {
		t.Fatalf("stale prior tags survived: %+v: %v", changed, err)
	}
}

func TestParseGeneratedV1RejectsUntrustedMembershipAndUnknownFields(t *testing.T) {
	for _, response := range []string{
		"files: []", // omitted manifest
		"files:\n- name: first.go\n  description: Text.\n  purpose: Testing",                                                                                       // omitted second
		"files:\n- name: first.go\n  description: Text.\n  purpose: Testing\n- name: first.go\n  description: Text.\n  purpose: Testing",                           // duplicate
		"files:\n- name: extra.go\n  description: Text.\n  purpose: Testing",                                                                                       // invented
		"files:\n- name: first.go\n  description: Text.\n  purpose: Testing\n  kind: production",                                                                   // obsolete field
		"description: Directory.\nfiles: []",                                                                                                                       // old format
		"files: []\n---\nfiles: []",                                                                                                                                // multiple docs
		"files:\n- name: first.go\n  description: Text.\n  purpose: Testing\n- name: second.go\n  description: \n  purpose: Testing",                               // missing description
		"files:\n- name: first.go\n  description: Text.\n  purpose: Testing\n- name: second.go\n  description: Text.",                                              // missing purpose
		"files:\n- name: first.go\n  description: Text.\n  purpose: Testing\n  keywords: [\"\\x00\"]\n- name: second.go\n  description: Text.\n  purpose: Testing", // invalid label
	} {
		if _, err := parseGeneratedV1(response, manifestV1(), directorymeta.DirectoryV1{}); err == nil {
			t.Errorf("accepted untrusted response: %q", response)
		}
	}
}
