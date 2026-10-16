package pihooks

import "github.com/greppleai/grepple/internal/grepguard"

// AllowMarker permits an explicit exception in a shell command.
const AllowMarker = grepguard.AllowMarker

// HookOutput is the Claude-compatible structured denial response.
type HookOutput = grepguard.HookOutput

// HookSpecificOutput contains the permission decision and reason.
type HookSpecificOutput = grepguard.HookSpecificOutput

// FindBlockedInvocation shares the shell-aware classifier with Claude Code.
func FindBlockedInvocation(command string) string { return grepguard.FindBlockedInvocation(command) }

// DenialReason describes the Grepple alternatives for a blocked command.
func DenialReason(hit string) string { return grepguard.DenialReason(hit) }

// HandleHook preserves the Pi-compatible pre-tool hook API.
func HandleHook(input []byte) []byte { return grepguard.HandleHook(input) }
