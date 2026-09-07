package repository

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func runGit(args []string, cwd, token string) (string, string, int) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if token != "" {
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraheader", "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+basic)
	}
	var out, er strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &er
	e := cmd.Run()
	code := 0
	if e != nil {
		code = 1
		if x, ok := e.(*exec.ExitError); ok {
			code = x.ExitCode()
		}
	}
	return out.String(), er.String(), code
}

func git(args []string, cwd, token string) (string, error) {
	o, e, c := runGit(args, cwd, token)
	if c != 0 {
		return "", fmt.Errorf("git %s failed (%d): %s", strings.Join(args, " "), c, strings.TrimSpace(firstNonempty(e, o)))
	}
	return o, nil
}

func firstNonempty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func gitValue(dir string, args ...string) *string {
	o, _, c := runGit(append([]string{"-C", dir}, args...), dir, "")
	if c != 0 {
		return nil
	}
	x := strings.TrimSpace(o)
	return &x
}
