package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tyr-zhang/agent-password/internal/audit"
	"github.com/tyr-zhang/agent-password/internal/executor"
	"github.com/tyr-zhang/agent-password/internal/i18n"
	"github.com/tyr-zhang/agent-password/internal/policy"
	"github.com/tyr-zhang/agent-password/internal/platform"
	"github.com/tyr-zhang/agent-password/internal/protocol"
	"github.com/tyr-zhang/agent-password/internal/store"
)

func (d *Daemon) handleAction(req protocol.Request) protocol.Response {
	switch req.Op {
	case "fill.revoke":
		d.exec.RevokeFill(argString(req.Args, "handle"))
		return protocol.OK(map[string]any{"status": "revoked"})
	case "fill.issue":
		return d.startAction(req, "fill", "fill.issue")
	case "fill.apply":
		return d.startAction(req, "fill", "fill.apply")
	case "action.ssh", "action.sql", "action.api", "action.env":
		action := strings.TrimPrefix(req.Op, "action.")
		return d.startAction(req, action, action)
	default:
		return protocol.Fail(protocol.CodeBadArgs, "unknown action")
	}
}

func (d *Daemon) startAction(req protocol.Request, policyAction, execAction string) protocol.Response {
	credID := argString(req.Args, "cred")
	body, err := d.store.Body()
	if err != nil {
		return protocol.Fail(protocol.CodeInternal, err.Error())
	}
	var cred *store.Credential
	if credID == "" && execAction == "fill.apply" {
		// A fill handle is already a capability; consumption is governed by
		// the fill policy but needs no credential id from the caller.
		cred = &store.Credential{ID: "fill-handle", Kind: store.KindWeb}
	} else if idx := findCredentialIndex(body, credID); idx >= 0 {
		cred = &body.Credentials[idx]
	}
	if cred == nil {
		return protocol.Fail(protocol.CodeNotFound, "credential not found")
	}
	ctx := policy.Context{
		Method: argString(req.Args, "method"),
		SQL:    argString(req.Args, "query"),
	}
	dec := policy.Decide(body.Policies, cred, policyAction, ctx)
	summary := d.redactor.Redact(fmt.Sprintf("%s/%s %s %s", cred.Kind, cred.Label,
		policyAction, preview(argFirst(req.Args, "command", "query", "path", "argv"))))
	if dec.Decision == policy.Deny {
		if err := d.record(cred.ID, policyAction, dec, "denied", protocol.CodeDenied, "", summary, req.Client); err != nil {
			return protocol.Fail(protocol.CodeInternal, "audit write failed: "+err.Error())
		}
		return protocol.Fail(protocol.CodeDenied, "policy denied this action (rule "+dec.RuleID+")")
	}
	p := &pending{
		op: execAction, policyAction: policyAction, credID: cred.ID,
		args: req.Args, client: req.Client, lang: i18n.ParseLang(req.Lang), decision: dec, summary: summary,
	}
	if dec.Decision == policy.Confirm {
		if err := d.pending.add(p); err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		if err := d.record(cred.ID, policyAction, dec, "started", "", "", summary, req.Client); err != nil {
			return protocol.Fail(protocol.CodeInternal, "audit write failed: "+err.Error())
		}
		go d.runPending(p)
		return protocol.OK(map[string]any{
			"status": "pending", "confirmation_id": p.id, "poll_token": p.pollToken,
			"expires_at": p.expires.UTC().Format(time.RFC3339),
			"reason": dec.Reason, "summary": summary,
			"next": i18n.T(p.lang, "tell the human the confirmation_id and poll token; then run agent-pwd confirm result <id> --token <poll_token>"),
		})
	}
	if err := d.record(cred.ID, policyAction, dec, "started", "", "", summary, req.Client); err != nil {
		return protocol.Fail(protocol.CodeInternal, "audit write failed: "+err.Error())
	}
	result, failure := d.execute(context.Background(), p)
	if failure != nil {
		return protocol.Fail(failure.Code, failure.Message)
	}
	return protocol.OK(result)
}

func argFirst(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := argString(args, k); v != "" {
			return v
		}
	}
	return ""
}

func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 100 {
		return s[:100] + "..."
	}
	return s
}

// runPending waits for a human decision or timeout, then executes or records
// the outcome. Results are retained briefly so `confirm result` can fetch
// them from another terminal.
func (d *Daemon) runPending(p *pending) {
	timer := time.NewTimer(time.Until(p.expires))
	defer timer.Stop()
	approved := false
	source := ""
	select {
	case approved = <-p.resolve:
		d.pending.mu.Lock()
		source = p.confirmSource
		d.pending.mu.Unlock()
	case <-timer.C:
		d.pending.setOutcome(p, "timeout", nil, nil)
		_ = d.record(p.credID, p.policyAction, p.decision, "timeout", protocol.CodeTimeout, "timeout", p.summary, p.client)
		return
	}
	if !approved || source == "lock" {
		state := "denied"
		code := protocol.CodeDenied
		if source == "lock" {
			state, code = "timeout", protocol.CodeTimeout
		}
		d.pending.setOutcome(p, state, nil, &protocol.Error{Code: code, Message: "confirmation " + state})
		_ = d.record(p.credID, p.policyAction, p.decision, state, code, source, p.summary, p.client)
		return
	}
	result, failure := d.execute(context.Background(), p)
	if failure != nil {
		d.pending.setOutcome(p, "error", nil, failure)
		return
	}
	d.pending.setOutcome(p, "done", result, nil)
}

func (d *Daemon) execute(ctx context.Context, p *pending) (map[string]any, *protocol.Error) {
	body, err := d.store.Body()
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeInternal, Message: err.Error()}
	}
	var cred *store.Credential
	if p.op == "fill.apply" {
		cred = &store.Credential{ID: "fill-handle", Kind: store.KindWeb}
	} else if idx := findCredentialIndex(body, p.credID); idx >= 0 {
		cred = &body.Credentials[idx]
	}
	if cred == nil {
		return nil, &protocol.Error{Code: protocol.CodeNotFound, Message: "credential no longer exists"}
	}
	var result map[string]any
	switch p.op {
	case "fill.issue":
		field := argString(p.args, "field")
		if field == "" {
			field = "password"
		}
		handle, expires, err := d.exec.IssueFill(cred, field, d.sess.StableID(), p.client.Name, d.cfg.FillTTL)
		if err != nil {
			return nil, errorResponse(err)
		}
		result = map[string]any{"handle": handle, "field": field,
			"expires_at": expires.UTC().Format(time.RFC3339),
			"note": i18n.T(p.lang, "use agent-pwd fill apply --handle <handle> -- <command>; the value is never printed")}
	default:
		result, err = d.exec.Run(ctx, cred, p.op, p.args, d.sess.StableID(), p.client.Name)
		if err != nil {
			return nil, errorResponse(err)
		}
	}
	result["credential_id"] = cred.ID
	result["decision"] = p.decision.Decision
	result["policy_rule"] = p.decision.RuleID
	if err := d.record(cred.ID, p.policyAction, p.decision, "completed", protocol.CodeOK, "", p.summary, p.client); err != nil {
		d.sess.Lock()
		return nil, &protocol.Error{Code: protocol.CodeInternal, Message: "audit write failed after execution: " + err.Error()}
	}
	d.sess.Touch()
	return result, nil
}

func errorResponse(err error) *protocol.Error {
	if errors.Is(err, executor.ErrUnsupported) {
		return &protocol.Error{Code: protocol.CodeBadArgs, Message: err.Error()}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &protocol.Error{Code: protocol.CodeTimeout, Message: "action timed out"}
	}
	return &protocol.Error{Code: protocol.CodeInternal, Message: err.Error()}
}

func (d *Daemon) record(credID, action string, dec policy.Decision, outcome, code, source, detail string, client protocol.ClientInfo) error {
	sum := sha256.Sum256([]byte(d.redactor.Redact(detail)))
	r := audit.Record{
		CredentialID:  credID,
		Action:        action,
		Decision:      dec.Decision,
		ConfirmSource: source,
		Outcome:       outcome,
		ResultCode:    code,
		DetailHash:    hex.EncodeToString(sum[:]),
		PolicyRule:    dec.RuleID,
		Client:        client.Name + " @" + client.CWD,
	}
	return audit.Append(d.auditPath, &r)
}

func (d *Daemon) handleConfirm(req protocol.Request) protocol.Response {
	id := argString(req.Args, "id")
	p := d.pending.get(id)
	if p == nil {
		return protocol.Fail(protocol.CodeNotFound, "confirmation not found (daemon restart cancels pending actions)")
	}
	switch req.Op {
	case "confirm.info":
		d.pending.mu.Lock()
		info := map[string]any{
			"status": p.state, "confirmation_id": id, "action": p.policyAction,
			"summary": p.summary, "nonce": hex.EncodeToString(p.nonce),
			"expires_at": p.expires.UTC().Format(time.RFC3339),
			"decision": p.decision.Decision,
		}
		d.pending.mu.Unlock()
		return protocol.OK(info)
	case "confirm.approve":
		source := ""
		if argBool(req.Args, "biometric") && platform.Current().BiometricAvailable() {
			ok, err := platform.Current().VerifyBiometric(p.nonce, p.summary)
			if err != nil || !ok {
				// Best effort: fall through to the master-password proof.
				_ = err
			} else {
				source = "biometric"
			}
		}
		if source == "" {
			proof := argString(req.Args, "proof")
			if proof == "" || !d.sess.VerifyProof(p.id, hex.EncodeToString(p.nonce), proof) {
				return protocol.Fail(protocol.CodeBiometric,
					"approval requires a biometric check or a master-password proof")
			}
			source = "password"
		}
		if _, ok := d.pending.fulfill(id, true, source); !ok {
			return protocol.Fail(protocol.CodeBadArgs, "confirmation is no longer pending")
		}
		_ = d.record(p.credID, p.policyAction, p.decision, "approved", "", source, p.summary, p.client)
		return protocol.OK(map[string]any{"status": "approved", "confirmation_id": id, "source": source})
	case "confirm.deny":
		if _, ok := d.pending.fulfill(id, false, "human"); !ok {
			return protocol.Fail(protocol.CodeBadArgs, "confirmation is no longer pending")
		}
		return protocol.OK(map[string]any{"status": "denied", "confirmation_id": id})
	case "confirm.result":
		token := argString(req.Args, "token")
		d.pending.mu.Lock()
		defer d.pending.mu.Unlock()
		if token == "" || !constantTimeEqual(token, p.pollToken) {
			return protocol.Fail(protocol.CodeBadArgs, "invalid poll token")
		}
		if p.state == "pending" {
			return protocol.OK(map[string]any{"status": "pending", "confirmation_id": id,
				"expires_at": p.expires.UTC().Format(time.RFC3339)})
		}
		p.pollToken = "" // one-shot: the final result can be fetched exactly once
		out := map[string]any{
			"status": p.state, "confirmation_id": id, "action": p.policyAction,
			"credential_id": p.credID, "decision": p.decision.Decision,
			"policy_rule": p.decision.RuleID,
		}
		if p.result != nil {
			for k, v := range p.result {
				out[k] = v
			}
		}
		if p.failure != nil {
			return protocol.Fail(p.failure.Code, p.failure.Message)
		}
		return protocol.OK(out)
	default:
		return protocol.Fail(protocol.CodeBadArgs, "unknown confirmation operation")
	}
}
