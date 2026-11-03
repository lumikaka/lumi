package production

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"strings"
	"unicode"

	"lumi/internal/platformpath"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

const (
	MaxSectionPremiseAssets       = 12
	sectionPremiseTileWidth       = 360
	sectionPremiseTileHeight      = 300
	sectionPremiseLabelHeight     = 72
	sectionPremiseGap             = 24
	sectionPremiseMaxColumns      = 3
	SectionPremiseComposerVersion = "v1"
)

var sectionPremiseFontPaths = platformpath.CJKFontPaths()

type SectionPremiseSource struct {
	Reference PremiseAssetReference
	Image     image.Image
}

type SectionPremiseComposition struct {
	Bytes  []byte
	Width  int
	Height int
}

func ComposeSectionPremise(sources []SectionPremiseSource) (SectionPremiseComposition, error) {
	if len(sources) == 0 {
		return SectionPremiseComposition{}, nil
	}
	if len(sources) > MaxSectionPremiseAssets {
		return SectionPremiseComposition{}, fmt.Errorf("section premise has %d assets; maximum is %d", len(sources), MaxSectionPremiseAssets)
	}
	labels := make([]string, len(sources))
	for index, source := range sources {
		if source.Image == nil || source.Image.Bounds().Dx() <= 0 || source.Image.Bounds().Dy() <= 0 {
			return SectionPremiseComposition{}, fmt.Errorf("section premise source %d is not a valid image", index+1)
		}
		title := strings.TrimSpace(source.Reference.Title)
		if title == "" {
			title = "Untitled"
		}
		labels[index] = fmt.Sprintf("%d. %s", index+1, title)
	}
	face, err := LoadSectionPremiseFont(labels, sectionPremiseFontPaths)
	if err != nil {
		return SectionPremiseComposition{}, err
	}
	defer face.Close()
	return ComposeSectionPremiseWithFace(sources, labels, face)
}

func ComposeSectionPremiseWithFace(sources []SectionPremiseSource, labels []string, face font.Face) (SectionPremiseComposition, error) {
	columns := min(sectionPremiseMaxColumns, len(sources))
	rows := int(math.Ceil(float64(len(sources)) / float64(columns)))
	width := columns*sectionPremiseTileWidth + (columns+1)*sectionPremiseGap
	height := rows*(sectionPremiseTileHeight+sectionPremiseLabelHeight) + (rows+1)*sectionPremiseGap
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	for index, source := range sources {
		column := index % columns
		row := index / columns
		x := sectionPremiseGap + column*(sectionPremiseTileWidth+sectionPremiseGap)
		y := sectionPremiseGap + row*(sectionPremiseTileHeight+sectionPremiseLabelHeight+sectionPremiseGap)
		tile := image.Rect(x, y, x+sectionPremiseTileWidth, y+sectionPremiseTileHeight)
		drawSectionPremiseBorder(canvas, tile, color.RGBA{R: 218, G: 222, B: 228, A: 255})

		sourceBounds := source.Image.Bounds()
		availableWidth := sectionPremiseTileWidth - 24
		availableHeight := sectionPremiseTileHeight - 24
		scale := math.Min(float64(availableWidth)/float64(sourceBounds.Dx()), float64(availableHeight)/float64(sourceBounds.Dy()))
		scaledWidth := max(1, int(math.Round(float64(sourceBounds.Dx())*scale)))
		scaledHeight := max(1, int(math.Round(float64(sourceBounds.Dy())*scale)))
		destination := image.Rect(
			x+(sectionPremiseTileWidth-scaledWidth)/2,
			y+(sectionPremiseTileHeight-scaledHeight)/2,
			x+(sectionPremiseTileWidth-scaledWidth)/2+scaledWidth,
			y+(sectionPremiseTileHeight-scaledHeight)/2+scaledHeight,
		)
		draw.CatmullRom.Scale(canvas, destination, source.Image, sourceBounds, draw.Over, nil)

		lines := wrapSectionPremiseLabel(labels[index], face, sectionPremiseTileWidth-16, 2)
		drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(color.RGBA{R: 31, G: 35, B: 41, A: 255}), Face: face}
		baseline := y + sectionPremiseTileHeight + 27
		lineHeight := max(22, face.Metrics().Height.Ceil()+4)
		for lineIndex, line := range lines {
			drawer.Dot = fixed.P(x+8, baseline+lineIndex*lineHeight)
			drawer.DrawString(line)
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return SectionPremiseComposition{}, fmt.Errorf("encode section premise PNG: %w", err)
	}
	return SectionPremiseComposition{Bytes: encoded.Bytes(), Width: width, Height: height}, nil
}

func drawSectionPremiseBorder(destination *image.RGBA, rectangle image.Rectangle, border color.Color) {
	draw.Draw(destination, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Max.X, rectangle.Min.Y+2), image.NewUniform(border), image.Point{}, draw.Src)
	draw.Draw(destination, image.Rect(rectangle.Min.X, rectangle.Max.Y-2, rectangle.Max.X, rectangle.Max.Y), image.NewUniform(border), image.Point{}, draw.Src)
	draw.Draw(destination, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Min.X+2, rectangle.Max.Y), image.NewUniform(border), image.Point{}, draw.Src)
	draw.Draw(destination, image.Rect(rectangle.Max.X-2, rectangle.Min.Y, rectangle.Max.X, rectangle.Max.Y), image.NewUniform(border), image.Point{}, draw.Src)
}

func wrapSectionPremiseLabel(value string, face font.Face, maxWidth, maxLines int) []string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		return []string{"Untitled"}
	}
	lines := make([]string, 0, maxLines)
	for len(runes) > 0 && len(lines) < maxLines {
		end := 1
		lastBreak := 0
		for end <= len(runes) {
			candidate := strings.TrimSpace(string(runes[:end]))
			if font.MeasureString(face, candidate).Ceil() > maxWidth {
				break
			}
			if unicode.IsSpace(runes[end-1]) {
				lastBreak = end
			}
			end++
		}
		if end > len(runes) {
			lines = append(lines, strings.TrimSpace(string(runes)))
			runes = nil
			break
		}
		cut := max(1, end-1)
		if lastBreak > 0 {
			cut = lastBreak
		}
		lines = append(lines, strings.TrimSpace(string(runes[:cut])))
		runes = []rune(strings.TrimSpace(string(runes[cut:])))
	}
	if len(runes) > 0 && len(lines) > 0 {
		last := []rune(lines[len(lines)-1])
		for len(last) > 0 && font.MeasureString(face, string(last)+"…").Ceil() > maxWidth {
			last = last[:len(last)-1]
		}
		lines[len(lines)-1] = strings.TrimSpace(string(last)) + "…"
	}
	return lines
}

func LoadSectionPremiseFont(labels, paths []string) (font.Face, error) {
	required := make(map[rune]struct{})
	requiresCJK := false
	for _, label := range labels {
		for _, current := range label {
			if unicode.IsSpace(current) {
				continue
			}
			required[current] = struct{}{}
			if current > unicode.MaxASCII {
				requiresCJK = true
			}
		}
	}
	if !requiresCJK {
		return basicfont.Face7x13, nil
	}
	var fontErrors []error
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				fontErrors = append(fontErrors, fmt.Errorf("%s: %w", path, err))
			}
			continue
		}
		collection, err := opentype.ParseCollectionReaderAt(file)
		if err != nil {
			_ = file.Close()
			fontErrors = append(fontErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		for index := 0; index < collection.NumFonts(); index++ {
			parsed, parseErr := collection.Font(index)
			if parseErr != nil {
				fontErrors = append(fontErrors, fmt.Errorf("%s font %d: %w", path, index, parseErr))
				continue
			}
			face, faceErr := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 18, DPI: 72, Hinting: font.HintingFull})
			if faceErr != nil {
				fontErrors = append(fontErrors, fmt.Errorf("%s font %d: %w", path, index, faceErr))
				continue
			}
			if sectionPremiseFontSupports(face, required) {
				return &sectionPremiseFontFace{Face: face, file: file}, nil
			}
			_ = face.Close()
		}
		_ = file.Close()
	}
	message := "no system CJK font covers all section premise labels"
	if len(fontErrors) > 0 {
		return nil, fmt.Errorf("%s: %w", message, errors.Join(fontErrors...))
	}
	return nil, errors.New(message)
}

func sectionPremiseFontSupports(face font.Face, required map[rune]struct{}) bool {
	for current := range required {
		if _, ok := face.GlyphAdvance(current); !ok {
			return false
		}
	}
	return true
}

type sectionPremiseFontFace struct {
	font.Face
	file *os.File
}

func (face *sectionPremiseFontFace) Close() error {
	return errors.Join(face.Face.Close(), face.file.Close())
}
