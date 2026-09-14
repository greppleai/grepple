package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/search"
)

const defaultBoundaryPolicyPath = ".grepple/boundary-policy.json"

func loadBoundaryPolicy(requested string) (search.BoundaryPolicy, string, error) {
	policyPath := requested
	if policyPath == "" {
		policyPath = defaultBoundaryPolicyPath
	}
	content, err := os.ReadFile(policyPath)
	if errors.Is(err, os.ErrNotExist) && requested == "" {
		return search.BoundaryPolicy{}, "", nil
	}
	if err != nil {
		return search.BoundaryPolicy{}, "", fmt.Errorf("read boundary policy: %w", err)
	}
	var policy search.BoundaryPolicy
	if err := json.Unmarshal(content, &policy); err != nil {
		return search.BoundaryPolicy{}, "", fmt.Errorf("decode boundary policy: %w", err)
	}
	if policy.Schema == "" {
		return search.BoundaryPolicy{}, "", fmt.Errorf("boundary policy schema is required")
	}
	if err := search.ValidateBoundaryPolicy(policy); err != nil {
		return search.BoundaryPolicy{}, "", err
	}
	return policy, filepath.ToSlash(filepath.Clean(policyPath)), nil
}
