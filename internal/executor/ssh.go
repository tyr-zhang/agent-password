package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tyr-zhang/agent-password/internal/platform"
	"github.com/tyr-zhang/agent-password/internal/store"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func (e *Executor) runSSH(ctx context.Context, cred *store.Credential, args map[string]any) (map[string]any, error) {
	if cred.Kind != store.KindServer || cred.Secret.Protocol != "ssh" {
		return nil, errors.New("executor: ssh requires a server credential with protocol=ssh")
	}
	s := cred.Secret
	if s.Host == "" {
		return nil, errors.New("executor: ssh credential has no host")
	}
	port := s.Port
	if port == 0 {
		port = 22
	}
	auth := []ssh.AuthMethod{}
	if s.Password != "" {
		auth = append(auth, ssh.Password(s.Password))
	}
	key := []byte(s.PrivateKey)
	if len(key) == 0 && s.KeyPath != "" {
		var err error
		key, err = os.ReadFile(s.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("executor: read private key: %w", err)
		}
	}
	if len(key) > 0 {
		var signer ssh.Signer
		var err error
		if s.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(s.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("executor: parse private key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if len(auth) == 0 {
		return nil, errors.New("executor: ssh credential has neither password nor private key")
	}

	callback, err := hostKeyCallback(boolArg(args, "insecure_host_key"))
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            s.ServerUser,
		Auth:            auth,
		HostKeyCallback: callback,
		Timeout:         10 * second,
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(port))
	type dialResult struct {
		c   *ssh.Client
		err error
	}
	ch := make(chan dialResult, 1)
	go func() {
		c, err := ssh.Dial("tcp", addr, cfg)
		ch <- dialResult{c: c, err: err}
	}()
	var dialed *ssh.Client
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("executor: ssh dial: %w", res.err)
		}
		dialed = res.c
	}
	defer dialed.Close()
	session, err := dialed.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	command := stringArg(args, "command")
	type runResult struct {
		out []byte
		err error
	}
	rch := make(chan runResult, 1)
	go func() {
		out, err := session.CombinedOutput(command)
		rch <- runResult{out: out, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-rch:
		text, truncated := capString(string(e.redactor.RedactBytes(out.out)))
		exit := 0
		var ee *ssh.ExitError
		if errors.As(out.err, &ee) {
			exit = ee.ExitStatus()
		} else if out.err != nil && !errors.Is(out.err, io.EOF) {
			return nil, fmt.Errorf("executor: ssh command: %w", out.err)
		}
		return map[string]any{"output": text, "exit_code": exit, "truncated": truncated}, nil
	}
}

const second = 1e9

func hostKeyCallback(insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	dir, err := platform.DataDir()
	if err != nil {
		return nil, err
	}
	file := filepath.Join(dir, "known_hosts")
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("executor: no known_hosts at %s (add the host key, or pass insecure_host_key for an action)", file)
	}
	return knownhosts.New(file)
}
