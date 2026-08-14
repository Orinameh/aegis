package table

import (
	"io"
	"strings"
)

// Table renders an aligned text table to a plain io.Writer. It is a small
// dependency-free alternative to a tablewriter library so the tool stays
// lean; only left-alignment is supported.
type Table struct {
	headers []string
	rows    [][]string
}

// New creates a table with the given header cells.
func New(headers ...string) *Table {
	return &Table{headers: headers}
}

// AddRow appends a row. Cells can be shorter than the header; missing cells
// are rendered as empty.
func (t *Table) AddRow(cells ...string) {
	t.rows = append(t.rows, cells)
}

// Render writes the header, a separator line, and all rows to w.
func (t *Table) Render(w io.Writer) {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = len(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	writeRow := func(cells []string) {
		var sb strings.Builder
		for i := range t.headers {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			if i > 0 {
				sb.WriteString("  ")
			}
			sb.WriteString(cell)
			sb.WriteString(strings.Repeat(" ", widths[i]-len(cell)))
		}
		io.WriteString(w, strings.TrimRight(sb.String(), " ")+"\n")
	}

	writeRow(t.headers)

	sep := make([]string, len(widths))
	for i, wd := range widths {
		sep[i] = strings.Repeat("-", wd)
	}
	io.WriteString(w, strings.Join(sep, "  ")+"\n")

	for _, row := range t.rows {
		writeRow(row)
	}
}