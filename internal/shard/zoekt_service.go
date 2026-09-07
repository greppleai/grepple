package shard

import (
	"grepple/internal/api"
	"path/filepath"
	"sync"
)

type zoektService interface {
	index(api.RepoInfo) error
	remove(string) error
	running() bool
	upToDate(api.RepoInfo) bool
	setCoverage(string, zoektCoverage)
	shardExists(string) bool
	coverageFor(string) (zoektCoverage, bool)
	primeCoverage([]api.RepoInfo)
	start() error
	stop()
	done() chan struct{}
	err() error
	indexedHead(string) (string, bool)
	repoCounts(string, []api.RepoInfo, []string) ([]api.RepoCount, bool, error)
	candidates(string, string, []api.RepoInfo) ([]string, bool, error)
}

func newZoektService(options zoektOptions, directory *directory) zoektService {
	return &zoektServiceImpl{
		options:   options,
		coverage:  map[string]zoektCoverage{},
		directory: directory,
	}
}

//grepple:filelocal
type zoektServiceImpl struct {
	options   zoektOptions
	process   *zoektProcess
	indexMux  sync.Mutex
	coverage  map[string]zoektCoverage
	directory *directory
}

func (service *zoektServiceImpl) index(info api.RepoInfo) error {
	service.indexMux.Lock()
	defer service.indexMux.Unlock()
	if service.coverage == nil {
		service.coverage = map[string]zoektCoverage{}
	}
	if info.Head == nil || *info.Head == "" {
		if err := removeZoektRepo(service.options, info.Repo); err != nil {
			return err
		}
		service.coverage[info.Repo] = zoektCoverage{known: true}
		service.directory.remove(info.Repo)
		return nil
	}
	if err := zoektIndex(service.options, info.Dir); err != nil {
		return err
	}
	coverage := inspectZoektCoverage(info.Dir)
	service.coverage[info.Repo] = coverage
	_ = service.directory.set(info.Repo, *info.Head)
	if coverage.indexed && service.running() {
		return waitForZoektVersion(service.options, info.Repo, *info.Head)
	}
	return nil
}

func (service *zoektServiceImpl) remove(repo string) error {
	service.indexMux.Lock()
	defer service.indexMux.Unlock()
	err := removeZoektRepo(service.options, repo)
	delete(service.coverage, repo)
	service.directory.remove(repo)
	return err
}

func (service *zoektServiceImpl) running() bool {
	return service != nil && service.process.running()
}

func (service *zoektServiceImpl) upToDate(info api.RepoInfo) bool {
	if info.Head == nil || *info.Head == "" {
		return false
	}
	indexedHead, ok := service.directory.head(info.Repo)
	return ok && indexedHead == *info.Head && service.shardExists(info.Repo)
}

func (service *zoektServiceImpl) setCoverage(repo string, coverage zoektCoverage) {
	service.indexMux.Lock()
	defer service.indexMux.Unlock()
	if service.coverage == nil {
		service.coverage = map[string]zoektCoverage{}
	}
	service.coverage[repo] = coverage
}

func (service *zoektServiceImpl) shardExists(repo string) bool {
	matches, err := filepath.Glob(filepath.Join(service.options.indexDir, zoektShardPrefix(repo)+"_v*.zoekt"))
	return err == nil && len(matches) > 0
}

func (service *zoektServiceImpl) coverageFor(repo string) (zoektCoverage, bool) {
	service.indexMux.Lock()
	defer service.indexMux.Unlock()
	coverage, ok := service.coverage[repo]
	return coverage, ok
}

func (service *zoektServiceImpl) primeCoverage(repos []api.RepoInfo) {
	service.indexMux.Lock()
	defer service.indexMux.Unlock()
	if service.coverage == nil {
		service.coverage = map[string]zoektCoverage{}
	}
	for _, repo := range repos {
		service.coverage[repo.Repo] = zoektCoverage{known: true}
	}
}

func (service *zoektServiceImpl) start() error {
	process, err := startZoekt(service.options)
	if err != nil {
		return err
	}
	service.process = process
	return nil
}

func (service *zoektServiceImpl) stop() {
	service.process.stop()
}

func (service *zoektServiceImpl) done() chan struct{} {
	if service == nil || service.process == nil {
		return nil
	}
	return service.process.done
}

func (service *zoektServiceImpl) err() error {
	if service == nil {
		return nil
	}
	return service.process.err()
}

func (service *zoektServiceImpl) indexedHead(repo string) (string, bool) {
	if service == nil || service.directory == nil {
		return "", false
	}
	return service.directory.head(repo)
}

func (service *zoektServiceImpl) repoCounts(q string, repos []api.RepoInfo, globs []string) ([]api.RepoCount, bool, error) {
	return zoektRepoCounts(service.options, q, repos, globs)
}

func (service *zoektServiceImpl) candidates(q, root string, repos []api.RepoInfo) ([]string, bool, error) {
	return zoektCandidates(service.options, q, root, repos)
}
