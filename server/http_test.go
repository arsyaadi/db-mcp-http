package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	h := newHandler("0123456789abcdef")
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	for _, c := range []struct {
		method, path, auth, host string
		want                     int
	}{
		{"POST", "/x", "Bearer 0123456789abcdef", "", 404},
		{"POST", "/mcp", "", "", 401},
		{"POST", "/mcp", "Bearer wrong", "", 401},
		{"GET", "/mcp", "Bearer 0123456789abcdef", "", 405},
		{"POST", "/mcp", "Bearer 0123456789abcdef", "", 200},
		{"POST", "/mcp", "Bearer 0123456789abcdef", "db.example.com", 200}, // behind a reverse proxy
	} {
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(init))
		// The SDK's rebinding check looks at the local socket address.
		r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3333}))
		if c.host != "" {
			r.Host = c.host
		}
		r.Header.Set("Authorization", c.auth)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s auth=%q host=%q: got %d want %d: %s", c.method, c.path, c.auth, c.host, w.Code, c.want, w.Body)
		}
		if c.want == 200 && !strings.Contains(w.Body.String(), `"name":"db-mcp"`) {
			t.Errorf("initialize body: %s", w.Body)
		}
	}
}
