package router

import (
	"encoding/json"
	"fmt"
	"grepple/internal/api"

	"github.com/gofiber/fiber/v3"
)

func readRepoRef(c fiber.Ctx) (api.RepoRef, error) {
	var ref api.RepoRef
	if c.Method() != fiber.MethodGet && len(c.Body()) > 0 {
		if err := json.Unmarshal(c.Body(), &ref); err != nil {
			return ref, fmt.Errorf("invalid JSON body")
		}
	}
	if ref.Repo == "" {
		ref.Repo = c.Query("repo", c.Query("repository"))
	}
	if ref.URL == "" {
		ref.URL = c.Query("url")
	}
	if ref.Ref == "" {
		ref.Ref = c.Query("ref")
	}
	if ref.Repo == "" {
		return ref, fmt.Errorf("missing 'repo'")
	}
	return ref, nil
}
