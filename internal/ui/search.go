package ui

import (
	"regexp"
	"slices"
	"strings"

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

// Sanitizes each run separately, since sanitizing changes byte offsets the matches were found at.
func (s searchTerms) highlight(text string, style lipgloss.Style) string {
	matched := make([]bool, len(text))
	for _, term := range s {
		for _, span := range term.FindAllStringIndex(text, -1) {
			for index := span[0]; index < span[1]; index++ {
				matched[index] = true
			}
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
