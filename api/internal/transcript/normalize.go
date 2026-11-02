package transcript

import (
	"html"
	"strings"
)

// normalizeText strips presentation-only markup, decodes HTML entities and
// collapses whitespace while preserving spoken words and punctuation.
func normalizeText(text string) string {
	text = stripTags(text)
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, " ", " ")
	return strings.Join(strings.Fields(text), " ")
}

// stripTags removes markup such as `<c>`, `<v Speaker>`, `<i>`, `<br>` etc.
func stripTags(text string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '<':
			depth++
		case text[i] == '>' && depth > 0:
			depth--
		case depth == 0:
			b.WriteByte(text[i])
		}
	}
	return b.String()
}
