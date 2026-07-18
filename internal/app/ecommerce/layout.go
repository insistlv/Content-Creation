package ecommerce

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

var (
	ErrLayoutInvalid      = errors.New("layout_invalid")
	ErrLayoutTextOverflow = errors.New("layout_text_overflow")
)

const LayoutFontSHA256 = "2c76254f6fc379fddfce0a7e84fb5385bb135d3e399294f6eeb6680d0365b74b"

//go:embed fonts/NotoSansCJKsc-Regular.otf
var layoutFontBytes []byte

type LayoutRect struct{ X, Y, Width, Height int }
type LayoutCanvas struct {
	Width, Height int
	SafeArea      LayoutRect
}
type LayoutBackground struct{ Color string }
type LayoutTextBlock struct {
	Text                                    string
	X, Y, Width, Height, FontSize, MaxLines int
	Color                                   string
}
type LayoutDocument struct {
	Version    int               `json:"version"`
	Canvas     LayoutCanvas      `json:"canvas"`
	Background LayoutBackground  `json:"background"`
	Template   string            `json:"template"`
	TextBlocks []LayoutTextBlock `json:"text_blocks"`
}
type LayoutRenderMetadata struct {
	LayoutVersion int    `json:"layout_version"`
	LayoutSHA256  string `json:"layout_sha256"`
	SourceSize    string `json:"source_size"`
	OutputSize    string `json:"output_size"`
	CropMode      string `json:"crop_mode"`
}

func EmbeddedLayoutFontSHA256() string {
	sum := sha256.Sum256(layoutFontBytes)
	return hex.EncodeToString(sum[:])
}

func LayoutFontSupports(text string) bool {
	parsed, err := sfnt.Parse(layoutFontBytes)
	if err != nil {
		return false
	}
	var buffer sfnt.Buffer
	for _, value := range text {
		index, err := parsed.GlyphIndex(&buffer, value)
		if err != nil || index == 0 {
			return false
		}
	}
	return true
}

func validLayoutTemplate(value string) bool {
	return value == "clean" || value == "dark_gradient" || value == "brand_band"
}

func ValidateLayoutDocument(doc LayoutDocument) error {
	if doc.Version != 1 || doc.Canvas.Width < 512 || doc.Canvas.Width > 4096 || doc.Canvas.Height < 512 || doc.Canvas.Height > 4096 || len(doc.TextBlocks) > 20 || !validLayoutTemplate(doc.Template) {
		return ErrLayoutInvalid
	}
	safe := doc.Canvas.SafeArea
	if safe.X < 0 || safe.Y < 0 || safe.Width <= 0 || safe.Height <= 0 || safe.X+safe.Width > doc.Canvas.Width || safe.Y+safe.Height > doc.Canvas.Height {
		return ErrLayoutInvalid
	}
	for _, block := range doc.TextBlocks {
		if block.FontSize < 12 || block.FontSize > 240 || block.Width <= 0 || block.Height <= 0 || block.MaxLines <= 0 ||
			block.X < safe.X || block.Y < safe.Y || block.X+block.Width > safe.X+safe.Width || block.Y+block.Height > safe.Y+safe.Height {
			return ErrLayoutInvalid
		}
	}
	return nil
}

func DefaultProductDetailLayout(section, template, aspectRatio string, texts []string) LayoutDocument {
	width, height := layoutSize(aspectRatio)
	margin := int(math.Round(float64(width) * 0.05))
	if margin < 32 {
		margin = 32
	}
	doc := LayoutDocument{Version: 1, Canvas: LayoutCanvas{Width: width, Height: height, SafeArea: LayoutRect{X: margin, Y: margin, Width: width - 2*margin, Height: height - 2*margin}}, Background: LayoutBackground{Color: "#ffffff"}, Template: template}
	if template == "dark_gradient" {
		doc.Background.Color = "#111827"
	}
	if len(texts) == 0 {
		return doc
	}
	startY := int(float64(height) * 0.57)
	if len(texts) > 3 {
		startY = doc.Canvas.SafeArea.Y
	}
	gap := maxInt(8, int(float64(height)*0.01))
	available := doc.Canvas.SafeArea.Y + doc.Canvas.SafeArea.Height - startY - gap*(len(texts)-1)
	blockHeight := available / len(texts)
	for index, text := range texts {
		y := startY + index*(blockHeight+gap)
		fontSize := maxInt(12, minInt(48, blockHeight/2))
		textColor := "#111111"
		if template == "dark_gradient" || template == "brand_band" {
			textColor = "#ffffff"
		}
		doc.TextBlocks = append(doc.TextBlocks, LayoutTextBlock{Text: text, X: margin + 20, Y: y, Width: width - 2*margin - 40, Height: blockHeight, FontSize: fontSize, MaxLines: 2, Color: textColor})
	}
	_ = section
	return doc
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func layoutSize(aspectRatio string) (int, int) {
	switch aspectRatio {
	case "3:4":
		return 1024, 1365
	case "4:5":
		return 1024, 1280
	case "9:16":
		return 1024, 1820
	default:
		return 1024, 1024
	}
}

func RenderLayout(source []byte, mimeType string, doc LayoutDocument) ([]byte, LayoutRenderMetadata, error) {
	if err := ValidateLayoutDocument(doc); err != nil {
		return nil, LayoutRenderMetadata{}, err
	}
	input, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, LayoutRenderMetadata{}, fmt.Errorf("decode layout source: %w", err)
	}
	output := image.NewRGBA(image.Rect(0, 0, doc.Canvas.Width, doc.Canvas.Height))
	draw.Draw(output, output.Bounds(), &image.Uniform{C: parseHexColor(doc.Background.Color, color.White)}, image.Point{}, draw.Src)
	coverImage(output, input)
	applyTemplateOverlay(output, doc.Template)
	parsed, err := opentype.Parse(layoutFontBytes)
	if err != nil {
		return nil, LayoutRenderMetadata{}, fmt.Errorf("parse embedded layout font: %w", err)
	}
	for _, block := range doc.TextBlocks {
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: float64(block.FontSize), DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			return nil, LayoutRenderMetadata{}, err
		}
		lines, overflow := wrapLayoutText(face, block.Text, block.Width, block.MaxLines)
		if overflow {
			_ = face.Close()
			return nil, LayoutRenderMetadata{}, ErrLayoutTextOverflow
		}
		lineHeight := face.Metrics().Height.Ceil()
		if lineHeight*len(lines) > block.Height {
			_ = face.Close()
			return nil, LayoutRenderMetadata{}, ErrLayoutTextOverflow
		}
		drawer := font.Drawer{Dst: output, Src: image.NewUniform(parseHexColor(block.Color, color.Black)), Face: face}
		for index, line := range lines {
			drawer.Dot = fixed.P(block.X, block.Y+face.Metrics().Ascent.Ceil()+index*lineHeight)
			drawer.DrawString(line)
		}
		_ = face.Close()
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, output); err != nil {
		return nil, LayoutRenderMetadata{}, err
	}
	docJSON, _ := EncodeJSON(doc)
	digest := sha256.Sum256([]byte(docJSON))
	metadata := LayoutRenderMetadata{LayoutVersion: doc.Version, LayoutSHA256: hex.EncodeToString(digest[:]), SourceSize: fmt.Sprintf("%dx%d", input.Bounds().Dx(), input.Bounds().Dy()), OutputSize: fmt.Sprintf("%dx%d", doc.Canvas.Width, doc.Canvas.Height), CropMode: "center_cover"}
	_ = mimeType
	return encoded.Bytes(), metadata, nil
}

func coverImage(destination *image.RGBA, source image.Image) {
	dw, dh, sw, sh := destination.Bounds().Dx(), destination.Bounds().Dy(), source.Bounds().Dx(), source.Bounds().Dy()
	scale := math.Max(float64(dw)/float64(sw), float64(dh)/float64(sh))
	tw, th := int(math.Ceil(float64(sw)*scale)), int(math.Ceil(float64(sh)*scale))
	temp := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(temp, temp.Bounds(), source, source.Bounds(), draw.Src, nil)
	sx, sy := (tw-dw)/2, (th-dh)/2
	draw.Draw(destination, destination.Bounds(), temp, image.Pt(sx, sy), draw.Src)
}

func applyTemplateOverlay(output *image.RGBA, template string) {
	w, h := output.Bounds().Dx(), output.Bounds().Dy()
	switch template {
	case "clean":
		draw.Draw(output, image.Rect(0, int(float64(h)*.64), w, h), &image.Uniform{C: color.RGBA{255, 255, 255, 225}}, image.Point{}, draw.Over)
	case "brand_band":
		draw.Draw(output, image.Rect(0, int(float64(h)*.64), w, h), &image.Uniform{C: color.RGBA{20, 63, 92, 235}}, image.Point{}, draw.Over)
	case "dark_gradient":
		start := int(float64(h) * .45)
		for y := start; y < h; y++ {
			alpha := uint8(float64(y-start) / float64(h-start) * 220)
			draw.Draw(output, image.Rect(0, y, w, y+1), &image.Uniform{C: color.RGBA{8, 12, 20, alpha}}, image.Point{}, draw.Over)
		}
	}
}

func wrapLayoutText(face font.Face, text string, width, maxLines int) ([]string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, false
	}
	lines, current := []string{}, ""
	for _, value := range []rune(text) {
		candidate := current + string(value)
		measured := font.MeasureString(face, candidate).Ceil()
		if measured <= width {
			current = candidate
			continue
		}
		if current == "" {
			return nil, true
		}
		lines = append(lines, current)
		current = string(value)
		if len(lines) >= maxLines {
			return nil, true
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines, len(lines) > maxLines
}

func parseHexColor(value string, fallback color.Color) color.Color {
	var r, g, b uint8
	if len(value) == 7 && value[0] == '#' {
		var rv, gv, bv uint64
		_, e1 := fmt.Sscanf(value[1:3], "%02x", &rv)
		_, e2 := fmt.Sscanf(value[3:5], "%02x", &gv)
		_, e3 := fmt.Sscanf(value[5:7], "%02x", &bv)
		if e1 == nil && e2 == nil && e3 == nil {
			r, g, b = uint8(rv), uint8(gv), uint8(bv)
			return color.RGBA{r, g, b, 255}
		}
	}
	return fallback
}
