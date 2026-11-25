package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/ssh"
)

const maxRows = 200

// DB-side cap (sql_select_limit) for a SELECT without LIMIT: stops pulling a whole big table, keeps the count exact up to here.
const countRows = 1000

// Domain knowledge lives only in the guide pack: instructions.md, reminder.md, and NN-*.md sections joined in filename order.
// Built into the binary from server/guide (the slot, git-ignored except its README); GUIDE_DIR overrides it with a folder on disk (edit and restart, no rebuild).
var instructions, note, guide string

// Embeds every .md in the slot, so copy only the served files there (not sources.md/review.md). README.md guarantees a match on an empty slot.
//
//go:embed guide/*.md
var builtinGuide embed.FS

// Guide only via get_guide: clients truncate long instructions (Claude Code at 4096 chars), and a cut guide looks complete.
// note (reminder.md) is repeated on every result: server instructions alone fade in long chats.
// dir "" = the built-in pack.
func loadGuide(dir string) error {
	var fsys fs.FS = os.DirFS(dir)
	if dir == "" {
		fsys, _ = fs.Sub(builtinGuide, "guide") // valid path, cannot fail
		dir = "built-in server/guide (empty? copy a pack there and rebuild, or set GUIDE_DIR)"
	}
	read := func(name string) (string, error) {
		b, err := fs.ReadFile(fsys, name)
		s := strings.TrimSpace(string(b))
		if err != nil || s == "" {
			return "", fmt.Errorf("guide dir %q: %s missing or empty", dir, name)
		}
		return s, nil
	}
	ins, err := read("instructions.md")
	if err != nil {
		return err
	}
	rem, err := read("reminder.md")
	if err != nil {
		return err
	}
	files, _ := fs.Glob(fsys, "[0-9]*.md") // sorted; pattern is valid
	if len(files) == 0 {
		return fmt.Errorf("guide dir %q: no NN-*.md sections", dir)
	}
	parts := make([]string, len(files))
	for i, f := range files {
		if parts[i], err = read(f); err != nil {
			return err
		}
	}
	instructions, note, guide = ins, "\n\n"+rem, strings.Join(parts, "\n\n")+"\n"
	return nil
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// loadEnv reads KEY=VALUE lines from path; variables already in the environment win.
func loadEnv(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := envLine(l); ok {
			if _, set := os.LookupEnv(k); !set {
				os.Setenv(k, v)
			}
		}
	}
}

// envLine parses one KEY=VALUE line: optional "export ", quoted values keep '#', unquoted values end at '#'.
// ponytail: no multiline values; add if a key ever needs one.
func envLine(l string) (k, v string, ok bool) {
	l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "export "))
	k, v, ok = strings.Cut(l, "=")
	if l == "" || l[0] == '#' || !ok {
		return "", "", false
	}
	k, v = strings.TrimSpace(k), strings.TrimSpace(v)
	if v != "" && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			if v[0] == '"' {
				return k, strings.ReplaceAll(v[1:end+1], `\n`, "\n"), true
			}
			return k, v[1 : end+1], true
		}
	}
	if i := strings.IndexByte(v, '#'); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return k, v, true
}

// ---- SSH tunnel: mysql dials DB_HOST:DB_PORT (as seen from the SSH server) through one cached client.

var (
	sshMu     sync.Mutex
	sshClient *ssh.Client
	sshCfg    *ssh.ClientConfig
	sshAddr   string
)

func getSSH() (*ssh.Client, error) {
	sshMu.Lock()
	defer sshMu.Unlock()
	if sshClient != nil {
		return sshClient, nil
	}
	c, err := dialSSH()
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %w", err)
	}
	sshClient = c
	go func() { c.Wait(); dropSSH(c) }() // reconnect lazily on next query
	go func() {
		for {
			time.Sleep(30 * time.Second)
			// No reply in 90s (~3 missed keepalives) = dead link; SendRequest alone would block forever.
			t := time.AfterFunc(90*time.Second, func() { c.Close() })
			_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
			t.Stop()
			if err != nil {
				c.Close()
				return
			}
		}
	}()
	return c, nil
}

func dialSSH() (*ssh.Client, error) {
	conn, err := net.DialTimeout("tcp", sshAddr, sshCfg.Timeout)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(sshCfg.Timeout)) // covers the handshake too
	sc, chans, reqs, err := ssh.NewClientConn(conn, sshAddr, sshCfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return ssh.NewClient(sc, chans, reqs), nil
}

func dropSSH(c *ssh.Client) {
	sshMu.Lock()
	if sshClient == c {
		sshClient = nil
	}
	sshMu.Unlock()
	c.Close()
}

func dialDB(ctx context.Context, addr string) (net.Conn, error) {
	c, err := getSSH()
	if err != nil {
		return nil, err
	}
	conn, err := c.DialContext(ctx, "tcp", addr)
	if err != nil {
		// Only a dead link drops the shared tunnel, not one cancelled request or a refused channel.
		var oce *ssh.OpenChannelError
		if !errors.Is(err, context.Canceled) && !errors.As(err, &oce) {
			dropSSH(c)
		}
		return nil, fmt.Errorf("SSH connection failed: %w", err)
	}
	return conn, nil
}

// ---- DB

var db *sql.DB

// ponytail: one connection per query (no idle pool: pooled conns die silently when the tunnel reconnects); pool if it ever gets busy.
func run(ctx context.Context, q string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second) // covers MySQL handshake, and MariaDB which ignores max_execution_time
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.ExecContext(ctx, "SET SESSION max_execution_time = 30000") // MySQL only; MariaDB ignores
	conn.ExecContext(ctx, fmt.Sprintf("SET SESSION sql_select_limit = %d", countRows+1))
	if _, err := conn.ExecContext(ctx, "START TRANSACTION READ ONLY"); err != nil {
		return "", err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	rs, err := conn.QueryContext(ctx, q)
	if err != nil {
		return "", err
	}
	defer rs.Close()
	cts, err := rs.ColumnTypes()
	if err != nil {
		return "", err
	}
	cols := make([]string, len(cts))
	for i, ct := range cts {
		cols[i] = ct.Name()
	}
	kept := [][]any{}
	total := 0
	for rs.Next() {
		vals := make([]any, len(cts))
		ptrs := make([]any, len(cts))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return "", err
		}
		total++
		if total > maxRows {
			continue
		}
		for i, ct := range cts {
			vals[i] = cell(ct.DatabaseTypeName(), vals[i])
		}
		kept = append(kept, vals)
	}
	if err := rs.Err(); err != nil {
		return "", err
	}
	return format(cols, kept, total), nil
}

// cell maps a driver value to JSON: BIGINT/DECIMAL and dates as strings (no float precision loss), binary as a size.
// The driver already parses ints/floats in the text protocol; everything else arrives as []byte.
func cell(typ string, v any) any {
	switch x := v.(type) {
	case int64:
		if strings.HasSuffix(typ, "BIGINT") {
			return strconv.FormatInt(x, 10)
		}
		return x
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
		return f
	case float64:
		return x
	case []byte:
		switch typ {
		case "BLOB", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB", "BINARY", "VARBINARY", "BIT", "GEOMETRY":
			return fmt.Sprintf("<binary %d bytes>", len(x))
		}
		return string(x) // DECIMAL, dates, text, JSON, ...
	}
	return nil
}

// Column names once, then one array per row: keeps column order and duplicate names, fewer tokens than objects.
func format(cols []string, rows [][]any, total int) string {
	var b strings.Builder
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.Encode(struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}{cols, redact(cols, rows)})
	s := strings.TrimSuffix(b.String(), "\n")
	if total > countRows {
		s += fmt.Sprintf("\n\n(Truncated: more than %d rows, showing %d. Result is INCOMPLETE; use aggregation or COUNT(*).)", countRows, maxRows)
	} else if total > maxRows {
		s += fmt.Sprintf("\n\n(Truncated: %d rows, showing %d. Use LIMIT/aggregation.)", total, maxRows)
	}
	return s + note
}

// quoteIdent: split on ".", backtick each part, double inner backticks (so "db.t" works and nothing escapes the quotes).
func quoteIdent(s string) string {
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = "`" + strings.ReplaceAll(p, "`", "``") + "`"
	}
	return strings.Join(parts, ".")
}

// ---- MCP

const noArgs = `{"type":"object","properties":{}}`

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: msg}}, IsError: true}
}

func okResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func newServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: getenv("MCP_NAME", "db-mcp"), Version: "0.1.0"}, &mcp.ServerOptions{
		Instructions: instructions,
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})
	tools := []struct{ name, desc, schema string }{
		{"get_guide", "Guide for this database: domain, flows, status meanings, column meanings, formulas, analysis playbook, answer rules. REQUIRED: call once at the start of every conversation, before any other tool. It is not in the server instructions.", noArgs},
		{"list_tables", "List all tables/views in the database.", noArgs},
		{"describe_table", "Show a table's columns, types, and comments. Use before writing a query.", `{"type":"object","properties":{"table":{"type":"string"}},"required":["table"]}`},
		{"query", fmt.Sprintf("Run one read query (SELECT/SHOW/DESCRIBE/EXPLAIN). Read-only, max %d rows, 30s timeout. Always use LIMIT or aggregation. Use the status meanings and formulas from get_guide. Results are for you to analyze, not to paste raw to the user.", maxRows), `{"type":"object","properties":{"sql":{"type":"string"}},"required":["sql"]}`},
	}
	for _, t := range tools {
		name := t.name
		s.AddTool(&mcp.Tool{
			Name: name, Description: t.desc, InputSchema: json.RawMessage(t.schema),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			fmt.Printf("%s tool=%s\n", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), name)
			return call(ctx, name, req.Params.Arguments), nil
		})
	}
	return s
}

func call(ctx context.Context, name string, raw json.RawMessage) *mcp.CallToolResult {
	var args map[string]any
	json.Unmarshal(raw, &args) // empty/invalid -> nil args -> per-tool error
	var out string
	var err error
	switch name {
	case "get_guide":
		return okResult(guide)
	case "list_tables":
		out, err = run(ctx, "SHOW FULL TABLES")
	case "describe_table":
		t, ok := args["table"].(string)
		if !ok {
			return errResult("Error: table must be a string")
		}
		out, err = run(ctx, "SHOW FULL COLUMNS FROM "+quoteIdent(t))
	case "query":
		q, _ := args["sql"].(string)
		if msg := checkReadOnly(q); msg != "" {
			return errResult(msg)
		}
		out, err = run(ctx, q)
	}
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) {
			err = errors.New(me.Message) // drop the "Error 1146 (42S02):" prefix
		}
		return errResult("Error: " + err.Error())
	}
	return okResult(out)
}

func main() {
	loadEnv(".env")
	if err := loadGuide(os.Getenv("GUIDE_DIR")); err != nil {
		fail(err.Error())
	}
	if _, ok := os.LookupEnv("BLOCKED_TABLES"); !ok {
		fail("BLOCKED_TABLES must be set in .env (comma-separated table names, may be empty)")
	}
	blockTables(os.Getenv("BLOCKED_TABLES"))
	host, port, token := getenv("HOST", "127.0.0.1"), getenv("PORT", "3333"), os.Getenv("MCP_TOKEN")
	if len(token) < 16 {
		fail("MCP_TOKEN must be set (at least 16 chars). Generate one: openssl rand -hex 32")
	}
	// No SSH_HOST = connect to DB_HOST:DB_PORT directly, no tunnel.
	useSSH := os.Getenv("SSH_HOST") != ""
	required := []string{"DB_ENV", "DB_USER", "DB_PASSWORD"}
	if useSSH {
		required = append(required, "SSH_USER", "SSH_KEY_PATH")
	}
	for _, k := range required {
		if os.Getenv(k) == "" {
			fail(k + " must be set in .env")
		}
	}
	// Environment comes from the deployment, never the guide: same pack serves dev and production.
	env := "Database environment: " + os.Getenv("DB_ENV") + "."
	instructions, guide = env+" "+instructions, env+"\n\n"+guide
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.DBName = os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME")
	cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(getenv("DB_HOST", "127.0.0.1"), getenv("DB_PORT", "3306"))
	cfg.Timeout = 15 * time.Second
	if useSSH {
		setupSSH()
		mysql.RegisterDialContext("ssh", dialDB)
		cfg.Net = "ssh"
	}
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		fail(err.Error())
	}
	db = sql.OpenDB(conn)
	db.SetMaxIdleConns(0)

	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf("%s MCP listening on http://%s:%s/mcp\n", getenv("MCP_NAME", "db-mcp"), host, port)
	fail((&http.Server{Handler: newHandler(token), ReadHeaderTimeout: 10 * time.Second}).Serve(ln).Error())
}

func setupSSH() {
	keyPath := os.Getenv("SSH_KEY_PATH")
	if strings.HasPrefix(keyPath, "~/") {
		keyPath = os.Getenv("HOME") + keyPath[1:]
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		fail("Cannot read SSH_KEY_PATH: " + err.Error())
	}
	var signer ssh.Signer
	if pass := os.Getenv("SSH_PASSPHRASE"); pass != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(pass))
	} else {
		signer, err = ssh.ParsePrivateKey(keyBytes)
	}
	if err != nil {
		fail("Cannot parse SSH_KEY_PATH: " + err.Error())
	}
	sshAddr = net.JoinHostPort(os.Getenv("SSH_HOST"), getenv("SSH_PORT", "22"))
	sshCfg = &ssh.ClientConfig{
		User: os.Getenv("SSH_USER"),
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// ponytail: host key not verified; known_hosts via SSH_KNOWN_HOSTS if the SSH host is ever untrusted.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
}

// ponytail: static bearer token; claude.ai custom connectors need OAuth instead — add before exposing publicly.
func newHandler(token string) http.Handler {
	// Stateless: one shared server, no sessions, plain JSON responses (no SSE).
	// Localhost (DNS-rebinding) protection off: it 403s requests via a reverse proxy, and the bearer token already blocks rebinding.
	srv := newServer()
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
	expected := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/mcp" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), expected) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// No server-initiated SSE stream (GET) or sessions (DELETE).
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
}
