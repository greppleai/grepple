package router

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// newLogger builds the router's structured logger.
//
// Production logs errors only (see docs/logging.md): the level defaults to
// "error" so info/debug lines are dropped unless explicitly enabled. Raise it
// for debugging with GREPPLE_LOG_LEVEL=debug|info|warn|error. Output is always
// JSON so logs are machine-parseable wherever they are shipped.
func newLogger() *zap.Logger {
	level := zapcore.ErrorLevel
	if v := strings.TrimSpace(os.Getenv("GREPPLE_LOG_LEVEL")); v != "" {
		_ = level.UnmarshalText([]byte(strings.ToLower(v)))
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.DisableCaller = true
	cfg.DisableStacktrace = true
	cfg.Sampling = nil
	cfg.Encoding = "json"
	logger, err := cfg.Build()
	if err != nil {
		return zap.NewNop()
	}
	return logger
}
