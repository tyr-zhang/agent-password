package protocol

// ClientInfo is self-reported metadata for audit records. It is never used
// for policy decisions: the client can forge it.
type ClientInfo struct {
	Name string `json:"name"`
	CWD  string `json:"cwd"`
}

// Request is the envelope sent by the CLI to the daemon.
type Request struct {
	Op     string         `json:"op"`
	Args   map[string]any `json:"args,omitempty"`
	Lang   string         `json:"lang,omitempty"`
	Client ClientInfo     `json:"client,omitempty"`
}

// Error describes a machine-readable failure.
type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retry_after,omitempty"`
}

// Response is the envelope returned by the daemon.
type Response struct {
	OK     bool           `json:"ok"`
	Result map[string]any `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

func OK(result map[string]any) Response {
	return Response{OK: true, Result: result}
}

func Fail(code, message string) Response {
	return Response{OK: false, Error: &Error{Code: code, Message: message}}
}

func FailRetry(code, message string, after int) Response {
	return Response{OK: false, Error: &Error{Code: code, Message: message, RetryAfter: after}}
}
