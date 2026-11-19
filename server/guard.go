package main

import (
	"fmt"
	"regexp"
	"strings"
)

// First line of defense only. Real guarantees: read-only DB user + READ ONLY transaction in main.go.
var allowed = map[string]bool{"select": true, "show": true, "describe": true, "desc": true, "explain": true, "with": true}

// Whitespace incl. \v and unicode spaces (RE2's \s has neither), so they can't hide a keyword from the guard.
const ws = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

// Credentials never leave the server: queries naming these are refused, and redact() masks them in results (SELECT *).
// Column names: passwords/passcodes/PINs, tokens, API headers/payloads.
var secretColumn = regexp.MustCompile(`(?i)pass(word|code|key)|(^|_)pin($|_)|token|secret|api_?key|private_?key|credential|session_id|request_header|response_header|payload`)

// Whole tables whose rows are secrets: DB-engine schemas always, app tables from BLOCKED_TABLES (see blockTables).
var secretTables = map[string]bool{"mysql": true, "performance_schema": true, "sys": true, "processlist": true}

// blockTables adds comma-separated table names (case-insensitive) to secretTables.
func blockTables(list string) {
	for _, t := range strings.Split(list, ",") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			secretTables[t] = true
		}
	}
}

const hidden = "[hidden]"

var (
	bcrypt       = regexp.MustCompile(`^\$2[aby]\$\d\d\$`)
	reBlock      = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reLine       = regexp.MustCompile(`(--[` + ws + `]|#)[^\n]*`)
	reTrim       = regexp.MustCompile(`^[` + ws + `]+|[` + ws + `]+$`)
	reTrailSemi  = regexp.MustCompile(`;[` + ws + `]*$`)
	reFirst      = regexp.MustCompile(`^[^` + ws + `]+`)
	reOutfile    = regexp.MustCompile(`(?i)\binto[` + ws + `]+(outfile|dumpfile)\b`)
	reStrings    = regexp.MustCompile(`'(?:[^'\\]|\\[^\n\r\x{2028}\x{2029}])*'|"(?:[^"\\]|\\[^\n\r\x{2028}\x{2029}])*"`)
	reIdentifier = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_$]*`)
)

// redact masks secret columns and bcrypt hashes in result rows.
func redact(cols []string, rows [][]any) [][]any {
	for _, r := range rows {
		for i, k := range cols {
			if s, ok := r[i].(string); secretColumn.MatchString(k) || (ok && bcrypt.MatchString(s)) {
				r[i] = hidden
			}
		}
	}
	return rows
}

// checkReadOnly returns "" when the query may run, else the refusal text.
func checkReadOnly(sql string) string {
	s := reBlock.ReplaceAllString(sql, " ")
	s = reLine.ReplaceAllString(s, " ")
	s = reTrim.ReplaceAllString(s, "")
	s = reTrailSemi.ReplaceAllString(s, "")
	if s == "" {
		return "Empty query."
	}
	if strings.Contains(s, ";") {
		return "Only one statement per query is allowed."
	}
	first := strings.ToLower(reFirst.FindString(s))
	if !allowed[first] {
		return fmt.Sprintf("Only read queries (SELECT/SHOW/DESCRIBE/EXPLAIN) are allowed, not %s.", strings.ToUpper(first))
	}
	if reOutfile.MatchString(s) {
		return "INTO OUTFILE/DUMPFILE is not allowed."
	}
	// Identifiers only: drop string literals so WHERE name LIKE '%token%' still works.
	ids := reIdentifier.FindAllString(strings.ReplaceAll(reStrings.ReplaceAllString(s, " "), "`", ""), -1)
	for _, w := range ids {
		if secretColumn.MatchString(w) || secretTables[strings.ToLower(w)] {
			return "Access denied: credentials (passwords, PINs, tokens, API keys) and protected tables are never available. Answer without them and tell the user this data is protected."
		}
	}
	return ""
}
