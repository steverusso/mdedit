package mdedit

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"
)

type mode byte

const (
	modeNormal mode = iota
	modeInsert
	modeInsertNormal
	modeVisual
	modeVisualLine
	modeVisualBlock
	modeCommand
)

type Editor struct {
	buf     buffer
	mode    mode
	pending command
	active  action
	history []action

	eventKey byte
	click    gesture.Click
	reqFocus bool
	reqSave  bool
	changed  bool

	maxSize     image.Point
	shaper      text.Shaper
	font        text.Font
	textSize    unit.Sp
	palette     Palette
	charWidth   int
	lnHeight    int
	lnNumSpace  int
	highlighter highlighter
	styleMarks  [][]mdStyleMark
}

type highlighter interface {
	highlight(*buffer) [][]mdStyleMark
}

func (ed *Editor) Layout(gtx C, sh text.Shaper, fnt text.Font, txtSize unit.Sp, pal Palette) D {
	ed.ensure(gtx, sh, fnt, txtSize, pal)

	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
	ed.click.Add(gtx.Ops)
	for _, e := range ed.click.Events(gtx) {
		if e.Type == gesture.TypePress {
			ed.reqFocus = true
		}
	}
	if ed.reqFocus {
		key.FocusOp{Tag: &ed.eventKey}.Add(gtx.Ops)
		ed.reqFocus = false
	}

	ed.processEvents(gtx)
	return layout.Inset{Left: 5}.Layout(gtx, func(gtx C) D {
		return ed.layLines(gtx)
	})
}

func (ed *Editor) processEvents(gtx C) {
	const keySet = "A|B|C|D|E|F|G|H|I|J|K|L|M|N|O|P|Q|R|S|T|U|V|W|U|X|Y|Z" +
		"|" + "Ctrl-[E,R,S]" +
		"|" + key.NameDeleteBackward +
		"|" + key.NameDeleteForward +
		"|" + key.NameEscape +
		"|" + key.NameReturn

	key.InputOp{Tag: &ed.eventKey, Keys: keySet}.Add(gtx.Ops)
	switch ed.mode {
	case modeNormal:
		ed.processNormalEvents(gtx)
	case modeInsert:
		ed.processInsertEvents(gtx)
	}
}

func (ed *Editor) processNormalEvents(gtx C) {
	for _, e := range gtx.Events(&ed.eventKey) {
		switch e := e.(type) {
		case key.Event:
			if e.State != key.Press {
				continue
			}
			switch e.Modifiers {
			case key.ModCtrl:
				switch e.Name {
				case "E":
					ed.buf.scrollVision(1)
				case "R":
					// TODO redo?
				case "S":
					ed.reqSave = true
				}
			case 0:
				switch e.Name {
				case key.NameDeleteForward:
					if ed.pending.motionCount != 0 || ed.pending.motionChar1 != 0 {
						ed.pending = command{}
					} else {
						ed.exec(&command{cmdChar: 'x'})
					}
				case key.NameEscape:
					ed.pending = command{}
				}
			}
		case key.EditEvent:
			ed.pending.process(e.Text[0])
			if ed.pending.cmdChar != 0 || ed.pending.hasMotion() {
				ed.exec(&ed.pending)
				ed.active.cmd = ed.pending
				ed.pending = command{}
				ed.changed = true
			}
		}
		ed.buf.mvViewIntoCursor()
	}
}

func (ed *Editor) processInsertEvents(gtx C) {
	for _, e := range gtx.Events(&ed.eventKey) {
		switch e := e.(type) {
		case key.Event:
			if e.State != key.Press {
				continue
			}
			switch e.Name {
			case key.NameDeleteBackward:
				ed.buf.deleteBack()
				ed.highlight()
			case key.NameDeleteForward:
				ed.buf.deleteForwardInsert()
				ed.highlight()
			case key.NameReturn:
				ed.buf.insertNewLine()
				ed.highlight()
			case key.NameEscape:
				ed.exitInsertMode()
			}
		case key.EditEvent:
			ed.buf.insert(e.Text)
		}
		ed.changed = true
	}
}

func (ed *Editor) exitInsertMode() {
	ed.buf.cursor.col = max(0, ed.buf.cursor.col-1)
	ed.mode = modeNormal
	ed.history = append(ed.history, ed.active)
	ed.active = action{}
}

func (ed *Editor) exec(c *command) {
	if c.modChar == 'g' {
		ed.gExec(c)
		return
	}
	switch c.opChar {
	case 0:
		switch c.cmdChar {
		case 0:
			ed.movement(c)
		case 'x':
			ed.buf.deleteForwardNormal()
		case 'i':
			ed.mode = modeInsert
		case 'I':
			ed.buf.cursorToLineStart()
			ed.mode = modeInsert
		case 'a':
			ed.buf.cursorRight()
			ed.mode = modeInsert
		case 'A':
			ed.buf.cursorToLineEnd()
			ed.mode = modeInsert
		case 'S':
			ed.buf.truncCurrentLineFromStart()
			ed.mode = modeInsert
		case 'O', 'o':
			ed.buf.startNewLine(c.cmdChar == 'o')
			ed.mode = modeInsert
		case 'C':
			ed.buf.truncCurrentLineFromCursor()
			ed.mode = modeInsert
		}
	case 'd':
		ed.del(c)
	case 'y':
		// TODO yank whatever motion covers
	case 'P':
		// TODO paste before cursor [count] times
	case 'p':
		// TODO paste after cursor [count] times
	}
}

func (ed *Editor) movement(c *command) {
	it := newIter(&ed.buf)
	n := c.motionCount
	switch c.motionChar1 {
	case '0':
		it.col = 0
	case '$':
		it.col = max(0, len(ed.buf.lines[it.row].text)-1)
	case 'h':
		it.seekByX(min(-1, 0-n))
	case 'l', ' ':
		it.seekByX(max(1, n))
	case 'w':
		it.seekByWordStart(max(1, n), iterForward)
	case 'b':
		it.seekByWordStart(max(1, n), iterBackward)
	case 'j':
		it.seekByY(max(1, n))
	case 'k':
		it.seekByY(min(-1, 0-n))
	case 'H':
		it.seekNthLineFromTop(max(n-1, 0))
	case 'L':
		it.seekNthLineFromBot(max(n-1, 0))
	}
	ed.buf.cursor = it.position()
}

func (ed *Editor) gExec(c *command) {
	switch c.cmdChar {
	case ' ':
		ed.buf.lines[ed.buf.cursor.row].toggleCheckItem()
	}
}

func (ed *Editor) del(c *command) {
	it := newIter(&ed.buf)
	n := c.motionCount
	switch c.motionChar1 {
	case '0':
		ln := &ed.buf.lines[ed.buf.cursor.row]
		ln.text = ln.text[ed.buf.cursor.col:]
		ed.buf.cursor.col = 0
	case 'l', ' ':
		it.eolpol = eolInclusive
		it.seekByX(max(1, n))
		ed.buf.lines[it.row].deleteRange(it.buf.cursor.col, it.col)
	case 'w':
		it.eolpol = eolInclusive
		it.seekByWordStart(max(1, n), iterForward)
		ed.buf.deleteBlock(it.bounds())
	case 'j':
		it.seekByY(max(1, n))
		ed.buf.deleteLines(it.yBounds())
	case 'k':
		it.seekByY(0 - max(1, n))
		ed.buf.deleteLines(it.yBounds())
	default:
		ed.pending = command{}
	}
	ed.highlight()
}

func (ed *Editor) layLines(gtx C) D {
	numBufLines := len(ed.buf.lines)
	botIndex := ed.buf.vision.y + ed.buf.vision.h
	textSize := fixed.I(gtx.Sp(ed.textSize))
	yOffset := 0
	// Draw each visible line of text.
	for row := ed.buf.vision.y; row < min(numBufLines, botIndex); row++ {
		gtx.Constraints.Min = image.Point{}
		vertOffset := op.Offset(image.Point{Y: yOffset}).Push(gtx.Ops)
		ed.drawLineNumber(gtx, textSize, row)

		xOffset := ed.lnNumSpace + ed.charWidth // Start the line's text after the line number.
		line := ed.buf.lines[row].text

		var marks []mdStyleMark
		if row < len(ed.styleMarks) {
			marks = ed.styleMarks[row]
		}
		nextMarkIndex := 0
		fg, fnt := ed.styleBreakdown(nil)

		segBegin := 0
		for {
			// Eat consecutive style markers that mark the same column and set the actual
			// styling based on the beginning of the segment (leave the loop with the mark
			// index set to the next marker).
			for nextMarkIndex < len(marks) && marks[nextMarkIndex].col == segBegin {
				fg, fnt = ed.styleBreakdown(&marks[nextMarkIndex])
				nextMarkIndex++
			}
			// The segment always starts out as the rest of the line. If there is a 'next'
			// marker though, the segment will end right before that marker's column.
			segEnd := len(line)
			if nextMarkIndex < len(marks) {
				segEnd = marks[nextMarkIndex].col
			}
			// If the cursor is within the current segment, then truncate the current
			// segment to right before the cursor position (since the cursor will have
			// different styling then the rest of the surrounding segment).
			if ed.buf.cursor.row == row && ed.buf.cursor.col > segBegin && ed.buf.cursor.col < segEnd {
				segEnd = ed.buf.cursor.col
			}
			// If the current segment end make no sense, these markers are tossed.
			if n := len(line); segEnd > n {
				segEnd = n
				ed.styleMarks = nil
				fg, fnt = ed.styleBreakdown(nil)
			}
			// If the beginning of the segement is at or past the end, then we're
			// certainly done with this line.
			if segBegin >= segEnd {
				break
			}

			xOffsetOp := op.Offset(image.Point{X: xOffset}).Push(gtx.Ops)
			if ed.buf.cursor.is(row, segBegin) {
				segEnd = segBegin + 1
				rect := clip.Rect{Max: image.Point{ed.charWidth, ed.lnHeight}}
				paint.FillShape(gtx.Ops, fg, rect.Op())
				paint.ColorOp{Color: ed.palette.Bg}.Add(gtx.Ops)
			} else {
				paint.ColorOp{Color: fg}.Add(gtx.Ops)
			}
			seg := string(line[segBegin:segEnd])
			segDims := drawText(gtx, ed.shaper, fnt, textSize, seg)
			xOffsetOp.Pop()

			xOffset += segDims.Size.X
			segBegin = segEnd
		}
		// Draw the cursor if it's after the last character on the line.
		if ed.buf.cursor.is(row, segBegin) {
			xOffsetOp := op.Offset(image.Point{X: xOffset}).Push(gtx.Ops)
			rect := clip.Rect{Max: image.Point{ed.charWidth, gtx.Sp(ed.textSize)}}
			paint.FillShape(gtx.Ops, ed.palette.Fg, rect.Op())
			xOffsetOp.Pop()
		}
		vertOffset.Pop()
		yOffset += ed.lnHeight
	}
	// The blank lines (if any).
	for row := numBufLines; row < botIndex; row++ {
		t := op.Offset(image.Point{Y: yOffset}).Push(gtx.Ops)
		clr := ed.palette.ListMarker
		clr.A = 100
		paint.ColorOp{Color: clr}.Add(gtx.Ops)
		drawText(gtx, ed.shaper, ed.font, textSize, "~")
		yOffset += ed.lnHeight
		t.Pop()
	}
	return D{Size: gtx.Constraints.Max}
}

func (ed *Editor) styleBreakdown(m *mdStyleMark) (color.NRGBA, text.Font) {
	fg := ed.palette.Fg
	fnt := ed.font
	if m == nil || m.value == 0 {
		return fg, fnt
	}

	if m.value&mdItalic == mdItalic {
		fnt.Style = text.Italic
	}
	if m.value&mdStrong == mdStrong {
		fnt.Weight = text.Bold
	}
	if m.value&mdCodeSpan == mdCodeSpan || m.value&mdCodeBlock == mdCodeBlock {
		fg = ed.palette.CodeBlock
	}
	if m.value&mdListMarker == mdListMarker {
		fg = ed.palette.ListMarker
		fnt.Weight = text.Bold
	}

	if m.value&mdHeading == mdHeading {
		fg = ed.palette.Heading
		fnt.Weight = text.Bold
	}
	if m.value&mdBlockquote == mdBlockquote {
		fg = ed.palette.BlockQuote
		fnt.Style = text.Italic
	}

	return fg, fnt
}

func (ed *Editor) drawLineNumber(gtx C, size fixed.Int26_6, row int) {
	num := row + 1
	if row < ed.buf.cursor.row {
		num = ed.buf.cursor.row - row
	}
	if row > ed.buf.cursor.row {
		num = row - ed.buf.cursor.row
	}
	numStr := strconv.Itoa(num)
	gtx.Constraints.Min.X = ed.lnNumSpace
	paint.ColorOp{Color: ed.palette.LineNumber}.Add(gtx.Ops)
	if row != ed.buf.cursor.row {
		// We want inactive line numbers to hug the text (in other words, be aligned
		// toward the right). So before drawing these line numbers, we offset by what
		// would be the remaining empty space so that the text will be off to the right.
		emptySpace := ed.lnNumSpace - len(numStr)*ed.charWidth
		opOffset := op.Offset(image.Point{X: emptySpace}).Push(gtx.Ops)
		drawText(gtx, ed.shaper, ed.font, size, numStr)
		opOffset.Pop()
	} else {
		drawText(gtx, ed.shaper, ed.font, size, numStr)
	}
}

func (ed *Editor) SetText(data []byte) {
	ed.buf.set(data)
	ed.changed = true
}

func (ed *Editor) highlight() {
	if ed.highlighter == nil {
		ed.highlighter = mdHighlighter{}
	}
	ed.styleMarks = ed.highlighter.highlight(&ed.buf)
}

func (ed *Editor) Text() []byte {
	return ed.buf.text()
}

func (ed *Editor) Focus() {
	ed.reqFocus = true
}

func (ed *Editor) SaveRequested() bool {
	v := ed.reqSave
	ed.reqSave = false
	return v
}

func (ed *Editor) HasChanged() bool {
	v := ed.changed
	ed.changed = false
	return v
}

func (ed *Editor) ensure(gtx C, sh text.Shaper, fnt text.Font, txtSize unit.Sp, pal Palette) {
	if ed.shaper != sh {
		ed.shaper = sh
	}
	if ed.font != fnt {
		ed.font = fnt
	}
	// Truncate and round up the given text size so it's never an odd number (which causes
	// ui bugs from rounding errors on groups of characters).
	txtSize = unit.Sp(math.Round(float64(txtSize)))
	if int(txtSize)%2 != 0 {
		txtSize = unit.Sp(int(txtSize + 1))
	}
	if ed.maxSize != gtx.Constraints.Max || ed.textSize != txtSize {
		ed.maxSize = gtx.Constraints.Max
		ed.textSize = txtSize
		// Determine character width and, from that, the width that will be used for the
		// line numbers.
		textSize := fixed.I(gtx.Sp(txtSize))
		ln := sh.LayoutString(fnt, textSize, ed.maxSize.X, gtx.Locale, " ")[0]
		ed.charWidth = ln.Width.Ceil()
		ed.lnHeight = ln.Ascent.Ceil() + ln.Descent.Ceil()
		ed.lnNumSpace = ed.charWidth * max(2, len(strconv.Itoa(len(ed.buf.lines))))
		ed.buf.vision.h = ed.maxSize.Y / ed.lnHeight
	}
	if ed.palette != pal {
		ed.palette = pal
	}
}

func drawText(gtx C, sh text.Shaper, fnt text.Font, size fixed.Int26_6, txt string) D {
	ln := sh.LayoutString(fnt, size, gtx.Constraints.Max.X, gtx.Locale, txt)[0]

	opOffset := op.Offset(image.Point{Y: ln.Ascent.Ceil()}).Push(gtx.Ops)
	opOutline := clip.Outline{Path: sh.Shape(fnt, size, ln.Layout)}.Op().Push(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	opOutline.Pop()
	opOffset.Pop()

	return D{Size: image.Point{
		X: ln.Width.Ceil(),
		Y: ln.Ascent.Ceil(),
	}}
}
