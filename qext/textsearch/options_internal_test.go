package textsearch

import (
	"strings"
	"testing"
)

func TestHighlightSmallMaxWords(t *testing.T) {
	got := Highlight{MaxWords: 5}.options()
	if !strings.Contains(got, "MaxWords=5") || !strings.Contains(got, "MinWords=4") {
		t.Errorf("options = %q: a small MaxWords must lower MinWords below it", got)
	}
	if got := (Highlight{MaxWords: 50}).options(); strings.Contains(got, "MinWords") {
		t.Errorf("options = %q: a large MaxWords needs no MinWords", got)
	}
}
