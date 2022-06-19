package mdedit

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
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
	styles      styling
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
	key.InputOp{Tag: &ed.eventKey}.Add(gtx.Ops)
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
			switch {
			case e.State != key.Press:
				break
			case e.Name == key.NameDeleteForward:
				if ed.pending.motionCount != 0 || ed.pending.motionChar1 != 0 {
					ed.pending = command{}
				} else {
					ed.exec(&command{cmdChar: 'x'})
				}
			case e.Name == key.NameEscape:
				ed.pending = command{}
			case e.Modifiers == key.ModCtrl:
				switch e.Name {
				case "E":
					ed.buf.scrollVision(1)
				case "R":
					// todo: redo?
				case "S":
					ed.reqSave = true
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
			switch {
			case e.State != key.Press:
				break
			case e.Name == key.NameDeleteBackward:
				ed.buf.deleteBack()
				ed.highlight()
			case e.Name == key.NameDeleteForward:
				ed.buf.deleteForwardInsert()
				ed.highlight()
			case e.Name == key.NameReturn:
				ed.buf.insertNewLine()
				ed.highlight()
			case e.Name == key.NameEscape:
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
		// todo: yank whatever motion covers
	case 'P':
		// todo: paste before cursor [count] times
	case 'p':
		// todo: paste after cursor [count] times
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
	var (
		bufLineTotal = len(ed.buf.lines)
		botIndex     = ed.buf.vision.y + ed.buf.vision.h
		yOffset      = 0
		mark         = ed.styles.findStart(ed.buf.vision.y, ed.buf.vision.x)
		fg, fnt      = ed.styleBreakdown(mark)
	)

	// Draw each line of text.
	for row := ed.buf.vision.y; row < min(bufLineTotal, botIndex); row++ {
		gtx.Constraints.Min = image.Point{}
		vertOffset := op.Offset(image.Point{Y: yOffset}).Push(gtx.Ops)
		ed.drawLineNumber(gtx, row)

		xOffset := ed.lnNumSpace + ed.charWidth // Start the line's text after the line number.
		line := ed.buf.lines[row].text

		segBegin := 0
		for {
			mark = ed.styles.peek()
			for mark != nil && mark.row == row && mark.col == segBegin {
				fg, fnt = ed.styleBreakdown(mark)
				ed.styles.index++
				mark = ed.styles.peek()
			}
			if segBegin > len(line)-1 {
				break
			}

			segEnd := len(line)
			if mark != nil && mark.row == row {
				segEnd = mark.col
			}
			if ed.buf.cursor.row == row && ed.buf.cursor.col > segBegin && ed.buf.cursor.col < segEnd {
				segEnd = ed.buf.cursor.col
			}
			// If the current segment end make no sense, then this set of markers is tossed.
			if n := len(line); segEnd > n {
				segEnd = n
				ed.styles = styling{}
				fg, fnt = ed.styleBreakdown(nil)
			}

			xOffsetOp := op.Offset(image.Point{X: xOffset}).Push(gtx.Ops)
			if ed.buf.cursor.is(row, segBegin) {
				segEnd = segBegin + 1
				rect := clip.Rect{Max: image.Point{ed.charWidth, gtx.Sp(ed.textSize)}}
				paint.FillShape(gtx.Ops, fg, rect.Op())
				paint.ColorOp{Color: ed.palette.Bg}.Add(gtx.Ops)
			} else {
				paint.ColorOp{Color: fg}.Add(gtx.Ops)
			}
			seg := string(line[segBegin:segEnd])
			segDims := widget.Label{MaxLines: 1}.Layout(gtx, ed.shaper, fnt, ed.textSize, seg)
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
	for row := bufLineTotal; row < botIndex; row++ {
		t := op.Offset(image.Point{Y: yOffset}).Push(gtx.Ops)
		clr := ed.palette.ListMarker
		clr.A = 100
		paint.ColorOp{Color: clr}.Add(gtx.Ops)
		widget.Label{}.Layout(gtx, ed.shaper, ed.font, ed.textSize, "~")
		yOffset += ed.lnHeight
		t.Pop()
	}

	return D{Size: gtx.Constraints.Max}
}

func (ed *Editor) styleBreakdown(m *styleMark) (color.NRGBA, text.Font) {
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

func (ed *Editor) drawLineNumber(gtx C, row int) {
	num := row + 1
	if row < ed.buf.cursor.row {
		num = ed.buf.cursor.row - row
	}
	if row > ed.buf.cursor.row {
		num = row - ed.buf.cursor.row
	}
	lbl := widget.Label{MaxLines: 1}
	if row != ed.buf.cursor.row {
		lbl.Alignment = text.End
	}
	gtx.Constraints.Min.X = ed.lnNumSpace
	paint.ColorOp{Color: ed.palette.LineNumber}.Add(gtx.Ops)
	lbl.Layout(gtx, ed.shaper, ed.font, ed.textSize, fmt.Sprint(num))
}

func (ed *Editor) SetText(data []byte) {
	ed.buf.set(data)
	ed.changed = true
}

func (ed *Editor) highlight() {
	if ed.highlighter == nil {
		ed.highlighter = &mdHighlighter{}
	}
	ed.styles = ed.highlighter.highlight(&ed.buf)
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
	if ed.maxSize != gtx.Constraints.Max || ed.textSize != txtSize {
		const lnPadding = 1
		ed.maxSize = gtx.Constraints.Max
		ed.textSize = txtSize
		ed.lnHeight = gtx.Sp(txtSize) + lnPadding
		ed.buf.vision.h = ed.maxSize.Y / ed.lnHeight
		// Determine character width and, from that, the width that will be used for the line numbers.
		textSize := fixed.I(gtx.Sp(txtSize))
		lines := sh.LayoutString(fnt, textSize, ed.maxSize.X, gtx.Locale, " ")
		ed.charWidth = lines[0].Width.Ceil()
		ed.lnNumSpace = ed.charWidth * max(2, len(fmt.Sprint(len(ed.buf.lines))))
	}
	if ed.palette != pal {
		ed.palette = pal
	}
}
