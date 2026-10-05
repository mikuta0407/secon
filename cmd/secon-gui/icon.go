package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"fyne.io/fyne/v2"
)

var (
	iconConnected    = makeIcon("connected", color.NRGBA{0x2e, 0xa0, 0x43, 0xff})
	iconDisconnected = makeIcon("disconnected", color.NRGBA{0x80, 0x80, 0x80, 0xff})
)

// makeIcon は鍵穴形のトレイアイコンを描く (外部画像ファイルを持たないため)。
func makeIcon(name string, c color.NRGBA) fyne.Resource {
	const size = 44
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-center, float64(y)+0.5-center
			d2 := dx*dx + dy*dy
			ring := d2 <= 20*20 && d2 >= 14*14
			hole := (dx*dx+(dy+4)*(dy+4) <= 5*5) || (dx >= -2.5 && dx <= 2.5 && dy >= -4 && dy <= 9)
			if ring || hole {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return fyne.NewStaticResource(name+".png", b.Bytes())
}
