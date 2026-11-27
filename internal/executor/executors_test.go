package executor

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tyr-zhang/agent-password/internal/store"
	"golang.org/x/crypto/ssh"
)

func testExec(secrets ...string) *Executor { return New(NewRedactor(secrets...)) }

func TestEnvRunInjectsAndRedacts(t *testing.T) {
	const canary = "canary-env-12345678"
	cred := &store.Credential{Kind: store.KindEnvVar, Secret: store.Secret{Key: "SECRET_KEY", Value: canary}}
	res, err := testExec(canary).Run(context.Background(), cred, "env", map[string]any{
		"argv": anyArgs(shell("echo VALUE=$SECRET_KEY")),
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	out := res["output"].(string)
	if contains(out, canary) || !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("secret not redacted: %q", out)
	}
}

func TestAPICallInjectsHeaderAndRedacts(t *testing.T) {
	const token = "api-canary-token-1234"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "path=%s token=%s", r.URL.Path, r.Header.Get("X-Api-Token"))
	}))
	defer srv.Close()
	cred := &store.Credential{Kind: store.KindAPIKey, Secret: store.Secret{
		BaseURL: srv.URL, HeaderName: "X-Api-Token", Token: token,
	}}
	res, err := testExec(token).Run(context.Background(), cred, "api", map[string]any{
		"method": "GET", "path": "/v1/ping",
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	body := res["body"].(string)
	if !strings.Contains(body, "path=/v1/ping") || contains(body, token) {
		t.Fatalf("unexpected body: %q", body)
	}
	if _, err := testExec(token).Run(context.Background(), cred, "api", map[string]any{
		"path": "http://evil.example/x",
	}, "", ""); err == nil {
		t.Fatal("accepted non-absolute path")
	}
}

func TestFillHandleOneShotAndTOTP(t *testing.T) {
	const secret = "fill-canary-98765432"
	e := testExec(secret)
	cred := &store.Credential{Kind: store.KindWeb, Secret: store.Secret{Password: secret,
		TOTPSeed: "JBSWY3DPEHPK3PXP"}}
	handle, expires, err := e.IssueFill(cred, "password", "sess", "client", 0)
	if err != nil || handle == "" || expires.IsZero() {
		t.Fatalf("issue: %v", err)
	}
	res, err := e.Run(context.Background(), cred, "fill.apply", map[string]any{
		"handle": handle, "argv": anyArgs(shell("printf %s \"$AGENT_PWD_FILL\"")),
	}, "sess", "client")
	if err != nil {
		t.Fatal(err)
	}
	if contains(res["output"].(string), secret) {
		t.Fatal("fill leaked secret into child output")
	}
	if _, err := e.ApplyFill(context.Background(), handle, "sess", "client", []string{"true"}); err == nil {
		t.Fatal("replayed a one-shot handle")
	}
	totpHandle, _, err := e.IssueFill(cred, "totp", "sess", "client", 0)
	if err != nil {
		t.Fatal(err)
	}
	if h := e.fillCalls[totpHandle]; h == nil || len(h.value) != 6 {
		t.Fatalf("TOTP handle value: %+v", e.fillCalls[totpHandle])
	}
	if _, _, err := e.IssueFill(cred, "bogus", "", "", 0); err == nil {
		t.Fatal("accepted unknown field")
	}
}

func TestRedisReadOnlyAdapter(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		for i := 0; i < 5; i++ { // "*2", "$3", "GET", "$3", "key"
			if _, err := r.ReadString('\n'); err != nil {
				return
			}
		}
		fmt.Fprint(conn, "$5\r\nhello\r\n")
	}()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portText)
	cred := &store.Credential{Kind: store.KindServer, Secret: store.Secret{
		Protocol: "redis", Host: host, Port: port, ServerUser: "default",
	}}
	res, err := testExec().Run(context.Background(), cred, "sql", map[string]any{"query": "GET key"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res["output"].(string), "hello") {
		t.Fatalf("redis output: %+v", res)
	}
	if _, err := testExec().Run(context.Background(), cred, "sql", map[string]any{"query": "SET key v"}, "", ""); err == nil {
		t.Fatal("write command was not blocked")
	}
}

func TestSSHExecWithMockServer(t *testing.T) {
	t.Setenv("AGENT_PWD_HOME", t.TempDir())
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	_ = pub
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) == "ssh-pass" {
				return nil, nil
			}
			return nil, fmt.Errorf("bad password")
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
		if err != nil {
			return
		}
		defer sconn.Close()
		go ssh.DiscardRequests(reqs)
		for ch := range chans {
			if ch.ChannelType() != "session" {
				ch.Reject(ssh.UnknownChannelType, "no")
				continue
			}
			channel, requests, err := ch.Accept()
			if err != nil {
				return
			}
			go func() {
				for req := range requests {
					if req.Type == "exec" {
						_ = req.Reply(true, nil)
						fmt.Fprintf(channel, "ran:%s", string(req.Payload))
						status := struct{ Status uint32 }{0}
						_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(status))
						channel.Close()
						return
					}
					_ = req.Reply(false, nil)
				}
			}()
		}
	}()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portText)
	cred := &store.Credential{Kind: store.KindServer, Secret: store.Secret{
		Protocol: "ssh", Host: host, Port: port, ServerUser: "tester", Password: "ssh-pass",
	}}
	res, err := testExec().Run(context.Background(), cred, "ssh", map[string]any{
		"command": "echo hi", "insecure_host_key": true,
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res["output"].(string), "echo hi") {
		t.Fatalf("ssh output: %+v", res)
	}
	if _, err := testExec().Run(context.Background(), cred, "ssh", map[string]any{
		"command": "echo hi",
	}, "", ""); err == nil {
		t.Fatal("strict host key check was skipped")
	}
	ln.Close()
	wg.Wait()
}
