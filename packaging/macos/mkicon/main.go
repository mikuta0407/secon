// mkicon は secon のアプリアイコン (角丸の四角に盾と鍵穴) を描き、.iconset 用の PNG を出力する。
//
//	go run ./packaging/macos/mkicon out.iconset && iconutil -c icns out.iconset -o secon.icns
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

const ss = 4 // スーパーサンプリング倍率 (アンチエイリアス)

// shade は 1024 基準の座標 (x, y) の色 (アルファ付き) を返す。
func shade(x, y float64) (color.NRGBA, bool) {
	// 背景: macOS のアイコングリッド (824 角, 角丸 185) に縦グラデーション
	const inset, size, r = 100.0, 824.0, 185.0
	if !roundRect(x, y, inset, inset, size, size, r) {
		return color.NRGBA{}, false
	}
	t := (y - inset) / size
	bg := lerp(color.NRGBA{0x2b, 0x8c, 0xf0, 0xff}, color.NRGBA{0x10, 0x4a, 0xb0, 0xff}, t)

	// 盾 (白)
	const cx, top, mid, tip, half = 512.0, 255.0, 540.0, 805.0, 235.0
	inShield := false
	if y >= top && y <= tip {
		w := half
		if y > mid {
			k := (y - mid) / (tip - mid)
			w = half * math.Sqrt(1-k*k)
		}
		// 上辺の角を少し丸める
		if y < top+40 {
			w -= 40 - math.Sqrt(math.Max(0, 40*40-(top+40-y)*(top+40-y)))
		}
		inShield = math.Abs(x-cx) <= w
	}
	if !inShield {
		return bg, true
	}
	// 鍵穴 (背景色でくり抜く)
	dx, dy := x-cx, y-470
	if dx*dx+dy*dy <= 62*62 {
		return bg, true
	}
	if y >= 480 && y <= 650 {
		w := 26 + (y-480)/(650-480)*20
		if math.Abs(x-cx) <= w {
			return bg, true
		}
	}
	return color.NRGBA{0xff, 0xff, 0xff, 0xff}, true
}

func roundRect(x, y, x0, y0, w, h, r float64) bool {
	if x < x0 || y < y0 || x > x0+w || y > y0+h {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x0+w-r))
	cy := math.Max(y0+r, math.Min(y, y0+h-r))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(p, q uint8) uint8 { return uint8(float64(p) + (float64(q)-float64(p))*t) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 0xff}
}

func render(px int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	scale := 1024.0 / float64(px)
	for py := 0; py < px; py++ {
		for qx := 0; qx < px; qx++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(qx) + (float64(sx)+0.5)/ss) * scale
					y := (float64(py) + (float64(sy)+0.5)/ss) * scale
					if c, ok := shade(x, y); ok {
						r += float64(c.R)
						g += float64(c.G)
						b += float64(c.B)
						a++
					}
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(qx, py, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(255 * a / (ss * ss))})
		}
	}
	return img
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: mkicon <out.iconset>")
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, s := range []int{16, 32, 128, 256, 512} {
		for _, k := range []int{1, 2} {
			name := fmt.Sprintf("icon_%dx%d.png", s, s)
			if k == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", s, s)
			}
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				log.Fatal(err)
			}
			if err := png.Encode(f, render(s*k)); err != nil {
				log.Fatal(err)
			}
			f.Close()
		}
	}
}
