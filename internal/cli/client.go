package cli

import (
	"encoding/json"
	"os"
	"time"

	"github.com/tyr-zhang/agent-password/internal/i18n"
	"github.com/tyr-zhang/agent-password/internal/platform"
	"github.com/tyr-zhang/agent-password/internal/protocol"
)

type CallError struct {
	Code       string
	Message    string
	RetryAfter int
}

func (e *CallError) Error() string { return e.Message }

func (e *CallError) ExitCode() int { return protocol.ExitCode(e.Code) }

// Call sends one request to the daemon. The CLI never starts a daemon itself.
func Call(op string, args map[string]any) (map[string]any, *CallError) {
	conn, err := platform.Current().Dial()
	if err != nil {
		return nil, &CallError{Code: protocol.CodeNoDaemon,
			Message: i18n.T(currentLang, "daemon is not running; start it with `agent-pwd daemon` (exit code 20)")}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(6 * time.Minute))
	cwd, _ := os.Getwd()
	name := "agent-pwd"
	if len(os.Args) > 0 {
		name = os.Args[0]
	}
	req := protocol.Request{
		Op: op, Args: args, Lang: string(currentLang),
		Client: protocol.ClientInfo{Name: platformDescribe(name), CWD: cwd},
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, &CallError{Code: protocol.CodeInternal, Message: err.Error()}
	}
	if err := protocol.WriteFrame(conn, payload); err != nil {
		return nil, &CallError{Code: protocol.CodeInternal, Message: err.Error()}
	}
	reply, err := protocol.ReadFrame(conn)
	if err != nil {
		return nil, &CallError{Code: protocol.CodeInternal, Message: i18n.T(currentLang, "daemon response: %v", err)}
	}
	var resp protocol.Response
	if err := json.Unmarshal(reply, &resp); err != nil {
		return nil, &CallError{Code: protocol.CodeInternal, Message: i18n.T(currentLang, "invalid daemon response")}
	}
	if !resp.OK && resp.Error != nil {
		return nil, &CallError{Code: resp.Error.Code, Message: resp.Error.Message, RetryAfter: resp.Error.RetryAfter}
	}
	return resp.Result, nil
}

func platformDescribe(s string) string {
	// Keep the audit client field readable and free of control characters.
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r < 32 || r == 127 {
			continue
		}
		out = append(out, r)
		if len(out) >= 160 {
			break
		}
	}
	return string(out)
}

