/*
 * Copyright (c) 2014-2017 Christian Muehlhaeuser
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 *	Authors:
 *		Christian Muehlhaeuser <muesli@gmail.com>
 *		Michael Wendland <michael@michiwend.com>
 *		Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>
 */

/*
Package smartcrop implements a content aware image cropping library based on
Jonas Wagner's smartcrop.js https://github.com/jwagner/smartcrop.js
*/
package smartcrop

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"log"
	"math"
	"time"

	"github.com/third-light/smartcrop/options"

	"golang.org/x/image/draw"
	"gocv.io/x/gocv"
)

var (
	// ErrInvalidDimensions gets returned when the supplied dimensions are invalid
	ErrInvalidDimensions = errors.New("Expect either a height or width")

	skinColor = [3]float64{0.78, 0.57, 0.44}
)

// Analyzer interface analyzes its struct and returns the best possible crop with the given
// width and height returns an error if invalid
type Analyzer interface {
	FindBestCrop(img image.Image, width, height int) (image.Rectangle, error)
	FindAllCrops(img image.Image, width, height int) ([]Crop, error)
}

// Score contains values that classify matches
type Score struct {
	Detail     float64
	Saturation float64
	Skin       float64
	Face       float64
	Total      float64
}

// Crop contains results
type Crop struct {
	image.Rectangle
	Score Score
}

func (c Crop) String() string {
	return fmt.Sprintf("%d,%d - %d,%d (%f)", c.Min.X, c.Min.Y, c.Max.X, c.Max.Y, c.Score.Total)
}

// Logger contains a logger.
type Logger struct {
	DebugMode bool
	Log       *log.Logger
}

type smartcropAnalyzer struct {
	logger Logger
	options.Resizer
	config Config
}

// NewAnalyzer returns a new Analyzer using the given Resizer.
func NewAnalyzer(c Config, resizer options.Resizer) Analyzer {
	logger := Logger{
		DebugMode: false,
	}

	return NewAnalyzerWithLogger(c, resizer, logger)
}

// NewAnalyzerWithLogger returns a new analyzer with the given Resizer and Logger.
func NewAnalyzerWithLogger(c Config, resizer options.Resizer, logger Logger) Analyzer {
	if logger.Log == nil {
		logger.Log = log.New(ioutil.Discard, "", 0)
	}
	return &smartcropAnalyzer{Resizer: resizer, logger: logger, config: c}
}

func (sca smartcropAnalyzer) preprocessForAnalysis(img image.Image, width, height int) (*image.RGBA, float64, float64, float64, float64) {
	// resize image for faster processing
	scale := math.Min(float64(img.Bounds().Dx())/float64(width), float64(img.Bounds().Dy())/float64(height))
	var rgbaImg *image.RGBA
	var prescalefactor = 1.0

	if sca.config.Prescale {
		if f := sca.config.PrescaleMin / math.Min(float64(img.Bounds().Dx()), float64(img.Bounds().Dy())); f < 1.0 {
			prescalefactor = f
		}
		sca.logger.Log.Println(prescalefactor)

		smallimg := sca.Resize(
			img,
			uint(float64(img.Bounds().Dx())*prescalefactor),
			0)

		rgbaImg = toRGBA(smallimg)
	} else {
		rgbaImg = toRGBA(img)
	}

	if sca.logger.DebugMode {
		writeImage("png", rgbaImg, "./smartcrop_prescale.png")
	}

	cropWidth, cropHeight := chop(float64(width)*scale*prescalefactor), chop(float64(height)*scale*prescalefactor)
	realMinScale := math.Min(sca.config.MaxScale, math.Max(1.0/scale, sca.config.MinScale))

	sca.logger.Log.Printf("original resolution: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())
	sca.logger.Log.Printf("scale: %f, cropw: %f, croph: %f, minscale: %f\n", scale, cropWidth, cropHeight, realMinScale)

	return rgbaImg, cropWidth, cropHeight, realMinScale, prescalefactor
}

func (sca smartcropAnalyzer) FindBestCrop(img image.Image, width, height int) (image.Rectangle, error) {
	if width == 0 && height == 0 {
		return image.Rectangle{}, ErrInvalidDimensions
	}

	rgbaImg, cropWidth, cropHeight, realMinScale, prescalefactor := sca.preprocessForAnalysis(img, width, height)

	allCrops, processedImg := sca.analyse(rgbaImg, cropWidth, cropHeight, realMinScale)
	topCrop := sca.findTopCrop(allCrops)

	if sca.logger.DebugMode {
		sca.drawDebugCrop(topCrop, processedImg)
		debugOutput(true, processedImg, "final")
	}

	if sca.config.Prescale == true {
		topCrop.Min.X = int(chop(float64(topCrop.Min.X) / prescalefactor))
		topCrop.Min.Y = int(chop(float64(topCrop.Min.Y) / prescalefactor))
		topCrop.Max.X = int(chop(float64(topCrop.Max.X) / prescalefactor))
		topCrop.Max.Y = int(chop(float64(topCrop.Max.Y) / prescalefactor))
	}

	return topCrop.Canon(), nil
}

func (sca smartcropAnalyzer) FindAllCrops(img image.Image, width, height int) ([]Crop, error) {
	if width == 0 && height == 0 {
		return []Crop{}, ErrInvalidDimensions
	}

	rgbaImg, cropWidth, cropHeight, realMinScale, prescalefactor := sca.preprocessForAnalysis(img, width, height)

	allCrops, _ := sca.analyse(rgbaImg, cropWidth, cropHeight, realMinScale)

	for i, crop := range allCrops {
		if sca.config.Prescale == true {
			allCrops[i].Min.X = int(chop(float64(crop.Min.X) / prescalefactor))
			allCrops[i].Min.Y = int(chop(float64(crop.Min.Y) / prescalefactor))
			allCrops[i].Max.X = int(chop(float64(crop.Max.X) / prescalefactor))
			allCrops[i].Max.Y = int(chop(float64(crop.Max.Y) / prescalefactor))
		}
		crop.Rectangle = crop.Canon()
	}

	return allCrops, nil
}

func chop(x float64) float64 {
	if x < 0 {
		return math.Ceil(x)
	}
	return math.Floor(x)
}

func thirds(x float64) float64 {
	x = (math.Mod(x-(1.0/3.0)+1.0, 2.0)*0.5 - 0.5) * 16.0
	return math.Max(1.0-x*x, 0.0)
}

func bounds(l float64) float64 {
	return math.Min(math.Max(l, 0.0), 255)
}

func (sca smartcropAnalyzer) importance(crop Crop, x, y int) float64 {
	if crop.Min.X > x || x >= crop.Max.X || crop.Min.Y > y || y >= crop.Max.Y {
		return sca.config.OutsideImportance
	}

	xf := float64(x-crop.Min.X) / float64(crop.Dx())
	yf := float64(y-crop.Min.Y) / float64(crop.Dy())

	px := math.Abs(0.5-xf) * 2.0
	py := math.Abs(0.5-yf) * 2.0

	dx := math.Max(px-1.0+sca.config.EdgeRadius, 0.0)
	dy := math.Max(py-1.0+sca.config.EdgeRadius, 0.0)
	d := (dx*dx + dy*dy) * sca.config.EdgeWeight

	s := 1.41 - math.Sqrt(px*px+py*py)
	if sca.config.RuleOfThirds {
		s += (math.Max(0.0, s+d+0.5) * 1.2) * (thirds(px) + thirds(py))
	}

	return s + d
}

func (sca smartcropAnalyzer) score(output *image.RGBA, crop Crop, faceRescts []image.Rectangle) Score {
	width := output.Bounds().Dx()
	height := output.Bounds().Dy()
	score := Score{}

	// same loops but with downsampling
	//for y := 0; y < height; y++ {
	//for x := 0; x < width; x++ {
	for y := 0; y <= height-sca.config.ScoreDownSample; y += sca.config.ScoreDownSample {
		for x := 0; x <= width-sca.config.ScoreDownSample; x += sca.config.ScoreDownSample {

			c := output.RGBAAt(x, y)
			r8 := float64(c.R)
			g8 := float64(c.G)
			b8 := float64(c.B)

			imp := sca.importance(crop, int(x), int(y))
			det := g8 / 255.0

			score.Skin += r8 / 255.0 * (det + sca.config.SkinBias) * imp
			score.Detail += det * imp
			score.Saturation += b8 / 255.0 * (det + sca.config.SaturationBias) * imp
		}
	}

	if oca.FaceDetectEnabled {
		// Score for face is based on the proportion of the crop taken up by a face
		cropRes := crop.Bounds().Dx() * crop.Bounds().Dy()
		for _ , r := range faceRects {
			if r.In(crop.Rectangle) {
				faceRes := r.Bounds().Dx() * r.Bounds().Dy()
				score.Face += float64(faceRes) / float64(cropRes)
			}
		}
	}

	score.Total = (score.Detail*sca.config.DetailWeight + score.Skin*sca.config.SkinWeight + score.Saturation*sca.config.SaturationWeight)
	score.Total = score.Total / (float64(crop.Dx()) * float64(crop.Dy()))
	score.Total = score.Total + score.Face

	return score
}

func (sca smartcropAnalyzer) analyse(img *image.RGBA, cropWidth, cropHeight, realMinScale float64) ([]Crop, *image.RGBA) {
	o := image.NewRGBA(img.Bounds())

	now := time.Now()
	sca.edgeDetect(img, o)
	sca.logger.Log.Println("Time elapsed edge:", time.Since(now))
	debugOutput(sca.logger.DebugMode, o, "edge")

	now = time.Now()
	sca.skinDetect(img, o)
	sca.logger.Log.Println("Time elapsed skin:", time.Since(now))
	debugOutput(sca.logger.DebugMode, o, "skin")

	now = time.Now()
	sca.saturationDetect(img, o)
	sca.logger.Log.Println("Time elapsed sat:", time.Since(now))
	debugOutput(sca.logger.DebugMode, o, "saturation")

	var faceRects []image.Rectangle
	if oca.config.FaceDetectEnabled {
		now = time.Now()
		var faceOut *image.RGBA
		if sca.logger.DebugMode {
			// Copy current output image so we can draw face rects on to new output
			// We need a copy because o is used for scoring later.
			faceOut = image.NewRGBA(img.Bounds())
			draw.Copy(faceOut, image.Pt(0, 0), img, img.Bounds(), draw.Src, nil)
		}
		faceRects = sca.faceDetect(img, faceOut)
		sca.logger.Log.Println("Time elapsed face:", time.Since(now))
		debugOutput(sca.logger.DebugMode, faceOut, "facedetect")
	}

	now = time.Now()
	cs := sca.crops(o, cropWidth, cropHeight, realMinScale)
	sca.logger.Log.Println("Time elapsed crops:", time.Since(now), len(cs))

	// evaluate the scores for each candidate crop, and update the Score field of each crop object
	now = time.Now()
	for i, crop := range cs {
		nowIn := time.Now()
		cs[i].Score = sca.score(o, crop)
		sca.logger.Log.Println("Time elapsed single-score:", time.Since(nowIn))
	}
	sca.logger.Log.Println("Time elapsed score:", time.Since(now))

	return cs, o
}

func (sca smartcropAnalyzer) findTopCrop(cs []Crop) Crop {
	var topCrop Crop
	topScore := -1.0
	for _, crop := range cs {
		if crop.Score.Total > topScore {
			topCrop = crop
			topScore = crop.Score.Total
		}
	}
	return topCrop
}

func saturation(c color.RGBA) float64 {
	cMax, cMin := uint8(0), uint8(255)
	if c.R > cMax {
		cMax = c.R
	}
	if c.R < cMin {
		cMin = c.R
	}
	if c.G > cMax {
		cMax = c.G
	}
	if c.G < cMin {
		cMin = c.G
	}
	if c.B > cMax {
		cMax = c.B
	}
	if c.B < cMin {
		cMin = c.B
	}

	if cMax == cMin {
		return 0
	}
	maximum := float64(cMax) / 255.0
	minimum := float64(cMin) / 255.0

	l := (maximum + minimum) / 2.0
	d := maximum - minimum

	if l > 0.5 {
		return d / (2.0 - maximum - minimum)
	}

	return d / (maximum + minimum)
}

func cie(c color.RGBA) float64 {
	return 0.5126*float64(c.B) + 0.7152*float64(c.G) + 0.0722*float64(c.R)
}

func skinCol(c color.RGBA) float64 {
	r8, g8, b8 := float64(c.R), float64(c.G), float64(c.B)

	mag := math.Sqrt(r8*r8 + g8*g8 + b8*b8)
	rd := r8/mag - skinColor[0]
	gd := g8/mag - skinColor[1]
	bd := b8/mag - skinColor[2]

	d := math.Sqrt(rd*rd + gd*gd + bd*bd)
	return 1.0 - d
}

func makeCies(img *image.RGBA) []float64 {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	cies := make([]float64, width*height, width*height)
	i := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cies[i] = cie(img.RGBAAt(x, y))
			i++
		}
	}

	return cies
}

func (sca smartcropAnalyzer) edgeDetect(i *image.RGBA, o *image.RGBA) {
	width := i.Bounds().Dx()
	height := i.Bounds().Dy()
	cies := makeCies(i)

	var lightness float64
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if x == 0 || x >= width-1 || y == 0 || y >= height-1 {
				//lightness = cie((*i).At(x, y))
				lightness = 0
			} else {
				lightness = cies[y*width+x]*4.0 -
					cies[x+(y-1)*width] -
					cies[x-1+y*width] -
					cies[x+1+y*width] -
					cies[x+(y+1)*width]
			}

			nc := color.RGBA{0, uint8(bounds(lightness)), 0, 255}
			o.SetRGBA(x, y, nc)
		}
	}
}

func (sca smartcropAnalyzer) skinDetect(i *image.RGBA, o *image.RGBA) {
	width := i.Bounds().Dx()
	height := i.Bounds().Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			lightness := cie(i.RGBAAt(x, y)) / 255.0
			skin := skinCol(i.RGBAAt(x, y))

			c := o.RGBAAt(x, y)
			if skin > sca.config.SkinThreshold && lightness >= sca.config.SkinBrightnessMin && lightness <= sca.config.SkinBrightnessMax {
				r := (skin - sca.config.SkinThreshold) * (255.0 / (1.0 - sca.config.SkinThreshold))
				nc := color.RGBA{uint8(bounds(r)), c.G, c.B, 255}
				o.SetRGBA(x, y, nc)
			} else {
				nc := color.RGBA{0, c.G, c.B, 255}
				o.SetRGBA(x, y, nc)
			}
		}
	}
}

func (sca smartcropAnalyzer) saturationDetect(i *image.RGBA, o *image.RGBA) {
	width := i.Bounds().Dx()
	height := i.Bounds().Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			lightness := cie(i.RGBAAt(x, y)) / 255.0
			saturation := saturation(i.RGBAAt(x, y))

			c := o.RGBAAt(x, y)
			if saturation > sca.config.SaturationThreshold && lightness >= sca.config.SaturationBrightnessMin && lightness <= sca.config.SaturationBrightnessMax {
				b := (saturation - sca.config.SaturationThreshold) * (255.0 / (1.0 - sca.config.SaturationThreshold))
				nc := color.RGBA{c.R, c.G, uint8(bounds(b)), 255}
				o.SetRGBA(x, y, nc)
			} else {
				nc := color.RGBA{c.R, c.G, 0, 255}
				o.SetRGBA(x, y, nc)
			}
		}
	}
}

func (sca smartcropAnalyzer) faceDetect(i *image.RGBA, o *image.RGBA) []image.Rectangle {

	img, err := gocv.ImageToMatRGBA(i)
	if err != nil {
		if sca.logger.DebugMode {
			sca.logger.Log.Printf("failed converting img to MatRGBA: %v", err)
		}
		return nil
	}

	classifier := gocv.NewCascadeClassifier()
	defer classifier.Close()

	if !classifier.Load(sca.FaceDetectClassifierFile) {
		panic(fmt.Errorf("Failed loading classifier file at %s", sca.config.FaceDetectClassifierFile))
	}

	rects := classifier.DetectMultiScale(img)
	faceRects := []image.Rectangle{}

	// Filter out the rects with too small area as they are unlikely to be important for smart
	// cropping. We say a face must consume at least 5% of image to be considered.
	origRes := i.Bounds().Dx() * i.Bounds().Dy()
	thresholdRes := 0.05 * float64(origRes)
	for _, r := range rects {
		if r.Size().X*r.Size().Y > thresholdRes {
			faceRects = append(faceRects, r)
		}
	}

	// Draw face rects on to output image to see what the algorithm is actually doing
	// o might be nil - when not in debug mode
	if o != nil {
		boxColor := color.RGBA{255, 0, 0, 0}
		for _, r := range faceRects {
		drawRect(o, boxColor, r)
	}

	return faceRects
}

func (sca smartcropAnalyzer) crops(i image.Image, cropWidth, cropHeight, realMinScale float64) []Crop {
	res := []Crop{}
	width := i.Bounds().Dx()
	height := i.Bounds().Dy()

	minDimension := math.Min(float64(width), float64(height))
	var cropW, cropH float64

	if cropWidth != 0.0 {
		cropW = cropWidth
	} else {
		cropW = minDimension
	}
	if cropHeight != 0.0 {
		cropH = cropHeight
	} else {
		cropH = minDimension
	}

	for scale := sca.config.MaxScale; scale >= realMinScale; scale -= sca.config.ScaleStep {
		for y := 0; float64(y)+cropH*scale <= float64(height); y += sca.config.Step {
			for x := 0; float64(x)+cropW*scale <= float64(width); x += sca.config.Step {
				res = append(res, Crop{
					Rectangle: image.Rect(x, y, x+int(cropW*scale), y+int(cropH*scale)),
				})
			}
		}
	}

	return res
}

func (sca smartcropAnalyzer) drawDebugCrop(topCrop Crop, o *image.RGBA) {
	width := o.Bounds().Dx()
	height := o.Bounds().Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := o.At(x, y).RGBA()
			r8 := float64(r >> 8)
			g8 := float64(g >> 8)
			b8 := uint8(b >> 8)

			imp := sca.importance(topCrop, x, y)

			if imp > 0 {
				g8 += imp * 32
			} else if imp < 0 {
				r8 += imp * -64
			}

			nc := color.RGBA{uint8(bounds(r8)), uint8(bounds(g8)), b8, 255}
			o.SetRGBA(x, y, nc)
		}
	}
}

// toRGBA converts an image.Image to an image.RGBA
func toRGBA(img image.Image) *image.RGBA {
	switch img.(type) {
	case *image.RGBA:
		return img.(*image.RGBA)
	}
	out := image.NewRGBA(img.Bounds())
	draw.Copy(out, image.Pt(0, 0), img, img.Bounds(), draw.Src, nil)
	return out
}
