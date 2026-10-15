package app

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/qr"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type codeInput struct {
	Kind    string `json:"kind"`
	Content string `json:"content"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Level   string `json:"level"`
	Label   bool   `json:"label"`
}
type codeResult struct {
	Kind    string `json:"kind"`
	Content string `json:"content"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	PNG     string `json:"png"`
	SVG     string `json:"svg"`
}

func (s *Server) generateCode(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in codeInput
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	if !s.allowedRequest(r, "code:"+idText(u.ID)+":", 120) {
		return nil, &apiError{429, "生成过于频繁，请稍后重试"}
	}
	select {
	case s.codeSlots <- struct{}{}:
		defer func() { <-s.codeSlots }()
	default:
		return nil, &apiError{429, "生成任务繁忙，请稍后重试"}
	}
	return renderCode(in)
}
func renderCode(in codeInput) (codeResult, error) {
	var result codeResult
	var b barcode.Barcode
	var e error
	if in.Content == "" || !utf8.ValidString(in.Content) || len(in.Content) > 1200 {
		return result, bad("内容不能为空，且不能超过 1200 字节")
	}
	if in.Width < 128 || in.Width > 2048 {
		return result, bad("宽度为 128–2048 像素")
	}
	quiet := 10
	switch in.Kind {
	case "qr":
		quiet = 4
		levels := map[string]qr.ErrorCorrectionLevel{"L": qr.L, "M": qr.M, "Q": qr.Q, "H": qr.H}
		level, ok := levels[in.Level]
		if !ok {
			return result, bad("请选择有效的二维码容错等级")
		}
		b, e = qr.Encode(in.Content, level, qr.Auto)
		in.Height = in.Width
	case "code128", "code39":
		if len(in.Content) > 80 {
			return result, bad("条码内容不能超过 80 个字符")
		}
		for _, ch := range in.Content {
			if ch < 32 || ch > 126 {
				return result, bad("条码仅支持可打印的 ASCII 字符")
			}
		}
		if in.Kind == "code128" {
			b, e = code128.Encode(in.Content)
		} else {
			for _, ch := range in.Content {
				if !strings.ContainsRune("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-. $/+%", ch) {
					return result, bad("Code 39 仅支持大写字母、数字、空格及 - . $ / + %")
				}
			}
			b, e = code39.Encode(in.Content, false, false)
		}
	case "ean13", "ean8":
		n := 13
		if in.Kind == "ean8" {
			n = 8
		}
		if len(in.Content) != n && len(in.Content) != n-1 {
			return result, badf("请输入 %d 位数字（自动校验）或 %d 位完整编码", n-1, n)
		}
		for _, ch := range in.Content {
			if ch < '0' || ch > '9' {
				return result, bad("EAN 编码只允许数字")
			}
		}
		b, e = ean.Encode(in.Content)
	default:
		return result, bad("不支持的编码格式")
	}
	if e != nil {
		return result, bad("内容无法编码，请检查格式、长度或 EAN 校验位")
	}
	if in.Kind != "qr" && (in.Height < 80 || in.Height > 512) {
		return result, bad("条码高度为 80–512 像素")
	}
	modules := b.Bounds().Dx() + quiet*2
	scale := in.Width / modules
	if scale < 2 {
		return result, badf("当前内容至少需要 %d 像素宽度，请增加宽度或缩短内容", modules*2)
	}
	margin := (in.Width - b.Bounds().Dx()*scale) / 2
	img := image.NewGray(image.Rect(0, 0, in.Width, in.Height))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	var path strings.Builder
	rect := func(x, y, w, h int) {
		draw.Draw(img, image.Rect(x, y, x+w, y+h), image.Black, image.Point{}, draw.Src)
		fmt.Fprintf(&path, "M%d %dh%dv%dh-%dz", x, y, w, h, w)
	}
	black := func(x, y int) bool { r, _, _, _ := b.At(x, y).RGBA(); return r < 32768 }
	if in.Kind == "qr" {
		for y := 0; y < b.Bounds().Dy(); y++ {
			for x := 0; x < b.Bounds().Dx(); {
				if !black(x, y) {
					x++
					continue
				}
				start := x
				for x < b.Bounds().Dx() && black(x, y) {
					x++
				}
				rect(margin+start*scale, margin+y*scale, (x-start)*scale, scale)
			}
		}
	} else {
		barHeight := in.Height - 20
		if in.Label {
			barHeight -= 20
		}
		for x := 0; x < b.Bounds().Dx(); {
			if !black(x, 0) {
				x++
				continue
			}
			start := x
			for x < b.Bounds().Dx() && black(x, 0) {
				x++
			}
			rect(margin+start*scale, 10, (x-start)*scale, barHeight)
		}
		if in.Label {
			label := image.NewGray(img.Bounds())
			draw.Draw(label, label.Bounds(), image.White, image.Point{}, draw.Src)
			d := font.Drawer{Dst: label, Src: image.Black, Face: basicfont.Face7x13}
			labelWidth := d.MeasureString(b.Content()).Ceil()
			d.Dot = fixed.P((in.Width-labelWidth)/2, in.Height-7)
			d.DrawString(b.Content())
			for y := in.Height - 22; y < in.Height; y++ {
				for x := 0; x < in.Width; {
					if label.GrayAt(x, y).Y > 128 {
						x++
						continue
					}
					start := x
					for x < in.Width && label.GrayAt(x, y).Y <= 128 {
						x++
					}
					rect(start, y, x-start, 1)
				}
			}
		}
	}
	var pngData bytes.Buffer
	if e = png.Encode(&pngData, img); e != nil {
		return result, e
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="white"/><path fill="black" d="%s"/></svg>`, in.Width, in.Height, in.Width, in.Height, path.String())
	result = codeResult{Kind: in.Kind, Content: b.Content(), Width: in.Width, Height: in.Height, PNG: base64.StdEncoding.EncodeToString(pngData.Bytes()), SVG: base64.StdEncoding.EncodeToString([]byte(svg))}
	return result, nil
}
