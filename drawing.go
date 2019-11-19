package smartcrop

import (
	"image"
	"image/color"
)

// HLine draws a horizontal line
func hLine(img *image.RGBA, col color.Color, y, x1, x2 int) {
	for ; x1 <= x2; x1++ {
		img.Set(x1, y, col)
	}
}

// VLine draws a veritcal line
func vLine(img *image.RGBA, col color.Color, x, y1, y2 int) {
	for ; y1 <= y2; y1++ {
		img.Set(x, y1, col)
	}
}

// Rect draws a rectangle utilizing HLine() and VLine()
func drawRect(img *image.RGBA, col color.Color, r image.Rectangle) {
	hLine(img, col, r.Min.Y, r.Min.X, r.Max.X)
	hLine(img, col, r.Max.Y, r.Min.X, r.Max.X)
	vLine(img, col, r.Min.X, r.Min.Y, r.Max.Y)
	vLine(img, col, r.Max.X, r.Min.Y, r.Max.Y)
}
