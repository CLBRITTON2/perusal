package highlight

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

var style = styles.Get("rose-pine")

// FileDiff colors the tokens of every line with the lexer path's file name matches, and returns fileDiff as it is
// when none does. Each hunk's old and new side are lexed as one block, so a token can span lines.
func FileDiff(path string, fileDiff diff.FileDiff) (diff.FileDiff, error) {
	lexer := lexers.Match(path)
	if lexer == nil || fileDiff.Binary {
		return fileDiff, nil
	}
	hunks := []diff.Hunk{}
	for _, hunk := range fileDiff.Hunks {
		colored, err := hunkTokens(chroma.Coalesce(lexer), hunk)
		if err != nil {
			return diff.FileDiff{}, fmt.Errorf("highlight %s as %s: %w", path, lexer.Config().Name, err)
		}
		hunks = append(hunks, colored)
	}
	return diff.FileDiff{Hunks: hunks, Binary: fileDiff.Binary, Change: fileDiff.Change}, nil
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
		var left diff.Line
		if row.Left.Kind == diff.LineBlank {
			left = row.Left
		} else {
			left = withTokens(row.Left, oldTokens[oldIndex])
			oldIndex++
		}
		var right diff.Line
		if row.Right.Kind == diff.LineBlank {
			right = row.Right
		} else {
			right = withTokens(row.Right, newTokens[newIndex])
			newIndex++
		}
		rows = append(rows, diff.Row{Left: left, Right: right})
	}
	return diff.Hunk{Header: hunk.Header, Rows: rows}, nil
}

func oldLine(row diff.Row) diff.Line {
	return row.Left
}

func newLine(row diff.Row) diff.Line {
	return row.Right
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

func withTokens(line diff.Line, tokens []diff.Token) diff.Line {
	return diff.Line{Kind: line.Kind, Number: line.Number, Text: line.Text, Changed: line.Changed, Tokens: tokens}
}

// sideTokens returns one token list per text. A token in the style's plain text color is left out, so the line
// keeps the foreground of its kind.
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
	plain := style.Get(chroma.Text).Colour
	line := 0
	offset := 0
	for _, token := range iterator.Tokens() {
		colour := style.Get(token.Type).Colour
		for index, piece := range strings.Split(token.Value, "\n") {
			if index > 0 {
				line++
				offset = 0
			}
			if piece == "" {
				continue
			}
			if line >= len(texts) || offset+len(piece) > len(texts[line]) || texts[line][offset:offset+len(piece)] != piece {
				return nil, fmt.Errorf("token %q does not match line %d at byte %d", piece, line+1, offset)
			}
			if colour.IsSet() && colour != plain {
				tokens[line] = append(tokens[line], diff.Token{
					Span:  diff.Span{Start: offset, End: offset + len(piece)},
					Color: colour.String(),
				})
			}
			offset += len(piece)
		}
	}
	return tokens, nil
}
