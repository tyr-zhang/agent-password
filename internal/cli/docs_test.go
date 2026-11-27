package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type docsDoc struct {
	Version  int `json:"version"`
	Commands []struct {
		Name    string   `json:"name"`
		Syntax  string   `json:"syntax"`
		Summary string   `json:"summary"`
		Flags   []string `json:"flags,omitempty"`
	} `json:"commands"`
	ExitCodes []struct {
		Code    int    `json:"code"`
		Meaning string `json:"meaning"`
	} `json:"exit_codes"`
	Contract string `json:"contract"`
}

func executeText(args ...string) (int, string) {
	var out bytes.Buffer
	code := Execute(args, &out, &out)
	return code, out.String()
}

func parseDocs(t *testing.T, lang string) docsDoc {
	t.Helper()
	code, out := executeText("docs", "--format", "json", "--lang", lang)
	if code != 0 {
		t.Fatalf("docs json (%s) exit %d: %s", lang, code, out)
	}
	var doc docsDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestDocsCoversEveryPublicCommand(t *testing.T) {
	cases := []struct {
		lang    string
		phrases []string
	}{
		{"en", []string{"## AI integration contract", "### Exit codes", "cred show", "confirmation_id", "Use credential labels with `--cred`"}},
		{"zh-CN", []string{"## AI 集成契约", "### 退出码", "cred show", "confirmation_id", "`--cred` 使用凭据 label"}},
	}
	for _, tc := range cases {
		code, out := executeText("docs", "--lang", tc.lang)
		if code != 0 {
			t.Fatalf("docs (%s) exit %d", tc.lang, code)
		}
		for _, cmd := range registry {
			if !cmd.Hidden && !strings.Contains(out, cmd.Name) {
				t.Errorf("docs (%s) missing command %q", tc.lang, cmd.Name)
			}
		}
		for _, phrase := range tc.phrases {
			if !strings.Contains(out, phrase) {
				t.Errorf("docs (%s) missing %q", tc.lang, phrase)
			}
		}
	}
}

func TestDocsJSONMachineTokensStayStable(t *testing.T) {
	en, zh := parseDocs(t, "en"), parseDocs(t, "zh-CN")
	if en.Version != 1 || zh.Version != 1 || len(en.Commands) < 10 ||
		len(en.ExitCodes) != 6 || len(zh.ExitCodes) != 6 || en.Contract == "" || zh.Contract == "" {
		t.Fatal("unexpected docs JSON shape")
	}
	if len(en.Commands) != len(zh.Commands) {
		t.Fatal("command lists differ between languages")
	}
	for i := range en.Commands {
		a, b := en.Commands[i], zh.Commands[i]
		if a.Name != b.Name || a.Syntax != b.Syntax || strings.Join(a.Flags, ",") != strings.Join(b.Flags, ",") {
			t.Errorf("machine tokens changed for %s", a.Name)
		}
	}
	for i := range en.ExitCodes {
		if en.ExitCodes[i].Code != zh.ExitCodes[i].Code {
			t.Errorf("exit code changed at %d", i)
		}
	}
	cjk := regexp.MustCompile(`[\p{Han}]`)
	if !cjk.MatchString(en.Contract) || !cjk.MatchString(zh.Contract) {
		// English contract legitimately contains no Han characters; Chinese must.
		if !cjk.MatchString(zh.Contract) {
			t.Fatal("Chinese contract has no Chinese text")
		}
	}
	for _, field := range []string{"status", "confirmation_id", "poll_token"} {
		if !strings.Contains(en.Contract, field) || !strings.Contains(zh.Contract, field) {
			t.Errorf("machine token %q missing from contract", field)
		}
	}
}

func TestLanguageResolution(t *testing.T) {
	t.Setenv("AGENT_PWD_LANG", "zh")
	if code, out := executeText("docs"); code != 0 || !strings.Contains(out, "AI 集成契约") {
		t.Fatalf("env language not applied: %d %s", code, out)
	}
	if code, out := executeText("--lang", "en", "docs"); code != 0 || !strings.Contains(out, "AI integration contract") {
		t.Fatalf("flag before command failed: %d %s", code, out)
	}
	if code, out := executeText("docs", "--lang=en"); code != 0 || !strings.Contains(out, "AI integration contract") {
		t.Fatalf("flag after command failed: %d %s", code, out)
	}
	t.Setenv("AGENT_PWD_LANG", "bogus")
	if code, out := executeText("docs"); code != 0 || !strings.Contains(out, "AI integration contract") {
		t.Fatalf("invalid env did not fall back to English: %d %s", code, out)
	}
}

func TestFlagParsing(t *testing.T) {
	cmd := &Command{Name: "x", Group: "Management (human)", Syntax: "x", Flags: []Flag{{Name: "cred"}, {Name: "wait", Bool: true}}, Run: func(*Context) int { return 0 }}
	register(cmd)
	ctx := &Context{Flags: map[string]string{}, Bools: map[string]bool{}}
	err := parseFlags(cmd, []string{"arg", "--cred", "abc", "--wait", "--", "--not-a-flag"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Positionals) != 2 || ctx.String("cred", "") != "abc" || !ctx.Bool("wait") {
		t.Fatalf("bad parse: %+v", ctx)
	}
	if err := parseFlags(cmd, []string{"--unknown"}, ctx); err == nil {
		t.Fatal("unknown flag accepted")
	}
}

func TestRepeatedValueFlags(t *testing.T) {
	cmd := &Command{Name: "y", Group: "Management (human)", Syntax: "y", Flags: []Flag{{Name: "arg"}}, Run: func(*Context) int { return 0 }}
	register(cmd)
	ctx := &Context{Flags: map[string]string{}, Bools: map[string]bool{}}
	if err := parseFlags(cmd, []string{"--arg", "a,b", "--arg=c"}, ctx); err != nil {
		t.Fatal(err)
	}
	got := ctx.Strings("arg")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("repeated flags: %+v", got)
	}
}

func TestRotatePasswordFlagSurface(t *testing.T) {
	var cmd *Command
	for _, c := range registry {
		if c.Name == "rotate-password" {
			cmd = c
			break
		}
	}
	if cmd == nil {
		t.Fatal("rotate-password command not registered")
	}
	ctx := &Context{Flags: map[string]string{}, Bools: map[string]bool{}}
	if err := parseFlags(cmd, []string{"--password", "old", "--new-password", "new", "--new-salt"}, ctx); err != nil {
		t.Fatalf("rotate-password flags rejected: %v", err)
	}
	if ctx.String("password", "") != "old" || ctx.String("new-password", "") != "new" || !ctx.Bool("new-salt") {
		t.Fatalf("bad rotate-password flags: %+v", ctx)
	}
}

func TestHelpLocalization(t *testing.T) {
	cases := map[string]string{
		"en": "agent-pwd — credential proxy for AI agents",
		"zh-CN": "agent-pwd — 面向 AI agent 的凭据代理",
	}
	for lang, phrase := range cases {
		code, out := executeText("help", "--lang", lang)
		if !strings.Contains(out, phrase) {
			t.Errorf("help (%s): exit %d, missing %q: %s", lang, code, phrase, out)
		}
	}
}

// TestNoStaleCredentialPlaceholders guards the documentation sweep: the
// historical design snapshot is exempt because it is an archived spec.
func TestNoStaleCredentialPlaceholders(t *testing.T) {
	stale := []string{
		"--cred <id>", "--cred <label>", "--cred <label-or-id>",
		"--cred <credential-label>", "cred show <id|label>", "cred rm <id|label>",
		"cred show <label-or-id>", "cred rm <label-or-id>",
	}
	root := filepath.Join("..", "..")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "bin") {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/docs/design/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, bad := range stale {
			if strings.Contains(text, bad) {
				t.Errorf("%s still contains %q", path, bad)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
