package shard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"grepple/internal/api"
	"grepple/internal/repository"

	"github.com/gofiber/fiber/v3"
)

type shardController struct {
	state Service
}

func (controller *shardController) register(app *fiber.App) {
	app.Post("/search", controller.handleSearch)
	app.All("/index", controller.handleIndex)
	app.Get("/raw", controller.handleRaw)
	app.Get("/tree", controller.handleTree)
}

func (controller *shardController) handleSearch(c fiber.Ctx) error {
	var request api.SearchRequest
	if err := json.Unmarshal(c.Body(), &request); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	response, err := controller.state.Search(request, c.Get("X-Request-Id"))
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	return writeJSON(c, fiber.StatusOK, response)
}

func (controller *shardController) handleIndex(c fiber.Ctx) error {
	if c.Method() == fiber.MethodGet {
		return controller.handleIndexGet(c)
	}
	ref, err := readRepoRef(c)
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	token := c.Get("x-github-token")
	switch c.Method() {
	case fiber.MethodPost:
		return controller.indexCreate(c, ref, token)
	case fiber.MethodPut:
		return controller.indexRefresh(c, ref, token)
	case fiber.MethodDelete:
		return controller.indexRemove(c, ref)
	default:
		return errorJSON(c, fiber.StatusMethodNotAllowed, "method not allowed")
	}
}

func (controller *shardController) handleIndexGet(c fiber.Ctx) error {
	repo := c.Query("repo", c.Query("repository"))
	if repo == "" {
		return writeJSON(c, fiber.StatusOK, api.IndexListResponse{Repos: controller.state.ListRepos()})
	}
	if info, ok := controller.state.GetRepo(repo); ok {
		return writeJSON(c, fiber.StatusOK, info)
	}
	return errorJSON(c, fiber.StatusNotFound, "repository not indexed: "+repo)
}

func (controller *shardController) indexCreate(c fiber.Ctx, ref api.RepoRef, token string) error {
	info, err := controller.state.CreateRepo(ref, token)
	if err != nil {
		return errorJSON(c, statusFor(err, fiber.StatusBadRequest), err)
	}
	return writeJSON(c, fiber.StatusCreated, info)
}

func (controller *shardController) indexRefresh(c fiber.Ctx, ref api.RepoRef, token string) error {
	info, err := controller.state.RefreshRepo(ref, token)
	if err != nil {
		return errorJSON(c, statusFor(err, fiber.StatusBadRequest), err)
	}
	return writeJSON(c, fiber.StatusOK, info)
}

func (controller *shardController) indexRemove(c fiber.Ctx, ref api.RepoRef) error {
	removed, err := controller.state.RemoveRepo(ref.Repo)
	if err != nil {
		return errorJSON(c, statusFor(err, fiber.StatusBadRequest), err)
	}
	return writeJSON(c, fiber.StatusOK, fiber.Map{"repo": ref.Repo, "removed": removed})
}

func (controller *shardController) handleRaw(c fiber.Ctx) error {
	repo, path := c.Query("repo"), c.Query("path")
	if repo == "" || path == "" {
		return errorJSON(c, fiber.StatusBadRequest, "raw requires 'repo' and 'path'")
	}
	info, ok := controller.state.GetRepo(repo)
	if !ok {
		return errorJSON(c, fiber.StatusNotFound, "repository not indexed: "+repo)
	}
	absolute, ok := repository.SafeRepoFilePath(info.Dir, path)
	if !ok {
		return errorJSON(c, fiber.StatusBadRequest, "invalid path")
	}
	contentBytes, err := os.ReadFile(absolute)
	if err != nil {
		return errorJSON(c, fiber.StatusNotFound, "file not found: "+path)
	}
	relative, _ := filepath.Rel(info.Dir, absolute)
	content := sliceLineRange(string(contentBytes), c.Query("start"), c.Query("end"))
	c.Set("x-grepple-repo", repo)
	c.Set("x-grepple-path", filepath.ToSlash(relative))
	if info.Head != nil {
		c.Set("x-grepple-commit", *info.Head)
	}
	if c.Query("format") == "json" {
		return writeJSON(c, fiber.StatusOK, fiber.Map{"repo": repo, "path": filepath.ToSlash(relative), "commit": info.Head, "content": content})
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
	return c.SendString(content)
}

func (controller *shardController) handleTree(c fiber.Ctx) error {
	repo := c.Query("repo")
	if repo == "" {
		return errorJSON(c, fiber.StatusBadRequest, "tree requires 'repo'")
	}
	info, ok := controller.state.GetRepo(repo)
	if !ok {
		return errorJSON(c, fiber.StatusNotFound, "repository not indexed: "+repo)
	}
	requested := c.Query("path")
	base := info.Dir
	if requested != "" && requested != "." && requested != "/" {
		var safe bool
		base, safe = repository.SafeRepoFilePath(info.Dir, requested)
		if !safe {
			return errorJSON(c, fiber.StatusBadRequest, "invalid path")
		}
	}
	stat, err := os.Stat(base)
	if err != nil || !stat.IsDir() {
		return errorJSON(c, fiber.StatusNotFound, "not a directory: "+requested)
	}
	depth := 2
	if number, err := strconv.Atoi(c.Query("depth")); err == nil && number >= 1 {
		depth = number
	}
	relative, _ := filepath.Rel(info.Dir, base)
	if relative == "." {
		relative = "."
	}
	return writeJSON(c, fiber.StatusOK, api.TreeResponse{
		Repo: repo, Path: filepath.ToSlash(relative), Depth: depth,
		Commit: info.Head, Entries: repository.WalkTree(base, depth),
	})
}

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

func sliceLineRange(content, startParam, endParam string) string {
	if startParam == "" && endParam == "" {
		return content
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	from, to := 1, len(lines)
	if number, err := strconv.Atoi(startParam); err == nil && number >= 1 {
		from = number
	}
	if number, err := strconv.Atoi(endParam); err == nil && number >= 1 {
		to = number
	}
	from = max(1, min(from, len(lines)+1))
	to = max(0, min(to, len(lines)))
	if from-1 <= to {
		return strings.Join(lines[from-1:to], "\n")
	}
	return ""
}
