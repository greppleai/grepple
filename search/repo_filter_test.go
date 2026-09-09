package search

import "testing"

// TestRepoFilterMatchesLegacySemantics pins RepoFilter.Allow to the exact
// include/exclude behaviour of the original per-call repoMatches path.
func TestRepoFilterMatchesLegacySemantics(t *testing.T) {
	repos := []string{
		"acme/some-service",
		"acme/other-service",
		"acme/analytics-dbt",
		"globex/widget",
	}
	cases := []struct {
		include, exclude []string
	}{
		{nil, nil},
		{[]string{"acme/*"}, nil},
		{[]string{"acme/some-.*"}, nil},           // regexp form
		{nil, []string{"acme/analytics-dbt"}},     // exclude literal
		{[]string{"acme/*"}, []string{"*/*-dbt"}}, // include + exclude glob
		{[]string{"globex/widget", "acme/other-.*"}, nil},
		{[]string{"nomatch/[unterminated"}, nil}, // bad pattern must not match
	}
	for _, tc := range cases {
		f := NewRepoFilter(tc.include, tc.exclude)
		for _, repo := range repos {
			want := repoMatches(repo, tc.include) && !repoMatchesAny(repo, tc.exclude)
			if got := f.Allow(repo); got != want {
				t.Errorf("Allow(%q) include=%v exclude=%v = %v, legacy = %v",
					repo, tc.include, tc.exclude, got, want)
			}
		}
	}
}

func TestRepoFilterNilAllowsEverything(t *testing.T) {
	var f *RepoFilter
	if !f.Allow("anything/at-all") {
		t.Fatal("nil RepoFilter must allow everything")
	}
}

// BenchmarkRepoFilterAllow shows the reusable filter costs no per-call regexp
// compile, unlike repoMatches.
func BenchmarkRepoFilterAllow(b *testing.B) {
	f := NewRepoFilter([]string{"acme/some-.*"}, nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Allow("acme/some-service")
	}
}

func BenchmarkRepoMatchesLegacy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = repoMatches("acme/some-service", []string{"acme/some-.*"})
	}
}
