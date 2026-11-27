package cli

import "github.com/tyr-zhang/agent-password/internal/build"

func init() {
	register(&Command{
		Name: "version", Group: "Management (human)", Syntax: "agent-pwd version",
		Summary: "print the agent-pwd build version",
		Run: func(c *Context) int {
			return c.PrintJSON(map[string]any{
				"version": build.Version, "commit": build.Commit, "date": build.Date,
			})
		},
	})
}
