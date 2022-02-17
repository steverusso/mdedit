package mdedit

type buffer struct {
	lines  []line
	cursor position
	vision vision
}

type line struct {
	text []byte
}

type position struct {
	row int
	col int
}

type vision struct {
	x, y int
	w, h int
}

func (b *buffer) currentLine() *line {
	return &b.lines[b.cursor.row]
}

func (b *buffer) cursorRight() {
	b.cursor.col = min(b.cursor.col+1, len(b.currentLine().text))
}

func (b *buffer) cursorToLineEnd() {
	b.cursor.col = len(b.currentLine().text)
}

func (b *buffer) cursorToLineStart() {
	b.cursor.col = b.lines[b.cursor.row].startingIndex()
}

func (b *buffer) deleteBack() {
	col := b.cursor.col
	if col == 0 {
		if b.cursor.row == 0 {
			return
		}
		// Append the current line's text to the one above it.
		prev := b.prevLine()
		prevLen := len(prev.text)
		prev.text = append(prev.text, b.currentLine().text...)
		// Cursor up a line, setting the column to it's length before being joined.
		b.cursor.row--
		b.cursor.col = prevLen
		// Remove the current line and cursor up to the previous one.
		b.lines = append(b.lines[:b.cursor.row+1], b.lines[b.cursor.row+2:]...)
	} else {
		ln := b.currentLine()
		ln.text = append(ln.text[:col-1], ln.text[col:]...)
		b.cursor.col--
	}
}

func (b *buffer) deleteForwardInsert() {
	ln := b.currentLine()
	col := b.cursor.col
	if col == len(ln.text) {
		if b.cursor.row == len(b.lines)-1 {
			return
		}
		next := &b.lines[b.cursor.row+1]
		ln.text = append(ln.text, next.text...)
		b.lines = append(b.lines[:b.cursor.row+1], b.lines[b.cursor.row+2:]...)
	} else {
		ln.text = append(ln.text[:col], ln.text[col+1:]...)
	}
}

func (b *buffer) deleteForwardNormal() {
	ln := b.currentLine()
	if len(ln.text) == 0 {
		return
	}
	col := b.cursor.col
	if col == len(ln.text)-1 {
		ln.text = ln.text[:col]
		if col > 0 {
			b.cursor.col--
		}
	} else {
		ln.text = append(ln.text[:col], ln.text[col+1:]...)
	}
}

func (b *buffer) deleteBlock(p1, p2 position) {
	if p1.row == p2.row {
		b.lines[b.cursor.row].deleteRange(p1.col, p2.col)
	} else {
		ln := &b.lines[p1.row]
		ln.text = append(ln.text[:p1.col], b.lines[p2.row].text[p2.col:]...)
		b.lines = append(b.lines[:p1.row+1], b.lines[p2.row+1:]...)
	}
}

func (b *buffer) deleteLines(y1, y2 int) {
	if y1 == y2 {
		return
	}
	b.lines = append(b.lines[:y1], b.lines[y2+1:]...)
	b.cursor.row = min(y1, len(b.lines)-1)
	b.setCursorCol(b.cursor.col)
}

func (b *buffer) insertNewLine() {
	ln := &b.lines[b.cursor.row]
	// Truncate the current line (from the cursor position on) and save the truncated text.
	trunced := newLine(ln.text[b.cursor.col:])
	ln.text = ln.text[:b.cursor.col]
	// Insert a duplicate of the current line and cursor to the beginning of that new line.
	b.lines = append(b.lines[:b.cursor.row+1], b.lines[b.cursor.row:]...)
	b.cursor.col = 0
	b.cursor.row++
	// Set the new line's text to the previous line's truncated text.
	b.lines[b.cursor.row] = trunced
}

func (b *buffer) insert(txt string) {
	ln := b.currentLine()
	col := b.cursor.col
	ln.text = append(ln.text[:col], append([]byte(txt), ln.text[col:]...)...)
	b.cursor.col += len(txt)
}

func (b *buffer) mvCursorIntoView() {
	b.cursor.row = min(max(b.cursor.row, b.vision.y), min(b.vision.y+b.vision.h, len(b.lines)-1))
	if lnLen := len(b.lines[b.cursor.row].text); b.cursor.col >= lnLen {
		b.cursor.col = max(lnLen-1, 0)
	}
}

func (b *buffer) mvViewIntoCursor() {
	y := b.cursor.row
	bot := b.vision.y + b.vision.h - 1
	switch {
	case y < b.vision.y:
		b.vision.y = y
	case y > bot:
		b.vision.y += y - bot
	}
}

func (b *buffer) prevLine() *line {
	if b.cursor.row == 0 {
		return nil
	}
	return &b.lines[b.cursor.row-1]
}

func (b *buffer) scrollVision(n int) {
	if b.vision.y < len(b.lines)-1 {
		b.vision.y += n
		b.mvCursorIntoView()
	}
}

func (b *buffer) set(data []byte) {
	var lines []line
	eofIndex := len(data) - 1
	leftOff := 0
	for i := 0; i < len(data); i++ {
		switch {
		case i == eofIndex:
			if data[i] != '\n' {
				i++
			}
			fallthrough
		case data[i] == '\n':
			lines = append(lines, newLine(data[leftOff:i]))
			leftOff = i + 1
		}
	}
	if len(lines) == 0 {
		lines = []line{{}}
	}
	b.lines = lines
}

func (b *buffer) setCursorCol(v int) {
	b.cursor.col = min(v, max(0, len(b.lines[b.cursor.row].text)-1))
}

func (b *buffer) startNewLine(below bool) {
	b.lines = append(b.lines[:b.cursor.row+1], b.lines[b.cursor.row:]...)
	b.cursor.col = 0
	if below {
		b.cursor.row++
	}
	b.lines[b.cursor.row] = line{}
}

func (b *buffer) text() (txt []byte) {
	for i := range b.lines {
		txt = append(txt, b.lines[i].text...)
		if i != len(b.lines)-1 {
			txt = append(txt, '\n')
		}
	}
	return
}

func (b *buffer) truncCurrentLineFromCursor() {
	ln := &b.lines[b.cursor.row]
	ln.text = ln.text[:b.cursor.col]
}

func (b *buffer) truncCurrentLineFromStart() {
	ln := &b.lines[b.cursor.row]
	start := ln.startingIndex()
	b.cursor.col = start
	ln.text = ln.text[:start]
}

func newLine(b []byte) (ln line) {
	ln.text = append(make([]byte, 0, len(b)), b...)
	return
}

func (ln *line) charAt(i int) byte {
	if i < 0 || i >= len(ln.text) {
		return 0
	}
	return ln.text[i]
}

func (ln *line) charIs(i int, cmps ...byte) bool {
	c := ln.charAt(i)
	for _, v := range cmps {
		if c == v {
			return true
		}
	}
	return false
}

func (ln *line) deleteRange(i, j int) {
	if len(ln.text) == 0 {
		return
	}
	ln.text = append(ln.text[:i], ln.text[j:]...)
}

func (ln *line) startingIndex() (start int) {
	for start = 0; start < len(ln.text); start++ {
		if ln.text[start] != ' ' && ln.text[start] != '\t' {
			return start
		}
	}
	return start
}

func (ln *line) toggleCheckItem() {
	col := ln.startingIndex() + 2
	if ln.charIs(col, '[') && ln.charIs(col+2, ']') {
		switch ln.text[col+1] {
		case 'x':
			ln.text[col+1] = ' '
		case ' ':
			ln.text[col+1] = 'x'
		}
	}
}

type eolPolicy byte

const (
	eolExclusive eolPolicy = iota
	eolInclusive
)

type iter struct {
	buf    *buffer
	eolpol eolPolicy
	row    int
	col    int
}

func newIter(b *buffer) iter {
	return iter{
		buf: b,
		row: b.cursor.row,
		col: b.cursor.col,
	}
}

func (it *iter) step(direction iterDirection) bool {
	if direction == iterForward {
		return it.next()
	}
	return it.prev()
}

func (it *iter) next() bool {
	it.col++
	ceilX := len(it.buf.lines[it.row].text) - 1
	if it.eolpol == eolInclusive {
		ceilX++
	}
	if it.col > ceilX {
		lineCount := len(it.buf.lines)
		if it.row >= lineCount-1 {
			it.col--
			return false
		}
		it.col = 0
		it.row++
	}
	return true
}

func (it *iter) prev() bool {
	it.col--
	if it.col < 0 {
		if it.row == 0 {
			it.col++
			return false
		}
		it.row--
		lnLen := len(it.buf.lines[it.row].text)
		it.col = max(0, lnLen-1)
	}
	return true
}

func (it *iter) seekNthLineFromTop(count int) {
	it.row = min(it.buf.vision.y+count, len(it.buf.lines)-1)
	it.ensureX()
}

func (it *iter) seekNthLineFromBot(count int) {
	bot := min(it.buf.vision.y+it.buf.vision.h-1, len(it.buf.lines)-1)
	it.row = bot - count
	it.ensureX()
}

func (it *iter) seekByX(inc int) {
	target := it.col + inc
	ceil := max(0, len(it.buf.lines[it.row].text)-1)
	if it.eolpol == eolInclusive {
		ceil++
	}
	it.col = max(min(target, ceil), 0)
}

func (it *iter) seekByY(inc int) {
	target := it.row + inc
	ceil := len(it.buf.lines) - 1
	it.row = max(min(target, ceil), 0)
	it.ensureX()
}

func (it *iter) seekByWordStart(count int, direction iterDirection) {
	var counter int
	for it.step(direction) {
		ln := it.buf.lines[it.row].text
		if len(ln) != 0 && it.eolpol == eolInclusive && it.col == len(ln) && counter == count-1 {
			return
		}
		if len(ln) == 0 || (it.eolpol == eolInclusive && it.col == len(ln)) || (!isSpace(ln[it.col]) && (it.col == 0 || isSpace(ln[it.col-1]))) {
			counter++
		}
		if counter == count {
			return
		}
	}
}

func (it *iter) ensureX() {
	if lnLen := len(it.buf.lines[it.row].text); it.col >= lnLen {
		it.col = max(lnLen-1, 0)
	}
}

func (it *iter) position() position {
	return position{row: it.row, col: it.col}
}

func (it *iter) bounds() (position, position) {
	if it.buf.cursor.row < it.row {
		return it.buf.cursor, it.position()
	}
	if it.buf.cursor.row > it.row || it.buf.cursor.col > it.col {
		return it.position(), it.buf.cursor
	}
	return it.buf.cursor, it.position()
}

func (it *iter) yBounds() (y1, y2 int) {
	if it.buf.cursor.row < it.row {
		return it.buf.cursor.row, it.row
	}
	return it.row, it.buf.cursor.row
}

type iterDirection byte

const (
	iterForward iterDirection = iota
	iterBackward
)

func (p position) is(row, col int) bool {
	return p.row == row && p.col == col
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func isSpace(chars ...byte) bool {
	for _, c := range chars {
		if c != ' ' && c != '\t' && c != '\n' {
			return false
		}
	}
	return true
}
