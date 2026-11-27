package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/tyr-zhang/agent-password/internal/store"
)

// runEnv injects an envvar credential into a child process. There is no
// command anywhere that prints the value itself.
func (e *Executor) runEnv(ctx context.Context, cred *store.Credential, args map[string]any) (map[string]any, error) {
	if cred.Kind != store.KindEnvVar {
		return nil, errors.New("executor: env requires an envvar credential")
	}
	if cred.Secret.Key == "" || cred.Secret.Value == "" {
		return nil, errors.New("executor: envvar credential is incomplete")
	}
	argv := stringsArg(args, "argv")
	if len(argv) == 0 {
		return nil, errors.New("executor: env run requires a command after --")
	}
	red := NewRedactor(e.redactor.Secrets()...)
	red.Add(cred.Secret.Value)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), cred.Secret.Key+"="+cred.Secret.Value)
	out, err := cmd.CombinedOutput()
	text, truncated := capString(string(red.RedactBytes(out)))
	exit := 0
	if ee := (*exec.ExitError)(nil); err != nil && errors.As(err, &ee) {
		exit = ee.ExitCode()
	} else if err != nil {
		return nil, fmt.Errorf("executor: run child: %w", err)
	}
	return map[string]any{"output": text, "exit_code": exit, "truncated": truncated}, nil
}
