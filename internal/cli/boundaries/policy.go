package boundaries

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/analysis"
)

const defaultBoundaryPolicyPath = ".grepple/boundary-policy.json"

// DefaultPolicyPath is the conventional repository boundary-policy location.
const DefaultPolicyPath = defaultBoundaryPolicyPath

func loadBoundaryPolicy(requested string) (analysis.BoundaryPolicy, string, error) {
	policyPath := requested
	if policyPath == "" {
		policyPath = defaultBoundaryPolicyPath
	}
	content, err := os.ReadFile(policyPath)
	if errors.Is(err, os.ErrNotExist) && requested == "" {
		return analysis.BoundaryPolicy{}, "", nil
	}
	if err != nil {
		return analysis.BoundaryPolicy{}, "", fmt.Errorf("read boundary policy: %w", err)
	}
	var policy analysis.BoundaryPolicy
	if err := json.Unmarshal(content, &policy); err != nil {
		return analysis.BoundaryPolicy{}, "", fmt.Errorf("decode boundary policy: %w", err)
	}
	if policy.Schema == "" {
		return analysis.BoundaryPolicy{}, "", fmt.Errorf("boundary policy schema is required")
	}
	if err := analysis.ValidateBoundaryPolicy(policy); err != nil {
		return analysis.BoundaryPolicy{}, "", err
	}
	return policy, filepath.ToSlash(filepath.Clean(policyPath)), nil
}
