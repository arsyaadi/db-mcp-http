package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestAllowsReads(t *testing.T) {
	for _, q := range []string{"SELECT 1", "select * from t limit 5;", "  /* hi */ SHOW TABLES", "DESCRIBE t", "WITH x AS (SELECT 1) SELECT * FROM x", "-- c\nSELECT 1"} {
		if msg := checkReadOnly(q); msg != "" {
			t.Errorf("%q blocked: %s", q, msg)
		}
	}
}

func TestBlocksWritesAndTricks(t *testing.T) {
	for _, q := range []string{"UPDATE t SET a=1", "delete from t", "SELECT 1; DROP TABLE t", "/* x */ DROP TABLE t", "SELECT * FROM t INTO OUTFILE '/tmp/x'", "", "CALL p()", "SET @a=1"} {
		if checkReadOnly(q) == "" {
			t.Errorf("%q allowed", q)
		}
	}
}

func TestBlocksCredentials(t *testing.T) {
	for _, q := range []string{"SELECT password FROM users", "SELECT u.`passcode` FROM users u", "SELECT user_pin FROM customers", "SELECT * FROM settings", "SELECT token FROM personal_access_tokens", "SELECT request_header FROM api_logs", "SELECT * FROM mysql.user", "SELECT * FROM information_schema.processlist"} {
		if checkReadOnly(q) == "" {
			t.Errorf("%q allowed", q)
		}
	}
	for _, q := range []string{"SELECT name FROM customers WHERE name LIKE '%token%'", "SELECT shipping_status FROM orders", "SELECT COUNT(*) FROM users"} {
		if msg := checkReadOnly(q); msg != "" {
			t.Errorf("%q blocked: %s", q, msg)
		}
	}
}

func TestMessages(t *testing.T) {
	for q, want := range map[string]string{
		"":                        "Empty query.",
		"SELECT 1; SELECT 2":      "Only one statement per query is allowed.",
		"drop table t":            "Only read queries (SELECT/SHOW/DESCRIBE/EXPLAIN) are allowed, not DROP.",
		"SELECT 1 INTO  dumpfile": "INTO OUTFILE/DUMPFILE is not allowed.",
	} {
		if got := checkReadOnly(q); got != want {
			t.Errorf("%q: got %q want %q", q, got, want)
		}
	}
}

func TestRedact(t *testing.T) {
	cols := []string{"id", "password", "user_pin", "note", "name"}
	got := redact(cols, [][]any{{int64(1), "x", "1", "$2y$10$abcdefghijklmnopqrstuv", "A"}})[0]
	want := []any{int64(1), hidden, hidden, hidden, "A"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("col %s: got %v want %v", cols[i], got[i], want[i])
		}
	}
}

func TestQuoteIdent(t *testing.T) {
	for in, want := range map[string]string{"a`b": "`a``b`", "db.t": "`db`.`t`", "t": "`t`", "": "``"} {
		if got := quoteIdent(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestEnvLine(t *testing.T) {
	for in, want := range map[string]string{
		"A=x":             "A=x",
		"  A = x  ":       "A=x",
		"A=x # note":      "A=x",
		"A=ab#cd":         "A=ab",
		`A="p#ss" # note`: "A=p#ss",
		"A='p#ss'":        "A=p#ss",
		`A="a\nb"`:        "A=a\nb",
		`A='a\nb'`:        `A=a\nb`,
		"export A=x":      "A=x",
		"A=":              "A=",
		"A=x=y":           "A=x=y",
		"# A=x":           "",
		"":                "",
		"no equals":       "",
	} {
		k, v, ok := envLine(in)
		got := ""
		if ok {
			got = k + "=" + v
		}
		if got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestFormat(t *testing.T) {
	cols := []string{"z", "a", "m", "n", "e", "b", "fl", "ub", "t", "f", "d", "z"} // duplicate name kept
	r := []any{
		cell("INT", int64(1)), cell("BIGINT", int64(9007199254740993)), cell("DECIMAL", []byte("1.50")), cell("VARCHAR", nil),
		cell("VARCHAR", []byte("x<y&z")), cell("BLOB", []byte{1, 2, 3}), cell("FLOAT", float32(1.1)),
		cell("UNSIGNED BIGINT", uint64(18446744073709551615)), cell("TEXT", []byte("txt")), cell("DOUBLE", 0.5),
		cell("DATETIME", []byte("2026-01-02 03:04:05")), cell("INT", int64(2)),
	}
	want := `{"columns":["z","a","m","n","e","b","fl","ub","t","f","d","z"],"rows":[[1,"9007199254740993","1.50",null,"x<y&z","<binary 3 bytes>",1.1,"18446744073709551615","txt",0.5,"2026-01-02 03:04:05",2]]}`
	if got := format(cols, [][]any{r}, 1); got != want+note {
		t.Errorf("got:\n%s", got)
	}
	got := format([]string{"a"}, [][]any{}, 250)
	if !strings.HasPrefix(got, `{"columns":["a"],"rows":[]}`+"\n\n(Truncated: 250 rows, showing 200. Use LIMIT/aggregation.)\n\n[Note:") {
		t.Errorf("got %q", got)
	}
}

func TestGuideSectionsInOrder(t *testing.T) {
	if !strings.HasPrefix(guide, "# ") {
		t.Fatal("guide must start with intro")
	}
	last := -1
	for n := 1; n <= 7; n++ {
		i := strings.Index(guide, fmt.Sprintf("\n## %d. ", n))
		if i <= last {
			t.Fatalf("section %d missing or out of order", n)
		}
		last = i
	}
}
