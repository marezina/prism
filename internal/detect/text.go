package detect

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ExcerptLen is the maximum excerpt length in runes (ARCHITECTURE §4).
const ExcerptLen = 200

// mdLine is one markdown line with its role.
type mdLine struct {
	text   string
	quoted bool // inside a "> " block
	code   bool // inside a ``` / ~~~ fence, or the fence line itself
}

// lines splits markdown into lines, marking quotes and code fences.
func lines(md string) []mdLine {
	var out []mdLine
	fence := ""
	for _, raw := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(raw)
		if fence != "" {
			out = append(out, mdLine{text: raw, code: true})
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			out = append(out, mdLine{text: raw, code: true})
			continue
		}
		if q, ok := strings.CutPrefix(trimmed, ">"); ok {
			for {
				q = strings.TrimSpace(q)
				next, more := strings.CutPrefix(q, ">")
				if !more {
					break
				}
				q = next
			}
			out = append(out, mdLine{text: q, quoted: true})
			continue
		}
		out = append(out, mdLine{text: raw})
	}
	return out
}

var (
	inlineCode = regexp.MustCompile("`[^`]*`")
	image      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	link       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	htmlTag    = regexp.MustCompile(`<!--[\s\S]*?-->|</?[a-zA-Z][^>]*>`)
	listMarker = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?`)
	heading    = regexp.MustCompile(`^\s*#{1,6}\s+`)
	emphasis   = regexp.MustCompile(`(\*\*|__|\*|_|~~)`)
	spaces     = regexp.MustCompile(`\s+`)
)

// plain strips markdown from one line of text.
func plain(s string) string {
	s = image.ReplaceAllString(s, "")
	s = link.ReplaceAllString(s, "$1")
	s = htmlTag.ReplaceAllString(s, "")
	s = listMarker.ReplaceAllString(s, "")
	s = heading.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "`", "")
	s = emphasis.ReplaceAllString(s, "")
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

// Excerpt turns a markdown body into ≤ ExcerptLen runes of plain text,
// leaving out quoted blocks and code fences.
func Excerpt(md string) string {
	md = htmlTag.ReplaceAllString(md, "") // multi-line HTML comments first
	var parts []string
	for _, l := range lines(md) {
		if l.quoted || l.code {
			continue
		}
		if p := plain(l.text); p != "" {
			parts = append(parts, p)
		}
	}
	return truncate(strings.Join(parts, " "), ExcerptLen)
}

// truncate cuts s to n runes, preferring a word boundary, and adds "…".
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n-1]
	if i := strings.LastIndexFunc(string(r), unicode.IsSpace); i > len(string(r))/2 {
		return strings.TrimRightFunc(string(r)[:i], unicode.IsSpace) + "…"
	}
	return strings.TrimRightFunc(string(r), unicode.IsSpace) + "…"
}

// normalizeLine prepares a line for quote matching: markdown emphasis and
// whitespace differences are ignored (event-detector skill).
func normalizeLine(s string) string {
	return strings.ToLower(plain(s))
}

// quotedLines returns the normalized, non-empty lines of all quote blocks.
func quotedLines(md string) []string {
	var out []string
	for _, l := range lines(md) {
		if l.quoted {
			if n := normalizeLine(l.text); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// ownLines returns the normalized, non-empty lines a comment author wrote
// themselves (not quoted from someone else).
func ownLines(md string) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines(md) {
		if l.quoted {
			continue
		}
		if n := normalizeLine(l.text); n != "" {
			out[n] = true
		}
	}
	return out
}

// quotes reports whether md quotes at least one full line of any of mine.
func quotes(md string, mine []string) bool {
	q := quotedLines(md)
	if len(q) == 0 {
		return false
	}
	for _, body := range mine {
		own := ownLines(body)
		for _, line := range q {
			if own[line] {
				return true
			}
		}
	}
	return false
}

// mentionableText drops code fences, quoted blocks and inline code, where an
// @login is not a mention.
func mentionableText(md string) string {
	var b strings.Builder
	for _, l := range lines(md) {
		if l.quoted || l.code {
			continue
		}
		b.WriteString(inlineCode.ReplaceAllString(l.text, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

// mentions reports whether md @mentions login (case-insensitive, word
// boundaries; "@login/team" is a team mention, not a user mention).
func mentions(md, login string) bool {
	if login == "" {
		return false
	}
	re := regexp.MustCompile(`(?i)(?:^|[^a-z0-9_@./-])@` + regexp.QuoteMeta(login) + `(?:$|[^a-z0-9_/-])`)
	return re.MatchString(mentionableText(md))
}
