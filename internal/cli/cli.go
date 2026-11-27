// Package cli implements the only AI-facing surface: the command-line
// interface. The command registry is also the single source of truth for
// `agent-pwd docs`.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/tyr-zhang/agent-password/internal/i18n"
)

type Flag struct {
	Name    string
	Bool    bool
	Default string
	Summary string
}

type Command struct {
	Name    string
	Group   string
	Summary string
	Syntax  string
	Flags   []Flag
	Doc     []string
	Hidden  bool
	Run     func(*Context) int
}

var registry []*Command

func register(c *Command) { registry = append(registry, c) }

type Context struct {
	Cmd         *Command
	Lang        i18n.Lang
	Flags       map[string]string
	Multi       map[string][]string // repeated value flags, in command-line order
	Bools       map[string]bool
	Positionals []string
	Stdout      io.Writer
	Stderr      io.Writer
}

func (c *Context) String(key, def string) string {
	if v, ok := c.Flags[key]; ok && v != "" {
		return v
	}
	return def
}

func (c *Context) Bool(key string) bool { return c.Bools[key] }

// T translates a canonical English source string into the active language.
func (c *Context) T(key string, args ...any) string { return i18n.T(c.Lang, key, args...) }

func (c *Context) Int(key string, def int) int {
	v := c.String(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (c *Context) Strings(key string) []string {
	if values := c.Multi[key]; len(values) > 0 {
		var out []string
		for _, v := range values {
			out = append(out, strings.Split(v, ",")...)
		}
		return out
	}
	v, ok := c.Flags[key]
	if !ok || v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

func (c *Context) Arg(i int) string {
	if i < len(c.Positionals) {
		return c.Positionals[i]
	}
	return ""
}

func (c *Context) ArgsFrom(i int) []string {
	if i >= len(c.Positionals) {
		return nil
	}
	return append([]string(nil), c.Positionals[i:]...)
}

func (c *Context) PrintJSON(result map[string]any) int {
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return c.Fail(1, err.Error())
	}
	fmt.Fprintln(c.Stdout, string(b))
	return 0
}

func (c *Context) Fail(code int, message string) int {
	b, _ := json.MarshalIndent(map[string]any{
		"ok": false, "error": map[string]any{"code": code, "message": message},
	}, "", "  ")
	fmt.Fprintln(c.Stdout, string(b))
	return code
}

func (c *Context) Print(v ...any) {
	fmt.Fprintln(c.Stdout, v...)
}

func (c *Context) Warn(v ...any) {
	fmt.Fprintln(c.Stderr, v...)
}

func (c *Context) isJSON() bool { return !c.Bool("human") }

// currentLang is process-scoped: Execute handles exactly one CLI command, and
// Call() attaches the language to every daemon request without changing the
// many call sites.
var currentLang = i18n.EN

func resolveLanguage(args []string) (i18n.Lang, []string) {
	lang := i18n.Detect("")
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			out = append(out, args[i:]...)
			break
		}
		if tok == "--lang" && i+1 < len(args) {
			lang = i18n.ParseLang(args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(tok, "--lang=") {
			lang = i18n.ParseLang(strings.TrimPrefix(tok, "--lang="))
			continue
		}
		out = append(out, tok)
	}
	return lang, out
}

func Execute(args []string, stdout, stderr io.Writer) int {
	lang, args := resolveLanguage(args)
	currentLang = lang
	cmd, rest, err := matchCommand(args, lang)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx := &Context{Cmd: cmd, Lang: lang, Flags: map[string]string{}, Multi: map[string][]string{},
		Bools: map[string]bool{}, Stdout: stdout, Stderr: stderr}
	if err := parseFlags(cmd, rest, ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	helpRequested := ctx.Bool("help") || len(args) == 0 || (len(args) > 0 && args[0] == "help")
	if helpRequested {
		printHelp(cmd, stdout, lang)
		if cmd == nil {
			return 1
		}
		return 0
	}
	return cmd.Run(ctx)
}

func matchCommand(args []string, lang i18n.Lang) (*Command, []string, error) {
	if len(args) == 0 {
		return nil, nil, nil
	}
	candidates := append([]*Command(nil), registry...)
	sort.Slice(candidates, func(i, j int) bool {
		return len(strings.Fields(candidates[i].Name)) > len(strings.Fields(candidates[j].Name))
	})
	for _, c := range candidates {
		words := strings.Fields(c.Name)
		if len(args) < len(words) {
			continue
		}
		ok := true
		for i, w := range words {
			if args[i] != w {
				ok = false
				break
			}
		}
		if ok {
			return c, args[len(words):], nil
		}
	}
	if args[0] == "help" {
		return nil, args[1:], nil
	}
	return nil, nil, fmt.Errorf("%s", i18n.T(lang, "unknown command %q; run agent-pwd docs to see available commands", args[0]))
}

var globalFlags = []Flag{
	{Name: "lang", Summary: "output language: en or zh-CN (also AGENT_PWD_LANG)"},
	{Name: "json", Bool: true, Summary: "emit machine-readable JSON (default)"},
	{Name: "human", Bool: true, Summary: "emit compact human-readable output"},
	{Name: "help", Bool: true, Summary: "show command help"},
}

func parseFlags(cmd *Command, args []string, ctx *Context) error {
	if ctx.Multi == nil {
		ctx.Multi = map[string][]string{}
	}
	known := map[string]Flag{}
	for _, f := range append(append([]Flag(nil), globalFlags...), cmdFlags(cmd)...) {
		known[f.Name] = f
	}
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			ctx.Positionals = append(ctx.Positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(tok, "--") {
			ctx.Positionals = append(ctx.Positionals, tok)
			continue
		}
		key := strings.TrimPrefix(tok, "--")
		value := ""
		hasValue := false
		if j := strings.IndexByte(key, '='); j >= 0 {
			value, key, hasValue = key[j+1:], key[:j], true
		}
		f, ok := known[key]
		if !ok {
			// Pass through unknown flags after `--`; before it they are errors.
			return fmt.Errorf("%s", ctx.T("unknown flag --%s for %s", key, cmdName(cmd)))
		}
		if f.Bool {
			if hasValue && strings.EqualFold(value, "false") {
				ctx.Bools[key] = false
			} else {
				ctx.Bools[key] = true
			}
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return fmt.Errorf("%s", ctx.T("flag --%s needs a value", key))
			}
			i++
			value = args[i]
		}
		ctx.Multi[key] = append(ctx.Multi[key], value)
		if _, exists := ctx.Flags[key]; !exists {
			ctx.Flags[key] = value
		} else {
			ctx.Flags[key] += "," + value
		}
	}
	for _, f := range known {
		if !f.Bool && ctx.Flags[f.Name] == "" && f.Default != "" {
			ctx.Flags[f.Name] = f.Default
		}
	}
	return nil
}

func cmdFlags(cmd *Command) []Flag {
	if cmd == nil {
		return nil
	}
	return cmd.Flags
}

func cmdName(cmd *Command) string {
	if cmd == nil {
		return "agent-pwd"
	}
	return "agent-pwd " + cmd.Name
}

func printHelp(cmd *Command, w io.Writer, lang i18n.Lang) {
	tr := func(key string, args ...any) string { return i18n.T(lang, key, args...) }
	if cmd == nil {
		fmt.Fprintln(w, tr("agent-pwd — credential proxy for AI agents"))
		fmt.Fprintln(w, tr("usage: agent-pwd <command> [flags] [args]"))
		fmt.Fprintln(w, tr("\ncommands:"))
		groups := map[string][]string{}
		for _, c := range registry {
			if c.Hidden {
				continue
			}
			groups[c.Group] = append(groups[c.Group], fmt.Sprintf("  %-34s %s", c.Name, tr(c.Summary)))
		}
		var names []string
		for g := range groups {
			names = append(names, g)
		}
		sort.Strings(names)
		for _, g := range names {
			fmt.Fprintf(w, "\n%s:\n", tr(g))
			for _, line := range groups[g] {
				fmt.Fprintln(w, line)
			}
		}
		fmt.Fprintln(w, tr("\nRun `agent-pwd docs` for the full AI integration contract."))
		return
	}
	fmt.Fprintf(w, tr("usage: %s\n\n%s\n"), cmd.Syntax, tr(cmd.Summary))
	if len(cmd.Flags) > 0 {
		fmt.Fprintln(w, tr("\nflags:"))
		for _, f := range cmd.Flags {
			fmt.Fprintf(w, "  --%-22s %s\n", f.Name, tr(f.Summary))
		}
	}
	for _, line := range cmd.Doc {
		fmt.Fprintln(w, tr(line))
	}
}
