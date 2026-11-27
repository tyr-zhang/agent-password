package executor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/tyr-zhang/agent-password/internal/store"
)

var redisReadCommands = map[string]bool{
	"GET": true, "MGET": true, "HGET": true, "HGETALL": true, "HMGET": true,
	"LRANGE": true, "LLEN": true, "SMEMBERS": true, "SISMEMBER": true,
	"ZRANGE": true, "ZSCORE": true, "TTL": true, "PTTL": true,
	"EXISTS": true, "TYPE": true, "INFO": true, "DBSIZE": true,
}

// runRedis implements the `sql` action for redis credentials with a small
// in-tree RESP2 client and a read-only command allowlist.
func (e *Executor) runRedis(ctx context.Context, s store.Secret, query string, binds []string) (map[string]any, error) {
	if s.Host == "" {
		return nil, errors.New("executor: redis credential has no host")
	}
	argv := append(strings.Fields(query), binds...)
	if len(argv) == 0 {
		return nil, errors.New("executor: empty redis command")
	}
	cmd := strings.ToUpper(argv[0])
	if !redisReadCommands[cmd] {
		return nil, fmt.Errorf("executor: redis command %s is not in the read-only allowlist", cmd)
	}
	port := s.Port
	if port == 0 {
		port = 6379
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(port))
	d := net.Dialer{Timeout: 5 * second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	r := bufio.NewReader(conn)
	if s.Password != "" {
		if err := writeRESP(conn, []string{"AUTH", s.Password}); err != nil {
			return nil, err
		}
		v, err := readRESP(r, 0)
		if err != nil {
			return nil, err
		}
		if e, ok := v.(respError); ok {
			return nil, fmt.Errorf("executor: redis AUTH: %s", e.Message)
		}
	}
	if err := writeRESP(conn, argv); err != nil {
		return nil, err
	}
	v, err := readRESP(r, 0)
	if err != nil {
		return nil, err
	}
	if re, ok := v.(respError); ok {
		return nil, fmt.Errorf("executor: redis: %s", re.Message)
	}
	out, truncated := capString(e.redactor.Redact(formatRESP(v)))
	return map[string]any{"output": out, "truncated": truncated}, nil
}

func writeRESP(w io.Writer, args []string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(a), a); err != nil {
			return err
		}
	}
	return nil
}

type respError struct{ Message string }

func readRESP(r *bufio.Reader, depth int) (any, error) {
	if depth > 16 {
		return nil, errors.New("executor: RESP nesting too deep")
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, errors.New("executor: malformed RESP reply")
	}
	body := strings.TrimRight(line[1:len(line)-2], "\r")
	switch line[0] {
	case '+':
		return body, nil
	case '-':
		return respError{Message: body}, nil
	case ':':
		n, err := strconv.ParseInt(body, 10, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, 0, min(n, 10000))
		for i := 0; i < n; i++ {
			v, err := readRESP(r, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("executor: unknown RESP type %q", line[0])
	}
}

func formatRESP(v any) string {
	switch x := v.(type) {
	case nil:
		return "(nil)"
	case []any:
		var b strings.Builder
		for i, e := range x {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(formatRESP(e))
		}
		return b.String()
	case respError:
		return x.Message
	default:
		return fmt.Sprint(x)
	}
}
