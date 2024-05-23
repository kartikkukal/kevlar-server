package img

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"strings"

	go_draw "golang.org/x/image/draw"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	font_size    = 8.0 * 25.0 // In points
	resized_size = 600
)

var (
	font_color = color.Black

	//go:embed asset/template.jpg
	template_file []byte

	//go:embed asset/poppins.ttf
	poppins_file []byte
)

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}

	return value
}

func GenerateImage(username string) ([]byte, error) {

	words := strings.Split(username, " ")

	var initials string

	for index, value := range words {
		if index > 1 {
			break
		}
		if len(value) > 0 {
			initials += strings.ToUpper(string(value[0])) + "."
		}
	}

	// Open font as font.Face
	font_true, err := truetype.Parse(poppins_file)
	if err != nil {
		return nil, err
	}

	font_face := truetype.NewFace(font_true, &truetype.Options{
		Hinting: font.HintingFull,
		Size:    font_size,
	})

	// Open source
	source, _, err := image.Decode(bytes.NewReader(template_file))
	if err != nil {
		return nil, err
	}

	// Copy source to image.RGBA
	template := image.NewRGBA(source.Bounds())
	draw.Draw(template, template.Bounds(), source, source.Bounds().Min, draw.Src)

	// Get width and height of string to be drawn
	drawer := &font.Drawer{
		Face: font_face,
	}

	// Calculate center position
	text_width := float64(drawer.MeasureString(initials) >> 6)
	text_height := font_size * 72 / 96

	image_width := float64(source.Bounds().Size().X)
	image_height := float64(source.Bounds().Size().Y)

	x_offset := clamp((image_width-text_width)/2, 0, image_width)
	y_offset := clamp((image_height+text_height)/2, 0, image_height)

	offset := fixed.Point26_6{
		X: fixed.I(int(x_offset)),
		Y: fixed.I(int(y_offset)),
	}

	// Draw text onto image
	drawer = &font.Drawer{
		Dst:  template,
		Src:  image.NewUniform(font_color),
		Face: font_face,
		Dot:  offset,
	}

	drawer.DrawString(initials)

	buffer := bytes.NewBuffer(nil)

	err = png.Encode(buffer, template)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func ResizeImage(input []byte) ([]byte, error) {
	source, _, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}

	// Do not resize if image is already small
	if source.Bounds().Dx() <= resized_size && source.Bounds().Dy() <= resized_size {
		return input, nil
	}

	// Scale image to resized_size
	resized := image.NewRGBA(image.Rect(0, 0, resized_size, resized_size))
	go_draw.CatmullRom.Scale(resized, resized.Bounds(), source, source.Bounds(), draw.Over, nil)

	buffer := bytes.NewBuffer(nil)

	err = png.Encode(buffer, resized)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
