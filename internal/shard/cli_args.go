package shard

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/alexflint/go-arg"
)

var errShardHelp = errors.New("shard help requested")

type shardOptions struct {
	host  string
	port  int
	root  string
	zoekt zoektOptions
}

type shardArgs struct {
	Host       string `arg:"--host" placeholder:"HOST" help:"bind host"`
	Port       int    `arg:"--port" placeholder:"PORT" help:"bind port"`
	Root       string `arg:"--root" placeholder:"DIRECTORY" help:"repository workspace"`
	ZoektIndex string `arg:"--zoekt-index" placeholder:"PATH" help:"Zoekt index directory"`
	ZoektPort  int    `arg:"--zoekt-port" placeholder:"PORT" help:"embedded Zoekt server port"`
	ZoektBin   string `arg:"--zoekt-bin" placeholder:"PATH" help:"Zoekt executable directory"`
}

func (shardArgs) Description() string {
	return "Run a repository search shard."
}

func parseShardOptions(args []string) (shardOptions, error) {
	port, _ := strconv.Atoi(envValue("GREPPLE_PORT", "8787"))
	zoektPort, _ := strconv.Atoi(envValue("GREPPLE_ZOEKT_PORT", "6070"))
	root := envValue("GREPPLE_ROOT", workingDirectory())
	values := shardArgs{
		Host:       envValue("GREPPLE_HOST", "0.0.0.0"),
		Port:       port,
		Root:       root,
		ZoektIndex: envValue("GREPPLE_ZOEKT_INDEX", ""),
		ZoektPort:  zoektPort,
		ZoektBin:   envValue("GREPPLE_ZOEKT_BIN", ""),
	}
	parser, err := arg.NewParser(arg.Config{Program: "shard"}, &values)
	if err != nil {
		return shardOptions{}, err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return shardOptions{}, errShardHelp
		}
		return shardOptions{}, err
	}
	if values.ZoektIndex == "" {
		values.ZoektIndex = defaultZoektIndex(values.Root)
	}
	if values.Port < 0 || values.Port > 65535 {
		return shardOptions{}, fmt.Errorf("--port must be a valid port number")
	}
	if values.ZoektPort <= 0 || values.ZoektPort > 65535 {
		return shardOptions{}, fmt.Errorf("--zoekt-port must be a valid port number")
	}
	return shardOptions{
		host: values.Host,
		port: values.Port,
		root: values.Root,
		zoekt: zoektOptions{
			indexDir: values.ZoektIndex,
			port:     values.ZoektPort,
			binDir:   values.ZoektBin,
		},
	}, nil
}

func defaultZoektIndex(root string) string {
	absolute, err := filepath.Abs(root)
	if err != nil {
		absolute = filepath.Clean(root)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	sum := sha256.Sum256([]byte(absolute))
	return filepath.Join(cache, "grepple", "zoekt", fmt.Sprintf("%x", sum[:8]))
}

func envValue(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func workingDirectory() string {
	directory, err := os.Getwd()
	if err != nil {
		return "."
	}
	return directory
}
