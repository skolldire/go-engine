package text

import "strings"

func CleanSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }
func Truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}
func JoinNonEmpty(sep string, parts ...string) string {
	nonempty := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			nonempty = append(nonempty, part)
		}
	}
	return strings.Join(nonempty, sep)
}
func RemoveChars(s string, chars ...string) string {
	for _, part := range chars {
		s = strings.ReplaceAll(s, part, "")
	}
	return s
}
