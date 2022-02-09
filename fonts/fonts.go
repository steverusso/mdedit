package fonts

import (
	_ "embed"

	"gioui.org/font/opentype"
	"gioui.org/text"
)

var UbuntuFontCollection = []text.FontFace{
	{text.Font{}, fontMust(ubuntuTTF)},
	{text.Font{Style: text.Italic}, fontMust(ubuntuItalicTTF)},
	{text.Font{Weight: text.Bold}, fontMust(ubuntuBoldTTF)},
	{text.Font{Weight: text.Medium}, fontMust(ubuntuMediumTTF)},
	{text.Font{Variant: "Mono"}, fontMust(ubuntuMonoTTF)},
	{text.Font{Variant: "Mono", Style: text.Italic}, fontMust(ubuntuMonoItalicTTF)},
	{text.Font{Variant: "Mono", Weight: text.Bold}, fontMust(ubuntuMonoBoldTTF)},
	{text.Font{Variant: "Mono", Weight: text.Bold, Style: text.Italic}, fontMust(ubuntuMonoBoldItalicTTF)},
}

// fontMust parses the given font and panics if unable to do so.
func fontMust(ttf []byte) *opentype.Font {
	fnt, err := opentype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	return fnt
}

//go:embed Ubuntu-R.ttf
var ubuntuTTF []byte

//go:embed Ubuntu-RI.ttf
var ubuntuItalicTTF []byte

//go:embed Ubuntu-B.ttf
var ubuntuBoldTTF []byte

//go:embed Ubuntu-M.ttf
var ubuntuMediumTTF []byte

//go:embed UbuntuMono-R.ttf
var ubuntuMonoTTF []byte

//go:embed UbuntuMono-B.ttf
var ubuntuMonoBoldTTF []byte

//go:embed UbuntuMono-Italic.ttf
var ubuntuMonoItalicTTF []byte

//go:embed UbuntuMono-BoldItalic.ttf
var ubuntuMonoBoldItalicTTF []byte
