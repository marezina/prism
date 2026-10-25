package detect

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExcerpt(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "Looks good, ship it.", "Looks good, ship it."},
		{"emphasis and code", "**Nice** work on `retry()` and _backoff_", "Nice work on retry() and backoff"},
		{"quote dropped", "> Why 3 retries?\n\nBecause the SLA says so.", "Because the SLA says so."},
		{"nested quote dropped", "> > old\n> new\nmine", "mine"},
		{"fence dropped", "Try this:\n```go\nx := 1\n```\nthen run", "Try this: then run"},
		{"tilde fence", "a\n~~~\ncode\n~~~\nb", "a b"},
		{"links and images", "See [the docs](https://x) ![img](https://y/z.png)", "See the docs"},
		{"html comment", "<!-- template -->\nReal text <br/>here", "Real text here"},
		{"multi-line html comment", "<!--\nhidden\n-->\nshown", "shown"},
		{"lists and headings", "## Summary\n- one\n- [x] two\n1. three", "Summary one two three"},
		{"crlf", "a\r\nb", "a b"},
		{"empty", "", ""},
		{"only quote", "> quoted", ""},
	}
	for _, tt := range tests {
		if got := Excerpt(tt.in); got != tt.want {
			t.Errorf("%s: Excerpt(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestExcerptTruncates(t *testing.T) {
	long := strings.Repeat("word ", 100)
	got := Excerpt(long)
	if n := utf8.RuneCountInString(got); n > ExcerptLen {
		t.Errorf("excerpt has %d runes", n)
	}
	if !strings.HasSuffix(got, "word…") {
		t.Errorf("want a word-boundary cut with ellipsis, got %q", got[len(got)-20:])
	}

	noSpaces := strings.Repeat("ž", 300)
	got = Excerpt(noSpaces)
	if n := utf8.RuneCountInString(got); n != ExcerptLen || !strings.HasSuffix(got, "…") || !utf8.ValidString(got) {
		t.Errorf("rune-safe cut failed: %d runes", n)
	}
}

func TestMentions(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"@me please look", true},
		{"thanks @ME.", true},
		{"(@me)", true},
		{"cc: @me, @bob", true},
		{"@meow", false},
		{"@me-too", false},
		{"email me@example.com", false},
		{"@me/team is a team", false},
		{"@@me", false},
		{"use `@me` in code", false},
		{"```\n@me\n```", false},
		{"> @me said", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := mentions(tt.body, "me"); got != tt.want {
			t.Errorf("mentions(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
	if mentions("@me", "") {
		t.Error("empty login must never match")
	}
}

func TestQuotes(t *testing.T) {
	mine := []string{"I think we should **retry** three times.\nAlso log it.", "> someone else's words\nok"}
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"full line", "> I think we should retry three times.\n\nAgreed", true},
		{"whitespace and emphasis ignored", ">   I think  we should *retry* three times.", true},
		{"case ignored", "> ALSO LOG IT.", true},
		{"second line", "> Also log it.", true},
		{"partial line", "> we should retry", false},
		{"no quote", "I think we should retry three times.", false},
		{"my own quote of someone else doesn't count", "> someone else's words", false},
		{"nested", "> > Also log it.", true},
		{"quoted empty line", ">\n> \nreply", false},
	}
	for _, tt := range tests {
		if got := quotes(tt.body, mine); got != tt.want {
			t.Errorf("%s: quotes = %v, want %v", tt.name, got, tt.want)
		}
	}
}
