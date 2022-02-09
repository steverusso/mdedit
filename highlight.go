package mdedit

import "bytes"

type highlighter interface {
	highlight(*buffer) styling
}

type styleMark struct {
	row   int
	col   int
	value uint16
}

type styling struct {
	markers []styleMark
	index   int
}

func (s *styling) add(v uint16, row, col int) {
	s.markers = append(s.markers, styleMark{
		value: v,
		row:   row,
		col:   col,
	})
}

func (s *styling) current() *styleMark {
	if s.index >= len(s.markers) {
		return nil
	}
	return &s.markers[s.index]
}

func (s *styling) findStart(row, col int) *styleMark {
	var i int
	for i = 0; i < len(s.markers)-1; i++ {
		if s.markers[i+1].row >= row {
			break
		}
	}
	s.index = i
	if s.index >= 0 && s.index < len(s.markers) {
		return &s.markers[s.index]
	}
	return nil
}

func (s *styling) peek() *styleMark {
	if s.index+1 >= len(s.markers) {
		return nil
	}
	return &s.markers[s.index+1]
}

type mdHighlighter struct{}

func (h *mdHighlighter) highlight(buf *buffer) (styles styling) {
	const (
		bqStarted uint8 = iota + 1
		bqHitChar
		codeSpan1 uint8 = iota + 1
		codeSpan2
	)
	var (
		marks        uint16
		maybeHeading bool
		bqState      uint8
		inCodeSpan   uint8
		inEmphasis1  byte
		inEmphasis2  byte
	)
	styles.markers = make([]styleMark, len(buf.lines)*2)

lineloop:
	for row := 0; row < len(buf.lines); row++ {
		line := buf.lines[row].text
		if len(line) == 0 {
			if marks&mdBlockquote == mdBlockquote && bqState == bqHitChar {
				marks = marks &^ mdBlockquote
				bqState = 0
			}
		}
		marks = marks &^ mdHeading
		styles.add(marks, row, 0)

		var start int
		for start = 0; start < len(line); start++ {
			if line[start] != ' ' && line[start] != '\t' {
				break
			}
		}

		for col := start; col < len(line); col++ {
			char := line[col]

			if col == start {
				switch char {
				case '#':
					maybeHeading = true
				case '>':
					marks |= mdBlockquote
					styles.add(marks, row, col)
					bqState = bqStarted
				case '*', '+', '-':
					if col+1 < len(line)-1 && line[col+1] == ' ' {
						styles.add(marks|mdListMarker, row, col)
						styles.add(marks, row, col+1)
						col++
					}
				case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
					if col+2 < len(line)-1 && line[col+1] == '.' && line[col+2] == ' ' {
						styles.add(marks|mdListMarker, row, col)
						styles.add(marks, row, col+2)
						col += 2
					}
				}
			}
			if bqState == bqStarted && char != ' ' && char != '\t' && char != '>' {
				bqState = bqHitChar
			}
			if maybeHeading && char != '#' {
				if char == ' ' {
					marks |= mdHeading
					styles.add(marks, row, start)
				}
				maybeHeading = false
			}
			switch char {
			case '*', '_':
				var prev, next byte
				if col > 0 {
					prev = line[col-1]
				}
				if col+1 < len(line) {
					next = line[col+1]
				}

				isPrevBlank := (prev == 0 || prev == ' ' || prev == '\t')
				isNextBlank := (next == 0 || next == ' ' || next == '\t')

				if char == next {
					switch {
					case inEmphasis2 == 0 && isPrevBlank:
						var next2 byte
						if col+2 < len(line) {
							next2 = line[col+2]
						}
						if next2 != ' ' && next2 != '\t' {
							inEmphasis2 = char
							marks |= mdStrong
							styles.add(marks, row, col)
							col++
						}
					case inEmphasis2 == char && !isPrevBlank:
						inEmphasis2 = 0
						marks &^= mdStrong
						styles.add(marks, row, col+2)
						col++
					}
				} else {
					switch {
					case inEmphasis1 == 0 && isPrevBlank && !isNextBlank:
						inEmphasis1 = char
						marks |= mdItalic
						styles.add(marks, row, col)
					case inEmphasis1 != 0 && !isPrevBlank && (isNextBlank || isPunct(next) || next == inEmphasis2):
						inEmphasis1 = 0
						marks &^= mdItalic
						if col < len(line)-1 {
							styles.add(marks, row, col+1)
						}
					}
				}
			case '`':
				var next byte
				if col+1 < len(line) {
					next = line[col+1]
				}
				if col == start && next == '`' && col+2 < len(line) && line[col+2] == '`' {
					styles.add(marks|mdCodeBlock, row, start)
					delimCodeBlock := []byte("```")
					for {
						row++
						if row >= len(buf.lines) {
							return
						}
						ln := &buf.lines[row]
						if bytes.Equal(ln.text[ln.startingIndex():], delimCodeBlock) {
							break
						}
					}
					styles.add(marks, row+1, 0)
					continue lineloop
				}
				switch inCodeSpan {
				case 0:
					inCodeSpan = codeSpan1
					marks |= mdCodeSpan
					styles.add(marks, row, col)
					if next == '`' {
						inCodeSpan = codeSpan2
						col += 2
					}
				case codeSpan1:
					marks &^= mdCodeSpan
					inCodeSpan = 0
					if col < len(line)-1 {
						styles.add(marks, row, col+1)
					}
				case codeSpan2:
					if next == '`' {
						marks &^= mdCodeSpan
						inCodeSpan = 0
						col++
						if col+1 < len(line)-1 {
							styles.add(marks, row, col+2)
						}
					}
				}
			}
		}
		if maybeHeading {
			styles.add(marks|mdHeading, row, start)
			maybeHeading = false
		}
	}

	return
}

func isPunct(char byte) bool {
	switch char {
	case '.', ',', '!', '?':
		return true
	default:
		return false
	}
}
