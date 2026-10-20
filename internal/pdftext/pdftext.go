// Package pdftext extracts text from PDFs page by page, reconstructing lines
// by vertical position. Used by the fake AI provider (document assessment with
// page citations) and for search.
package pdftext

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Page is the text of one page as lines, top to bottom.
type Page struct {
	Number int
	Lines  []string
}

// Extract returns the text of every page. Encrypted or image-only PDFs yield
// empty pages rather than an error.
func Extract(data []byte) (pages []Page, err error) {
	defer func() {
		if r := recover(); r != nil { // the parser panics on some malformed files
			err = fmt.Errorf("pdf: unreadable document: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			pages = append(pages, Page{Number: i})
			continue
		}
		// Group glyph runs by baseline (rounded Y), then order each line by X.
		type run struct {
			x float64
			s string
		}
		byY := map[int][]run{}
		for _, t := range p.Content().Text {
			y := int(t.Y + 0.5)
			byY[y] = append(byY[y], run{t.X, t.S})
		}
		ys := make([]int, 0, len(byY))
		for y := range byY {
			ys = append(ys, y)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(ys)))
		var lines []string
		for _, y := range ys {
			runs := byY[y]
			sort.SliceStable(runs, func(a, b int) bool { return runs[a].x < runs[b].x })
			var b strings.Builder
			for _, r := range runs {
				b.WriteString(r.s)
			}
			if l := strings.TrimSpace(b.String()); l != "" {
				lines = append(lines, l)
			}
		}
		pages = append(pages, Page{Number: i, Lines: lines})
	}
	return pages, nil
}
