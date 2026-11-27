package daemon

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"

	"github.com/tyr-zhang/agent-password/internal/i18n"
	"github.com/tyr-zhang/agent-password/internal/policy"
	"github.com/tyr-zhang/agent-password/internal/protocol"
)

type pending struct {
	id            string
	pollToken     string
	nonce         []byte
	op            string
	policyAction  string
	credID        string
	args          map[string]any
	client        protocol.ClientInfo
	lang          i18n.Lang
	decision      policy.Decision
	summary       string
	expires       time.Time
	resolve       chan bool
	state         string // pending | approved | denied | timeout | done | error
	result        map[string]any
	failure       *protocol.Error
	resolvedAt    time.Time
	confirmSource string
}

type pendingRegistry struct {
	mu       sync.Mutex
	items    map[string]*pending
	confirmTTL time.Duration
}

func newPendingRegistry(ttl time.Duration) *pendingRegistry {
	return &pendingRegistry{items: map[string]*pending{}, confirmTTL: ttl}
}

func randomHex(n int) (string, []byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(b), b, nil
}

func (r *pendingRegistry) add(p *pending) error {
	id, nonce, err := randomHex(16)
	if err != nil {
		return err
	}
	token, _, err := randomHex(32)
	if err != nil {
		return err
	}
	p.id = "c_" + id
	p.pollToken = token
	p.nonce = nonce
	p.expires = time.Now().Add(r.confirmTTL)
	p.state = "pending"
	p.resolve = make(chan bool, 1)
	r.mu.Lock()
	r.items[p.id] = p
	r.mu.Unlock()
	return nil
}

func (r *pendingRegistry) get(id string) *pending {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.items[id]
}

func (r *pendingRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

func (r *pendingRegistry) fulfill(id string, approved bool, source string) (*pending, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.items[id]
	if p == nil || p.state != "pending" || time.Now().After(p.expires) {
		return p, false
	}
	p.confirmSource = source
	select {
	case p.resolve <- approved:
	default:
		return p, false
	}
	return p, true
}

func (r *pendingRegistry) setOutcome(p *pending, state string, result map[string]any, failure *protocol.Error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.state = state
	p.result = result
	p.failure = failure
	p.resolvedAt = time.Now()
}

func (r *pendingRegistry) cancelAll() {
	r.mu.Lock()
	items := make([]*pending, 0, len(r.items))
	for _, p := range r.items {
		items = append(items, p)
	}
	r.mu.Unlock()
	for _, p := range items {
		r.fulfill(p.id, false, "lock")
	}
}

func (r *pendingRegistry) cleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			for id, p := range r.items {
				if p.resolvedAt.IsZero() && time.Now().After(p.expires.Add(10*time.Minute)) {
					delete(r.items, id)
					continue
				}
				if !p.resolvedAt.IsZero() && time.Now().After(p.resolvedAt.Add(5*time.Minute)) {
					delete(r.items, id)
				}
			}
			r.mu.Unlock()
		}
	}
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
