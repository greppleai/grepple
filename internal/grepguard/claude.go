package grepguard

import "encoding/json"

// HandleClaudeHook also denies Claude's built-in content and filename search tools.
// Bash commands retain the Pi guard's shell-aware classification and escape hatch.
func HandleClaudeHook(input []byte) []byte {
	var payload struct {
		ToolName string `json:"tool_name"`
	}
	if json.Unmarshal(input, &payload) != nil {
		return nil
	}
	switch payload.ToolName {
	case "Grep", "Glob":
		output, err := json.Marshal(HookOutput{HookSpecificOutput: HookSpecificOutput{
			HookEventName: "PreToolUse", PermissionDecision: "deny",
			PermissionDecisionReason: DenialReason(payload.ToolName),
		}})
		if err != nil {
			return nil
		}
		return output
	case "Bash", "": // Empty tool name supports the older Pi hook payload.
		return HandleHook(input)
	default:
		return nil
	}
}
