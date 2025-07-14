package stringx

import (
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func Title(s string) string {
	c := cases.Title(language.Und)
	return c.String(s)
}

func ToUpperFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func ToLowerFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}
