//go:build windows

package executor

func shell(script string) []string { return []string{"cmd", "/c", script} }

func anyArgs(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
