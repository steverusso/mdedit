package mdedit

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

const (
	// blocks
	mdHeading uint16 = 1 << iota
	mdBlockquote
	mdCodeBlock
	mdThematicBreak
	// inlines
	mdItalic
	mdStrong
	mdCodeSpan
	mdListMarker
	mdLinkURL
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	iconDirectory  = iconMust(icons.FileFolderOpen)
	iconEdit       = iconMust(icons.EditorModeEdit)
	iconHome       = iconMust(icons.ActionHome)
	iconReader     = iconMust(icons.ActionChromeReaderMode)
	iconRegFile    = iconMust(icons.ActionDescription)
	iconVisibility = iconMust(icons.ActionVisibility)
	iconUnknown    = iconMust(icons.ContentBlock)
	iconWebAsset   = iconMust(icons.AVWebAsset)
)

// iconMust returns a new `*widget.Icon` for the given byte slice. It panics on error.
func iconMust(iconBytes []byte) *widget.Icon {
	ic, err := widget.NewIcon(iconBytes)
	if err != nil {
		panic(err)
	}
	return ic
}

func darken(c color.NRGBA, f float32) color.NRGBA {
	return color.NRGBA{
		R: uint8(float32(c.R) * (1 - f)),
		G: uint8(float32(c.G) * (1 - f)),
		B: uint8(float32(c.B) * (1 - f)),
		A: 255,
	}
}

func lighten(c color.NRGBA, f float32) color.NRGBA {
	return color.NRGBA{
		R: c.R + uint8(float32(255-c.R)*f),
		G: c.G + uint8(float32(255-c.G)*f),
		B: c.B + uint8(float32(255-c.B)*f),
		A: 255,
	}
}

func merge(c1, c2 color.NRGBA, p float32) color.NRGBA {
	return color.NRGBA{
		R: mergeCalc(c1.R, c2.R, p),
		G: mergeCalc(c1.G, c2.G, p),
		B: mergeCalc(c1.B, c2.B, p),
		A: 255,
	}
}

func mergeCalc(a, b uint8, p float32) uint8 {
	v1 := float32(a) * (1 - p)
	v2 := float32(b) * p
	return uint8(v1 + v2)
}

type separator struct {
	width    int
	color    color.NRGBA
	vertical bool
}

func (sep separator) Layout(gtx C) D {
	if sep.width == 0 {
		sep.width = 1
	}
	size := image.Point{gtx.Constraints.Max.X, sep.width}
	if sep.vertical {
		size = image.Point{sep.width, gtx.Constraints.Max.Y}
	}
	rect := clip.Rect{Max: size}.Op()
	paint.FillShape(gtx.Ops, sep.color, rect)
	return D{Size: size}
}

type buttonGroup struct {
	bg       color.NRGBA
	fg       color.NRGBA
	shaper   text.Shaper
	textSize unit.Value
}

type groupButton struct {
	click    *widget.Clickable
	icon     *widget.Icon
	text     string
	disabled bool
}

func (g buttonGroup) layout(gtx C, buttons []groupButton) D {
	if len(buttons) == 0 {
		return D{}
	}
	// Determine the height of the tallest element.
	var maxHeight int
	{
		for _, b := range buttons {
			m := op.Record(gtx.Ops)
			dims := g.drawButton(gtx, b)
			_ = m.Stop()
			if h := dims.Size.Y; h > maxHeight {
				maxHeight = h
			}
		}
	}
	// Make the flex wrapped buttons with dividers in between.
	border := merge(g.fg, g.bg, 0.3)
	divider := func(gtx C) D {
		gtx.Constraints.Max.Y = maxHeight
		return separator{color: border, vertical: true}.Layout(gtx)
	}
	flexBtns := make([]layout.FlexChild, 0, len(buttons)*2)
	for i := 0; i < len(buttons); i++ {
		b := buttons[i]
		if i != 0 {
			flexBtns = append(flexBtns, layout.Rigid(divider))
		}
		flexBtns = append(flexBtns, layout.Rigid(func(gtx C) D {
			return g.drawButtonWithBg(gtx, b, maxHeight)
		}))
	}
	// Determine this button group's full size for the rounded rectangle clip.
	const radius = 5
	m := op.Record(gtx.Ops)
	fullDims := widget.Border{
		Color:        border,
		CornerRadius: unit.Dp(radius),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{}.Layout(gtx, flexBtns...)
	})
	fullDraw := m.Stop()
	// Clip the rounded rectangle area and draw the button group.
	defer clip.RRect{
		Rect: f32.Rectangle{Max: f32.Point{
			X: float32(fullDims.Size.X),
			Y: float32(fullDims.Size.Y),
		}},
		SE: radius, SW: radius, NW: radius, NE: radius,
	}.Push(gtx.Ops).Pop()
	fullDraw.Add(gtx.Ops)
	return fullDims
}

func (g *buttonGroup) drawButtonWithBg(gtx C, b groupButton, height int) D {
	// Pre-draw in order to get the width for filling the background.
	m := op.Record(gtx.Ops)
	dims := g.drawButton(gtx, b)
	call := m.Stop()
	// Adjust background color under certain conditions.
	bg := g.bg
	switch {
	case b.disabled:
		bg = darken(bg, 0.05)
	case b.click.Pressed():
		bg = lighten(bg, 0.1)
	case b.click.Hovered():
		bg = lighten(bg, 0.4)
	case !b.disabled:
		bg = lighten(bg, 0.2)
	}
	size := image.Point{X: dims.Size.X, Y: height}
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
	// Vertically center the button content.
	defer op.Offset(f32.Point{Y: float32(height/2) - float32(dims.Size.Y/2)}).Push(gtx.Ops).Pop()
	return b.click.Layout(gtx, func(gtx C) D {
		call.Add(gtx.Ops)
		return D{Size: size}
	})
}

func (g *buttonGroup) drawButton(gtx C, b groupButton) D {
	fg := g.fg
	if b.disabled {
		fg.A /= 2
	}
	var content layout.Widget
	switch {
	case b.icon != nil:
		content = func(gtx C) D {
			return b.icon.Layout(gtx, fg)
		}
	case b.text != "":
		content = func(gtx C) D {
			paint.ColorOp{Color: fg}.Add(gtx.Ops)
			return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				return widget.Label{MaxLines: 1}.Layout(gtx, g.shaper, text.Font{}, g.textSize, b.text)
			})
		}
	default:
		return D{}
	}
	return layout.UniformInset(unit.Dp(6)).Layout(gtx, content)
}
