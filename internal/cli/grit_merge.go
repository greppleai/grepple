package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
)

func mergeGritResponses(local, remote api.GritResponse, currentRepo string) (api.GritResponse, error) {
	if err := validateGritResponseMetadata(local.Metadata, remote.Metadata); err != nil {
		return api.GritResponse{}, err
	}
	findings := make([]api.GritFinding, 0, len(local.Findings)+len(remote.Findings))
	findings = append(findings, local.Findings...)
	for _, finding := range remote.Findings {
		if currentRepo == "" || finding.Repo != currentRepo {
			findings = append(findings, finding)
		}
	}
	normalized := normalizeGritFindings(findings)
	merged := api.GritResponse{
		Metadata:    local.Metadata,
		Findings:    normalized,
		Diagnostics: normalizeGritSlice(appendCopy(local.Diagnostics, remote.Diagnostics...)),
		Truncations: normalizeGritSlice(appendCopy(local.Truncations, remote.Truncations...)),
		Statistics:  addGritStatistics(local.Statistics, remote.Statistics),
		ShardErrors: normalizeGritSlice(appendCopy(local.ShardErrors, remote.ShardErrors...)),
	}
	localTotal := max(local.Total, len(local.Findings))
	remoteTotal := max(remote.Total, len(remote.Findings))
	dropped := len(local.Findings) + len(remote.Findings) - len(normalized)
	merged.Total = saturatingAddInt(localTotal, remoteTotal) - dropped
	if merged.Total < len(normalized) {
		merged.Total = len(normalized)
	}
	return merged, nil
}

func validateGritResponseMetadata(local, remote api.GritMetadata) error {
	if local.Compatibility != remote.Compatibility {
		return fmt.Errorf("structural response compatibility mismatch: local %q, remote %q", local.Compatibility, remote.Compatibility)
	}
	if local.GoGrammar != remote.GoGrammar {
		return fmt.Errorf("structural response Go grammar mismatch: local %q, remote %q", local.GoGrammar, remote.GoGrammar)
	}
	if local.Language != remote.Language {
		return fmt.Errorf("structural response language mismatch: local %q, remote %q", local.Language, remote.Language)
	}
	if local.Grammar != remote.Grammar {
		return fmt.Errorf("structural response grammar mismatch: local %q, remote %q", local.Grammar, remote.Grammar)
	}
	return nil
}

func normalizeGritFindings(findings []api.GritFinding) []api.GritFinding {
	for index := range findings {
		findings[index].Path = normalizeGritPath(findings[index].Path)
		if findings[index].Bindings == nil {
			findings[index].Bindings = []api.GritBinding{}
		}
	}
	sort.SliceStable(findings, func(left, right int) bool {
		return compareGritFinding(findings[left], findings[right]) < 0
	})
	result := make([]api.GritFinding, 0, len(findings))
	previous := ""
	for _, finding := range findings {
		key := gritJSONKey(finding)
		if len(result) == 0 || key != previous {
			result = append(result, finding)
			previous = key
		}
	}
	return result
}

func normalizeGritPath(value string) string {
	normalized := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if normalized == "." && value == "" {
		return ""
	}
	return normalized
}

func compareGritFinding(left, right api.GritFinding) int {
	if result := strings.Compare(left.Repo, right.Repo); result != 0 {
		return result
	}
	if result := strings.Compare(left.Path, right.Path); result != 0 {
		return result
	}
	if result := compareGritRange(left.Range, right.Range); result != 0 {
		return result
	}
	if result := strings.Compare(left.PatternID, right.PatternID); result != 0 {
		return result
	}
	return strings.Compare(gritJSONKey(left), gritJSONKey(right))
}

func compareGritRange(left, right api.GritRange) int {
	leftValues := [...]int{left.StartByte, left.EndByte, left.Start.Line, left.Start.Column, left.End.Line, left.End.Column}
	rightValues := [...]int{right.StartByte, right.EndByte, right.Start.Line, right.Start.Column, right.End.Line, right.End.Column}
	for index := range leftValues {
		if leftValues[index] < rightValues[index] {
			return -1
		}
		if leftValues[index] > rightValues[index] {
			return 1
		}
	}
	return 0
}

func gritJSONKey(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode deterministic structural response value: %v", err))
	}
	return string(encoded)
}

func normalizeGritSlice[T any](values []T) []T {
	if values == nil {
		values = []T{}
	}
	sort.SliceStable(values, func(left, right int) bool {
		return gritJSONKey(values[left]) < gritJSONKey(values[right])
	})
	result := make([]T, 0, len(values))
	previous := ""
	for _, value := range values {
		key := gritJSONKey(value)
		if len(result) == 0 || key != previous {
			result = append(result, value)
			previous = key
		}
	}
	return result
}

func appendCopy[T any](values []T, additional ...T) []T {
	result := append([]T(nil), values...)
	return append(result, additional...)
}

func addGritStatistics(left, right api.GritStatistics) api.GritStatistics {
	return api.GritStatistics{
		Candidates:      saturatingAddInt(left.Candidates, right.Candidates),
		Eligible:        saturatingAddInt(left.Eligible, right.Eligible),
		Evaluated:       saturatingAddInt(left.Evaluated, right.Evaluated),
		BytesRead:       saturatingAddInt64(left.BytesRead, right.BytesRead),
		SkippedLanguage: saturatingAddInt(left.SkippedLanguage, right.SkippedLanguage),
		SkippedGlob:     saturatingAddInt(left.SkippedGlob, right.SkippedGlob),
		SkippedBinary:   saturatingAddInt(left.SkippedBinary, right.SkippedBinary),
		SkippedAnchor:   saturatingAddInt(left.SkippedAnchor, right.SkippedAnchor),
	}
}

func saturatingAddInt(left, right int) int {
	maximum := int(^uint(0) >> 1)
	if right > 0 && left > maximum-right {
		return maximum
	}
	return left + right
}

func saturatingAddInt64(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}
