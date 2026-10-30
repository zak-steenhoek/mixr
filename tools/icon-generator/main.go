// Command icon-generator turns mixr's deliberately simple SVG artwork into
// the multi-resolution ICO and Go byte slice consumed by the application.
// It supports only the SVG rectangle subset used by mixr-icon.svg so the
// branding build remains deterministic and dependency-free.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var iconSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

type svgDocument struct {
	Width  float64   `xml:"width,attr"`
	Height float64   `xml:"height,attr"`
	Rects  []svgRect `xml:"rect"`
}

type svgRect struct {
	X      float64 `xml:"x,attr"`
	Y      float64 `xml:"y,attr"`
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
	Radius float64 `xml:"rx,attr"`
	Fill   string  `xml:"fill,attr"`
}

func main() {
	sourcePath := flag.String("source", "pkg/mixr/assets/mixr-icon.svg", "editable SVG source artwork")
	icoPath := flag.String("ico", "pkg/mixr/assets/mixr.ico", "generated multi-resolution ICO")
	goPath := flag.String("go", "pkg/mixr/icon/icon.go", "generated Go icon byte slice")
	editIconPath := flag.String("edit-icon", "pkg/mixr/assets/menu-items/edit-config.ico", "edit-config menu ICO")
	refreshIconPath := flag.String("refresh-icon", "pkg/mixr/assets/menu-items/refresh-sessions.ico", "refresh-sessions menu ICO")
	menuGoPath := flag.String("menu-go", "pkg/mixr/icon/menu_icons.go", "generated Go menu icon byte slices")
	previewPath := flag.String("preview", "", "optional generated PNG preview")
	flag.Parse()

	doc, err := readSVG(*sourcePath)
	if err != nil {
		fatal(err)
	}

	frames := make([][]byte, 0, len(iconSizes))
	for _, size := range iconSizes {
		frame, err := renderPNG(doc, size)
		if err != nil {
			fatal(fmt.Errorf("render %dx%d: %w", size, size, err))
		}
		frames = append(frames, frame)
	}

	ico, err := encodeICO(iconSizes, frames)
	if err != nil {
		fatal(err)
	}
	if err := writeFile(*icoPath, ico); err != nil {
		fatal(err)
	}
	if err := writeFile(*goPath, goSource(ico)); err != nil {
		fatal(err)
	}
	editIcon, err := ioutil.ReadFile(*editIconPath)
	if err != nil {
		fatal(fmt.Errorf("read edit-config icon: %w", err))
	}
	refreshIcon, err := ioutil.ReadFile(*refreshIconPath)
	if err != nil {
		fatal(fmt.Errorf("read refresh-sessions icon: %w", err))
	}
	if err := writeFile(*menuGoPath, menuGoSource(editIcon, refreshIcon)); err != nil {
		fatal(err)
	}
	if *previewPath != "" {
		if err := writeFile(*previewPath, frames[len(frames)-1]); err != nil {
			fatal(err)
		}
	}
}

func readSVG(path string) (svgDocument, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return svgDocument{}, fmt.Errorf("read SVG: %w", err)
	}
	var doc svgDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return svgDocument{}, fmt.Errorf("parse SVG: %w", err)
	}
	if doc.Width <= 0 || doc.Height <= 0 || len(doc.Rects) == 0 {
		return svgDocument{}, fmt.Errorf("SVG needs positive dimensions and at least one rectangle")
	}
	if doc.Width != doc.Height {
		return svgDocument{}, fmt.Errorf("SVG must be square, got %.0fx%.0f", doc.Width, doc.Height)
	}
	return doc, nil
}

func renderPNG(doc svgDocument, size int) ([]byte, error) {
	const samples = 4
	highSize := size * samples
	high := image.NewNRGBA(image.Rect(0, 0, highSize, highSize))
	scale := float64(highSize) / doc.Width

	for _, rect := range doc.Rects {
		fill, err := parseColor(rect.Fill)
		if err != nil {
			return nil, err
		}
		drawRoundedRect(high, rect, scale, fill)
	}

	output := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for sampleY := 0; sampleY < samples; sampleY++ {
				for sampleX := 0; sampleX < samples; sampleX++ {
					pixel := high.NRGBAAt(x*samples+sampleX, y*samples+sampleY)
					r += uint32(pixel.R)
					g += uint32(pixel.G)
					b += uint32(pixel.B)
					a += uint32(pixel.A)
				}
			}
			const divisor = samples * samples
			output.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r / divisor),
				G: uint8(g / divisor),
				B: uint8(b / divisor),
				A: uint8(a / divisor),
			})
		}
	}

	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&encoded, output); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func drawRoundedRect(target *image.NRGBA, rect svgRect, scale float64, fill color.NRGBA) {
	left := rect.X * scale
	top := rect.Y * scale
	right := (rect.X + rect.Width) * scale
	bottom := (rect.Y + rect.Height) * scale
	radius := rect.Radius * scale

	for y := max(0, int(top)); y < min(target.Bounds().Dy(), int(bottom+1)); y++ {
		for x := max(0, int(left)); x < min(target.Bounds().Dx(), int(right+1)); x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			if roundedRectContains(px, py, left, top, right, bottom, radius) {
				target.SetNRGBA(x, y, fill)
			}
		}
	}
}

func roundedRectContains(x, y, left, top, right, bottom, radius float64) bool {
	if x < left || x >= right || y < top || y >= bottom {
		return false
	}
	if radius <= 0 {
		return true
	}
	cx := minFloat(maxFloat(x, left+radius), right-radius)
	cy := minFloat(maxFloat(y, top+radius), bottom-radius)
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= radius*radius
}

func parseColor(value string) (color.NRGBA, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 && len(value) != 8 {
		return color.NRGBA{}, fmt.Errorf("unsupported fill %q", value)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("parse fill: %w", err)
	}
	result := color.NRGBA{R: decoded[0], G: decoded[1], B: decoded[2], A: 255}
	if len(decoded) == 4 {
		result.A = decoded[3]
	}
	return result, nil
}

func encodeICO(sizes []int, frames [][]byte) ([]byte, error) {
	if len(sizes) != len(frames) || len(frames) > 65535 {
		return nil, fmt.Errorf("invalid ICO frame set")
	}
	var output bytes.Buffer
	writeUint16(&output, 0)
	writeUint16(&output, 1)
	writeUint16(&output, uint16(len(frames)))

	offset := 6 + 16*len(frames)
	for index, frame := range frames {
		size := sizes[index]
		dimension := byte(size)
		if size == 256 {
			dimension = 0
		}
		output.WriteByte(dimension)
		output.WriteByte(dimension)
		output.WriteByte(0)
		output.WriteByte(0)
		writeUint16(&output, 1)
		writeUint16(&output, 32)
		writeUint32(&output, uint32(len(frame)))
		writeUint32(&output, uint32(offset))
		offset += len(frame)
	}
	for _, frame := range frames {
		output.Write(frame)
	}
	return output.Bytes(), nil
}

func goSource(icon []byte) []byte {
	var output strings.Builder
	output.WriteString("// Code generated by go run ./tools/icon-generator; DO NOT EDIT.\n")
	output.WriteString("\npackage icon\n\n")
	output.WriteString("// MixrLogo is the embedded multi-resolution application icon.\n")
	writeByteSlice(&output, "MixrLogo", icon)
	return []byte(output.String())
}

func menuGoSource(editIcon, refreshIcon []byte) []byte {
	var output strings.Builder
	output.WriteString("// Code generated by go run ./tools/icon-generator; DO NOT EDIT.\n")
	output.WriteString("\npackage icon\n\n")
	output.WriteString("// EditConfig and RefreshSessions are the embedded tray-menu icons.\n")
	writeByteSlice(&output, "EditConfig", editIcon)
	output.WriteString("\n")
	writeByteSlice(&output, "RefreshSessions", refreshIcon)
	return []byte(output.String())
}

func writeByteSlice(output *strings.Builder, name string, data []byte) {
	output.WriteString("var ")
	output.WriteString(name)
	output.WriteString(" = []byte{\n")
	for index, value := range data {
		if index%12 == 0 {
			output.WriteString("\t")
		}
		output.WriteString("0x")
		output.WriteString(strconv.FormatUint(uint64(value), 16))
		output.WriteString(",")
		if index%12 == 11 || index == len(data)-1 {
			output.WriteString("\n")
		} else {
			output.WriteString(" ")
		}
	}
	output.WriteString("}\n")
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeUint16(writer io.Writer, value uint16) {
	_ = binary.Write(writer, binary.LittleEndian, value)
}

func writeUint32(writer io.Writer, value uint32) {
	_ = binary.Write(writer, binary.LittleEndian, value)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "icon-generator:", err)
	os.Exit(1)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
