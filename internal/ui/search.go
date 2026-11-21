package ui

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// searchTerms holds a regex per space-separated word of a log search. A commit matches when every word does.
type searchTerms []*regexp.Regexp

// Smart case per word, as fzf --smart-case does. nil when the pattern has no words.
func compileSearch(pattern string) (searchTerms, error) {
	words := strings.Fields(pattern)
	if len(words) == 0 {
		return nil, nil
	}
	terms := searchTerms{}
	for _, word := range words {
		var expression string
		if strings.ToLower(word) == word {
			expression = "(?i)" + word
		} else {
			expression = word
		}
		term, err := regexp.Compile(expression)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, nil
}

func (s searchTerms) matches(commit gitrepo.Commit) bool {
	fields := []string{commit.Hash, strings.Join(commit.Refs, ", "), commit.Author, commit.Message}
	for _, term := range s {
		if !slices.ContainsFunc(fields, term.MatchString) {
			return false
		}
	}
	return true
}

func (s searchTerms) highlight(text string, style lipgloss.Style) string {
	spans := [][]int{}
	for _, term := range s {
		spans = append(spans, term.FindAllStringIndex(text, -1)...)
	}
	return highlightSpans(text, spans, style)
}

// subsequenceWords holds the space-separated words of the overview's filter. A word matches a text that holds its
// characters in order, not necessarily next to each other.
type subsequenceWords []string

func (w subsequenceWords) matchEvery(fields []string) bool {
	for _, word := range w {
		matchesWord := func(field string) bool { return subsequenceSpans(word, field) != nil }
		if !slices.ContainsFunc(fields, matchesWord) {
			return false
		}
	}
	return true
}

func (w subsequenceWords) highlight(text string, style lipgloss.Style) string {
	spans := [][]int{}
	for _, word := range w {
		spans = append(spans, subsequenceSpans(word, text)...)
	}
	return highlightSpans(text, spans, style)
}

// subsequenceSpans takes the first fit for each character of word in turn, with compileSearch's smart case.
func subsequenceSpans(word string, text string) [][]int {
	wanted := []rune(word)
	ignoreCase := strings.ToLower(word) == word
	spans := [][]int{}
	for index := 0; index < len(text) && len(spans) < len(wanted); {
		character, size := utf8.DecodeRuneInString(text[index:])
		if ignoreCase {
			character = unicode.ToLower(character)
		}
		if character == wanted[len(spans)] {
			spans = append(spans, []int{index, index + size})
		}
		index += size
	}
	if len(spans) < len(wanted) {
		return nil
	}
	return spans
}

// Sanitizes each run separately, since sanitizing changes byte offsets the matches were found at.
func highlightSpans(text string, spans [][]int, style lipgloss.Style) string {
	matched := make([]bool, len(text))
	for _, span := range spans {
		for index := span[0]; index < span[1]; index++ {
			matched[index] = true
		}
	}
	var rendered strings.Builder
	start := 0
	for start < len(text) {
		end := start
		for end < len(text) && matched[end] == matched[start] {
			end++
		}
		if matched[start] {
			rendered.WriteString(searchMatchStyle.Render(sanitizeText(text[start:end])))
		} else {
			rendered.WriteString(style.Render(sanitizeText(text[start:end])))
		}
		start = end
	}
	return rendered.String()
}
