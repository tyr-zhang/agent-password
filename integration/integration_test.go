package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tyr-zhang/agent-password/internal/cli"
	"github.com/tyr-zhang/agent-password/internal/daemon"
	"github.com/tyr-zhang/agent-password/internal/platform"
)

const (
	master = "integration-master-pass"
	canary = "integration-canary-secret-1234"
)

func run(args ...string) (int, string) {
	var out bytes.Buffer
	code := cli.Execute(args, &out, &out)
	return code, out.String()
}

func mustRun(t *testing.T, args ...string) map[string]any {
	t.Helper()
	code, out := run(args...)
	if code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, out)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("%v produced invalid JSON: %s", args, out)
	}
	return result
}

func waitForDaemon(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := platform.Current().Dial()
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("daemon did not come up")
}

func TestFullCLIAndDaemonFlow(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PWD_HOME", home)
	if code, out := run("init", "--password", master); code != 0 {
		t.Fatalf("init: %d %s", code, out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d, err := daemon.New(daemon.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitForDaemon(t)
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("daemon did not stop")
		}
	}()

	mustRun(t, "unlock", "--password", master)
	added := mustRun(t, "cred", "add", "envvar", "demo",
		"--secret", `{"key":"TOKEN","value":"`+canary+`"}`)
	id, _ := added["credential"].(map[string]any)["id"].(string)
	if id == "" {
		t.Fatalf("no credential id: %+v", added)
	}
	listed := mustRun(t, "cred", "list")
	if len(listed["credentials"].([]any)) != 1 {
		t.Fatal("credential was not listed")
	}
	shown := mustRun(t, "cred", "show", id)
	shownJSON, _ := json.Marshal(shown)
	if strings.Contains(string(shownJSON), canary) || strings.Contains(string(shownJSON), "value") {
		t.Fatalf("cred show leaked a secret: %s", shownJSON)
	}

	envArgs := append([]string{"env", "run", "--cred", id, "--"}, shell("echo V=$TOKEN")...)
	res := mustRun(t, envArgs...)
	if strings.Contains(mustJSON(res), canary) || !strings.Contains(mustJSON(res), "[REDACTED]") {
		t.Fatalf("env output leaked canary: %v", res)
	}

	high := mustRun(t, "cred", "add", "envvar", "risky", "--risk", "high",
		"--secret", `{"key":"RISK","value":"`+canary+`-risk"}`)
	highID, _ := high["credential"].(map[string]any)["id"].(string)
	highArgs := append([]string{"env", "run", "--cred", highID, "--"}, shell("true")...)
	code, out := run(highArgs...)
	if code != 10 {
		t.Fatalf("high-risk action exit %d: %s", code, out)
	}
	var pending map[string]any
	if err := json.Unmarshal([]byte(out), &pending); err != nil {
		t.Fatal(err)
	}
	cid, _ := pending["confirmation_id"].(string)
	token, _ := pending["poll_token"].(string)
	if cid == "" || token == "" {
		t.Fatalf("pending response missing tokens: %s", out)
	}
	mustRun(t, "confirm", "approve", cid, "--no-biometric", "--password", master)
	var final map[string]any
	for i := 0; i < 30; i++ {
		final = mustRun(t, "confirm", "result", cid, "--token", token)
		if final["status"] != "pending" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if final["status"] != "done" {
		t.Fatalf("unexpected result: %+v", final)
	}
	if code, _ := run("confirm", "result", cid, "--token", token); code != 1 {
		t.Fatalf("poll token replay did not fail: %d", code)
	}

	verify := mustRun(t, "audit", "verify")
	if verify["ok"] != true || verify["records"].(float64) < 4 {
		t.Fatalf("audit verify: %+v", verify)
	}
	mustRun(t, "lock")
	if code, out := run("env", "run", "--cred", id, "--", "true"); code != 20 {
		t.Fatalf("locked action exit %d: %s", code, out)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestDaemonNotRunningExitCode(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PWD_HOME", home)
	if code, _ := run("status"); code != 20 {
		t.Fatalf("status without daemon exit %d", code)
	}
}

func TestNoCommandOutputLeaksSecrets(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PWD_HOME", home)
	mustRun(t, "init", "--password", master)
	ctx, cancel := context.WithCancel(context.Background())
	d, err := daemon.New(daemon.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitForDaemon(t)
	defer func() {
		cancel()
		<-done
	}()
	mustRun(t, "unlock", "--password", master)

	canaries := []string{
		"web-canary-secret-9876",
		"token-canary-secret-9876",
		"ssh-canary-secret-9876",
		"env-canary-secret-9876",
	}
	adds := [][]string{
		{"cred", "add", "web", "site", "--secret", `{"username":"alice","password":"` + canaries[0] + `"}`},
		{"cred", "add", "apikey", "api", "--secret", `{"base_url":"http://127.0.0.1:1","token":"` + canaries[1] + `"}`},
		{"cred", "add", "server", "host", "--secret", `{"protocol":"ssh","host":"127.0.0.1","server_user":"u","password":"` + canaries[2] + `"}`},
		{"cred", "add", "envvar", "env", "--secret", `{"key":"CANARY","value":"` + canaries[3] + `"}`},
	}
	var ids []string
	for _, args := range adds {
		res := mustRun(t, args...)
		cred, _ := res["credential"].(map[string]any)
		ids = append(ids, cred["id"].(string))
	}
	res := mustRun(t, "fill", "--cred", ids[0], "--field", "password")
	handle, _ := res["handle"].(string)

	commands := [][]string{
		{"status"}, {"cred", "list"}, {"cred", "show", ids[0]}, {"cred", "show", ids[2]},
		{"policy", "list"}, {"audit", "tail"}, {"audit", "verify"}, {"doctor"}, {"docs"}, {"docs", "--lang", "zh-CN"},
		{"fill", "--cred", ids[1], "--field", "token"},
		{"fill", "apply", "--handle", handle, "--", "true"},
		{"env", "run", "--cred", ids[3], "--", "true"},
	}
	for _, args := range commands {
		_, out := run(args...)
		for _, canary := range canaries {
			if strings.Contains(out, canary) {
				t.Fatalf("%v leaked %q: %s", args, canary, out)
			}
		}
	}
	auditData, err := os.ReadFile(home + "/audit.log")
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range canaries {
		if strings.Contains(string(auditData), canary) {
			t.Fatalf("audit log leaked %q", canary)
		}
	}
}

func TestConfirmationTimeout(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PWD_HOME", home)
	mustRun(t, "init", "--password", master)
	cfg := daemon.DefaultConfig()
	cfg.ConfirmTTL = 1200 * time.Millisecond
	d, err := daemon.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitForDaemon(t)
	defer func() {
		cancel()
		<-done
	}()
	mustRun(t, "unlock", "--password", master)
	high := mustRun(t, "cred", "add", "envvar", "risky", "--risk", "high",
		"--secret", `{"key":"X","value":"timeout-canary-1234"}`)
	id, _ := high["credential"].(map[string]any)["id"].(string)
	code, out := run("env", "run", "--cred", id, "--", "true")
	if code != 10 {
		t.Fatalf("expected exit 10, got %d: %s", code, out)
	}
	var pending map[string]any
	_ = json.Unmarshal([]byte(out), &pending)
	cid, _ := pending["confirmation_id"].(string)
	token, _ := pending["poll_token"].(string)
	time.Sleep(1600 * time.Millisecond)
	for i := 0; i < 10; i++ {
		code, out = run("confirm", "result", cid, "--token", token)
		if code != 0 || !strings.Contains(out, `"status": "pending"`) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(out, `"status": "timeout"`) {
		t.Fatalf("unexpected timeout result: %d %s", code, out)
	}
	auditData, err := os.ReadFile(home + "/audit.log")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(auditData), `"outcome":"timeout"`) {
		t.Fatal("timeout outcome was not audited")
	}
}

func TestPasswordRotationPreservesVault(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PWD_HOME", home)
	mustRun(t, "init", "--password", master)
	ctx, cancel := context.WithCancel(context.Background())
	d, err := daemon.New(daemon.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitForDaemon(t)
	defer func() {
		cancel()
		<-done
	}()
	mustRun(t, "unlock", "--password", master)
	added := mustRun(t, "cred", "add", "envvar", "rotate",
		"--secret", `{"key":"K","value":"rotation-canary-1234"}`)
	cred, _ := added["credential"].(map[string]any)
	id := cred["id"].(string)

	const next = "rotated-master-pass-9876"
	mustRun(t, "rotate-password", "--password", master, "--new-password", next)
	mustRun(t, "lock")
	mustRun(t, "unlock", "--password", next)
	listed := mustRun(t, "cred", "list")
	if n := len(listed["credentials"].([]any)); n != 1 {
		t.Fatalf("credentials lost after rotation: %d", n)
	}
	if code, _ := run("unlock", "--password", master); code != 20 {
		t.Fatalf("old password still accepted after rotation: %d", code)
	}
	res := mustRun(t, "env", "run", "--cred", id, "--", "true")
	if res["decision"] != "allow" {
		t.Fatalf("credential unusable after rotation: %+v", res)
	}
}

func TestChineseOutputKeepsMachineTokens(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PWD_HOME", home)
	t.Setenv("AGENT_PWD_LANG", "zh-CN")
	mustRun(t, "init", "--password", master)
	ctx, cancel := context.WithCancel(context.Background())
	d, err := daemon.New(daemon.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitForDaemon(t)
	defer func() {
		cancel()
		<-done
	}()
	mustRun(t, "unlock", "--password", master)

	status := mustRun(t, "status")
	if status["unlocked"] != true {
		t.Fatalf("status machine token changed: %+v", status)
	}
	added := mustRun(t, "cred", "add", "envvar", "双语",
		"--secret", `{"key":"TOKEN","value":"bilingual-canary-1234"}`)
	cred, _ := added["credential"].(map[string]any)
	id := cred["id"].(string)
	if !strings.HasPrefix(id, "cred_") {
		t.Fatalf("credential id format changed: %q", id)
	}
	res := mustRun(t, "env", "run", "--cred", id, "--", "true")
	if res["decision"] != "allow" || res["exit_code"] != float64(0) {
		t.Fatalf("action machine tokens changed: %+v", res)
	}
	if strings.Contains(mustJSON(res), "bilingual-canary-1234") {
		t.Fatal("secret leaked in Chinese mode")
	}
	code, docsOut := run("docs")
	if code != 0 || !strings.Contains(docsOut, "AI 集成契约") || !strings.Contains(docsOut, "confirmation_id") {
		t.Fatalf("Chinese contract: %d %s", code, docsOut)
	}
}
