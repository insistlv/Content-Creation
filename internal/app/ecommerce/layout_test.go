package ecommerce

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestLayoutEmbeddedFontAndGlyphs(t *testing.T) {
	if EmbeddedLayoutFontSHA256() == "" || EmbeddedLayoutFontSHA256() != LayoutFontSHA256 {
		t.Fatalf("font sha = %q, constant = %q", EmbeddedLayoutFontSHA256(), LayoutFontSHA256)
	}
	for _, text := range []string{"中文", "0123456789", "，。！？：；"} {
		if !LayoutFontSupports(text) {
			t.Fatalf("font does not support %q", text)
		}
	}
}

func TestLayoutDocumentValidation(t *testing.T) {
	doc := LayoutDocument{Version: 1, Canvas: LayoutCanvas{Width: 1024, Height: 1280, SafeArea: LayoutRect{X: 40, Y: 40, Width: 944, Height: 1200}}, Background: LayoutBackground{Color: "#ffffff"}, Template: "clean", TextBlocks: []LayoutTextBlock{{Text: "材质：不锈钢", X: 80, Y: 900, Width: 864, Height: 160, FontSize: 48, MaxLines: 2, Color: "#111111"}}}
	if err := ValidateLayoutDocument(doc); err != nil {
		t.Fatalf("valid doc: %v", err)
	}
	bad := doc
	bad.TextBlocks = append([]LayoutTextBlock(nil), doc.TextBlocks...)
	bad.Canvas.Width = 400
	if err := ValidateLayoutDocument(bad); !errors.Is(err, ErrLayoutInvalid) {
		t.Fatalf("small canvas error = %v", err)
	}
	bad = doc
	bad.TextBlocks = append([]LayoutTextBlock(nil), doc.TextBlocks...)
	bad.TextBlocks[0].X = 1000
	if err := ValidateLayoutDocument(bad); !errors.Is(err, ErrLayoutInvalid) {
		t.Fatalf("unsafe block error = %v", err)
	}
	bad = doc
	bad.TextBlocks = append([]LayoutTextBlock(nil), doc.TextBlocks...)
	bad.Template = "unknown"
	if err := ValidateLayoutDocument(bad); !errors.Is(err, ErrLayoutInvalid) {
		t.Fatalf("template error = %v", err)
	}
}

func TestLayoutRendererDeterministicTemplatesAndTrueFourByFive(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1024, 1536))
	for y := 0; y < 1536; y++ {
		for x := 0; x < 1024; x++ {
			source.Set(x, y, color.RGBA{R: byte(x), G: byte(y), B: 90, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	for _, template := range []string{"clean", "dark_gradient", "brand_band"} {
		doc := DefaultProductDetailLayout("hero", template, "4:5", []string{"全天保温", "材质：不锈钢"})
		first, metadata, err := RenderLayout(encoded.Bytes(), "image/png", doc)
		if err != nil {
			t.Fatalf("%s render: %v", template, err)
		}
		second, _, err := RenderLayout(encoded.Bytes(), "image/png", doc)
		if err != nil || !bytes.Equal(first, second) {
			t.Fatalf("%s output is not deterministic", template)
		}
		decoded, _, err := image.Decode(bytes.NewReader(first))
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Bounds().Dx() != 1024 || decoded.Bounds().Dy() != 1280 || metadata.SourceSize != "1024x1536" || metadata.OutputSize != "1024x1280" || metadata.CropMode == "" {
			t.Fatalf("metadata/bounds = %#v %v", metadata, decoded.Bounds())
		}
		sum := sha256.Sum256(first)
		if hex.EncodeToString(sum[:]) == "" {
			t.Fatal("missing output hash")
		}
	}
}

func TestLayoutRendererRejectsOverflowWithoutTruncation(t *testing.T) {
	imageBytes := solidPNG(t, 1024, 1024)
	doc := DefaultProductDetailLayout("specification", "clean", "1:1", []string{"这是一个绝对无法放进单行狭窄文本框且不允许静默截断的超长规格文本"})
	doc.TextBlocks[0].Width, doc.TextBlocks[0].Height, doc.TextBlocks[0].MaxLines = 120, 30, 1
	if _, _, err := RenderLayout(imageBytes, "image/png", doc); !errors.Is(err, ErrLayoutTextOverflow) {
		t.Fatalf("error = %v, want overflow", err)
	}
}

func TestLayoutDefaultDoesNotSilentlyDropConfirmedText(t *testing.T) {
	texts := []string{"标题", "卖点", "规格：500ml"}
	doc := DefaultProductDetailLayout("specification", "clean", "1:1", texts)
	if len(doc.TextBlocks) != len(texts) {
		t.Fatalf("text blocks = %d, want %d", len(doc.TextBlocks), len(texts))
	}
	for index := range texts {
		if doc.TextBlocks[index].Text != texts[index] {
			t.Fatalf("block %d = %q", index, doc.TextBlocks[index].Text)
		}
	}
}

func solidPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
