package protocol

// Error codes with their CLI exit-code contract.
const (
	CodeOK        = "OK"
	CodePending   = "PENDING"
	CodeLocked    = "LOCKED"
	CodeNoDaemon  = "NO_DAEMON"
	CodeDenied    = "DENIED"
	CodeNotFound  = "NOT_FOUND"
	CodeBadArgs   = "BAD_ARGS"
	CodeExists    = "EXISTS"
	CodeCorrupt   = "CORRUPT"
	CodeTimeout   = "TIMEOUT"
	CodeBiometric = "BIOMETRIC_REQUIRED"
	CodeInternal  = "INTERNAL"
)

// ExitCode maps an error code to the documented CLI exit code.
func ExitCode(code string) int {
	switch code {
	case CodeOK:
		return 0
	case CodePending:
		return 10
	case CodeLocked, CodeNoDaemon:
		return 20
	case CodeDenied:
		return 30
	case CodeNotFound:
		return 40
	default:
		return 1
	}
}
