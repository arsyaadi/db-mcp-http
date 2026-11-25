package main

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if err := loadGuide("guide.example"); err != nil {
		panic(err)
	}
	blockTables("settings,password_resets,personal_access_tokens,api_logs")
	os.Exit(m.Run())
}

func TestBlockTablesAndLoadGuide(t *testing.T) {
	blockTables(" Foo_Bar , ")
	if checkReadOnly("select * from FOO_BAR") == "" {
		t.Error("blocked table allowed")
	}
	if loadGuide(t.TempDir()) == nil {
		t.Error("empty guide dir accepted")
	}
	if _, err := os.Stat("guide/instructions.md"); err != nil { // slot as committed (README only), unless a dev filled it locally
		if err := loadGuide(""); err == nil || !strings.Contains(err.Error(), "GUIDE_DIR") {
			t.Errorf("empty built-in slot: want error naming GUIDE_DIR, got %v", err)
		}
	}
	if err := loadGuide("guide.example"); err != nil { // restore the template for the other tests
		t.Fatal(err)
	}
}

func TestFormatTruncation(t *testing.T) {
	for total, want := range map[int]string{
		200:           "",
		201:           "Truncated: 201 rows",
		countRows:     "Truncated: 1000 rows",
		countRows + 1: "more than 1000 rows",
	} {
		s := format([]string{"id"}, [][]any{{1}}, total)
		if want == "" && strings.Contains(s, "Truncated") || want != "" && !strings.Contains(s, want) {
			t.Errorf("total=%d: want %q in %q", total, want, s)
		}
	}
}
