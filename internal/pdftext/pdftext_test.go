package pdftext

import (
	"os"
	"strings"
	"testing"
)

func TestExtractSampleSOC2(t *testing.T) {
	data, err := os.ReadFile("../../testdata/vendors/northwind-soc2-type2.pdf")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 9 {
		t.Fatalf("expected 9 pages, got %d", len(pages))
	}
	all := ""
	for _, p := range pages {
		all += strings.Join(p.Lines, "\n") + "\n"
	}
	for _, want := range []string{"unqualified opinion", "CC6.2 | Access removed within one business day of termination | Exception noted", "CUEC-2", "Page 7"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q", want)
		}
	}
	if pages[2].Lines[1] != "Section 1 - Independent Service Auditor's Report" {
		t.Errorf("lines not reconstructed: %q", pages[2].Lines[:3])
	}
	if _, err := Extract([]byte("not a pdf")); err == nil {
		t.Error("garbage should error")
	}
}
