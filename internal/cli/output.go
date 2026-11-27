package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func emit(ctx *Context, result map[string]any, e *CallError) int {
	if e != nil {
		if ctx.isJSON() {
			return ctx.Fail(e.ExitCode(), e.Message)
		}
		ctx.Warn(e.Message)
		if e.RetryAfter > 0 {
			ctx.Warn(ctx.T("retry after %ds", e.RetryAfter))
		}
		return e.ExitCode()
	}
	if ctx.isJSON() {
		return ctx.PrintJSON(result)
	}
	keys := make([]string, 0, len(result))
	for k := range result {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b, err := json.Marshal(result[k])
		if err != nil {
			continue
		}
		v := string(b)
		if len(v) > 160 {
			v = v[:160] + "..."
		}
		fmt.Fprintf(ctx.Stdout, "%s: %s\n", k, strings.Trim(v, `"`))
	}
	return 0
}
