package executor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/tyr-zhang/agent-password/internal/store"
)

const (
	MaxOutputBytes = 1 << 20
	MaxRows        = 1000
)

var ErrUnsupported = errors.New("executor: unsupported credential/action combination")

type Executor struct {
	redactor  *Redactor
	timeout   time.Duration
	mu        sync.Mutex
	fillCalls map[string]*fillHandle
}

func New(redactor *Redactor) *Executor {
	return &Executor{
		redactor:  redactor,
		timeout:   30 * time.Second,
		fillCalls: map[string]*fillHandle{},
	}
}

func (e *Executor) Redactor() *Redactor { return e.redactor }

func stringArg(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(v)
	}
}

func boolArg(args map[string]any, key string) bool {
	v, ok := args[key].(bool)
	return ok && v
}

func stringsArg(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok {
		return nil
	}
	switch x := raw.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, v := range x {
			out = append(out, stringArg(map[string]any{"v": v}, "v"))
		}
		return out
	default:
		if s := stringArg(args, key); s != "" {
			return []string{s}
		}
		return nil
	}
}

func capString(s string) (string, bool) {
	if len(s) <= MaxOutputBytes {
		return s, false
	}
	return s[:MaxOutputBytes], true
}

// Run dispatches an action to the matching credential kind.
func (e *Executor) Run(ctx context.Context, cred *store.Credential, action string, args map[string]any, sessionID, peer string) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	switch action {
	case "ssh":
		return e.runSSH(ctx, cred, args)
	case "sql":
		return e.runSQL(ctx, cred, args)
	case "api":
		return e.runAPI(ctx, cred, args)
	case "env":
		return e.runEnv(ctx, cred, args)
	case "fill.apply":
		return e.ApplyFill(ctx, stringArg(args, "handle"), sessionID, peer, stringsArg(args, "argv"))
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, action)
	}
}
