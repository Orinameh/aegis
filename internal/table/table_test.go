package table

import (
	"strings"
	"testing"
)

func TestTableRender(t *testing.T) {
	tbl := New("NAME", "SIZE")
	tbl.AddRow("a", "1")
	tbl.AddRow("longer-name", "12345")

	var sb strings.Builder
	tbl.Render(&sb)
	out := sb.String()

	if !strings.Contains(out, "NAME") || !strings.Contains(out, "longer-name") {
		t.Fatalf("expected header and rows in output:\n%s", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (header, separator, 2 rows), got %d:\n%s", len(lines), out)
	}
	// The header's first cell must be padded to align with the longest name:
	// "NAME" (4) padded to 11 (width of "longer-name") + 2-space column gap.
	wantHeader := "NAME         SIZE"
	if lines[0] != wantHeader {
		t.Fatalf("expected header %q, got %q", wantHeader, lines[0])
	}
}

func TestTableShorterRow(t *testing.T) {
	tbl := New("A", "B", "C")
	tbl.AddRow("x", "y")

	var sb strings.Builder
	tbl.Render(&sb)
	if strings.Contains(sb.String(), "??") {
		t.Fatal("unexpected placeholder chars")
	}
}
