package grepguard

import "encoding/json"

// CommandGuardResponse is the host-neutral response emitted by grepple grep-guard.
type CommandGuardResponse struct {
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason,omitempty"`
}

// HandleCommandGuard classifies one shell command. Invalid input produces no
// response so integrations can fail open rather than accidentally blocking work.
func HandleCommandGuard(input []byte) []byte {
	var request struct {
		Command any `json:"command"`
	}
	if json.Unmarshal(input, &request) != nil {
		return nil
	}
	command, ok := request.Command.(string)
	if !ok {
		return nil
	}

	response := CommandGuardResponse{}
	if hit := FindBlockedInvocation(command); hit != "" {
		response.Blocked = true
		response.Reason = DenialReason(hit)
	}
	output, err := json.Marshal(response)
	if err != nil {
		return nil
	}
	return output
}
