package executor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/tyr-zhang/agent-password/internal/store"
)

func (e *Executor) runSQL(ctx context.Context, cred *store.Credential, args map[string]any) (map[string]any, error) {
	if cred.Kind != store.KindServer {
		return nil, errors.New("executor: sql requires a server credential")
	}
	s := cred.Secret
	query := stringArg(args, "query")
	if query == "" {
		return nil, errors.New("executor: empty SQL query")
	}
	binds := stringsArg(args, "args")
	params := make([]any, len(binds))
	for i := range binds {
		params[i] = binds[i]
	}
	switch s.Protocol {
	case "redis":
		return e.runRedis(ctx, s, query, binds)
	case "mysql":
		return e.querySQL(ctx, "mysql", mysqlDSN(s), query, params)
	case "postgres", "postgresql":
		return e.querySQL(ctx, "pgx", postgresDSN(s), query, params)
	default:
		return nil, fmt.Errorf("executor: unsupported sql protocol %q", s.Protocol)
	}
}

func mysqlDSN(s store.Secret) string {
	port := s.Port
	if port == 0 {
		port = 3306
	}
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?multiStatements=false&timeout=5s",
		s.ServerUser, s.Password, net.JoinHostPort(s.Host, strconv.Itoa(port)), s.Database)
}

func postgresDSN(s store.Secret) string {
	port := s.Port
	if port == 0 {
		port = 5432
	}
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(s.ServerUser, s.Password),
		Host:     net.JoinHostPort(s.Host, strconv.Itoa(port)),
		Path:     "/" + s.Database,
		RawQuery: "sslmode=require&connect_timeout=5",
	}
	return u.String()
}

func (e *Executor) querySQL(ctx context.Context, driver, dsn, query string, params []any) (map[string]any, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetConnMaxLifetime(time.Minute)
	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("executor: query failed: %w", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	size, truncated := 0, false
	for rows.Next() {
		if len(result) >= MaxRows {
			truncated = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				text, cut := capString(string(e.redactor.RedactBytes(v)))
				row[c] = text
				truncated = truncated || cut
				size += len(text)
			case string:
				text, cut := capString(e.redactor.Redact(v))
				row[c] = text
				truncated = truncated || cut
				size += len(text)
			case nil:
				row[c] = nil
			default:
				text, cut := capString(e.redactor.Redact(fmt.Sprint(v)))
				row[c] = text
				truncated = truncated || cut
				size += len(text)
			}
		}
		result = append(result, row)
		if size > MaxOutputBytes {
			truncated = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"rows": result, "row_count": len(result), "truncated": truncated}, nil
}
