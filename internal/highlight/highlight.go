package highlight

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

var chromaStyle = styles.Get("rose-pine")

// FileDiff colors the tokens of every line with the lexer path's file name matches, and returns fileDiff as it is
// when none does. Each hunk's old and new side are lexed as one block, so a token can span lines.
func FileDiff(path string, fileDiff diff.FileDiff) (diff.FileDiff, error) {
	matched := lexers.Match(path)
	if matched == nil || fileDiff.Binary {
		return fileDiff, nil
	}
	lexer := chroma.Coalesce(matched)
	hunks := []diff.Hunk{}
	for _, hunk := range fileDiff.Hunks {
		colored, err := hunkTokens(lexer, hunk)
		if err != nil {
			return diff.FileDiff{}, fmt.Errorf("highlight %s as %s: %w", path, lexer.Config().Name, err)
		}
		hunks = append(hunks, colored)
	}
	colored := fileDiff
	colored.Hunks = hunks
	return colored, nil
}

func hunkTokens(lexer chroma.Lexer, hunk diff.Hunk) (diff.Hunk, error) {
	oldTokens, err := sideTokens(lexer, sideTexts(hunk.Rows, oldLine))
	if err != nil {
		return diff.Hunk{}, fmt.Errorf("old side of %s: %w", hunk.Header, err)
	}
	newTokens, err := sideTokens(lexer, sideTexts(hunk.Rows, newLine))
	if err != nil {
		return diff.Hunk{}, fmt.Errorf("new side of %s: %w", hunk.Header, err)
	}
	rows := []diff.Row{}
	oldIndex := 0
	newIndex := 0
	for _, row := range hunk.Rows {
		var left, right diff.Line
		left, oldIndex = withNextTokens(row.Old, oldTokens, oldIndex)
		right, newIndex = withNextTokens(row.New, newTokens, newIndex)
		rows = append(rows, diff.Row{Old: left, New: right})
	}
	colored := hunk
	colored.Rows = rows
	return colored, nil
}

func oldLine(row diff.Row) diff.Line {
	return row.Old
}

func newLine(row diff.Row) diff.Line {
	return row.New
}

func sideTexts(rows []diff.Row, side func(diff.Row) diff.Line) []string {
	texts := []string{}
	for _, row := range rows {
		if side(row).Kind != diff.LineBlank {
			texts = append(texts, side(row).Text)
		}
	}
	return texts
}

// withNextTokens gives a line that is not blank the tokens at next, and returns where the following line's tokens are.
func withNextTokens(line diff.Line, sideTokens [][]diff.Token, next int) (diff.Line, int) {
	if line.Kind == diff.LineBlank {
		return line, next
	}
	colored := line
	colored.Tokens = sideTokens[next]
	return colored, next + 1
}

// A token in the style's plain text color is left out, so the line keeps the foreground of its kind.
func sideTokens(lexer chroma.Lexer, texts []string) ([][]diff.Token, error) {
	tokens := make([][]diff.Token, len(texts))
	if len(texts) == 0 {
		return tokens, nil
	}
	// EnsureLF off: a CR left in a line must stay, or the offsets would drift from Text.
	iterator, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, strings.Join(texts, "\n"))
	if err != nil {
		return nil, fmt.Errorf("tokenise: %w", err)
	}
	textColor := chromaStyle.Get(chroma.Text).Colour
	textIndex := 0
	offset := 0
	for _, token := range iterator.Tokens() {
		color := chromaStyle.Get(token.Type).Colour
		for index, tokenLine := range strings.Split(token.Value, "\n") {
			if index > 0 {
				textIndex++
				offset = 0
			}
			if tokenLine == "" {
				continue
			}
			if textIndex >= len(texts) || offset+len(tokenLine) > len(texts[textIndex]) ||
				texts[textIndex][offset:offset+len(tokenLine)] != tokenLine {
				return nil, fmt.Errorf("token %q does not match line %d at byte %d", tokenLine, textIndex+1, offset)
			}
			if color.IsSet() && color != textColor {
				tokens[textIndex] = append(tokens[textIndex], diff.Token{
					Span:  diff.Span{Start: offset, End: offset + len(tokenLine)},
					Color: color.String(),
				})
			}
			offset += len(tokenLine)
		}
	}
	return tokens, nil
}
