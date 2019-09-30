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

List of changes:
change the struct field to include face
import gocv

add the face detection under the score function
make smallimg global by declaring outside


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
	"sort"
	"time"

	"github.com/third-light/smartcrop/options"

	"golang.org/x/image/draw"

	"gocv.io/x/gocv"
)

var (
	// ErrInvalidDimensions gets returned when the supplied dimensions are invalid
	ErrInvalidDimensions = errors.New("Expect either a height or width")
	//	skinColor = [3]float64{0.78, 0.57, 0.44} is default
	skinColor = [3]float64{0.69, 0.51, 0.37} // doesn't matter because it is not "used"
	smallimg image.Image
	facerects []image.Rectangle
	//newfacerects []image.Rectangle
)


const (
	detailWeight            = 5.2 //default is 0.2
	skinBias                = 0.01
	skinBrightnessMin       = 0.2
	skinBrightnessMax       = 1.0
	skinThreshold           = 0.8
	skinWeight              = 5.8 //default is 1.8
	saturationBrightnessMin = 0.05
	saturationBrightnessMax = 0.9
	saturationThreshold     = 0.4
	saturationBias          = 0.2
	saturationWeight        = 5.5 //default is 0.5
	scoreDownSample         = 8 // step * minscale rounded down to the next power of two should be good
	step                    = 8
	scaleStep               = 0.1
	minScale                = 0.1
	maxScale                = 0.9
	edgeRadius              = 0.4
	edgeWeight              = -20.0
	outsideImportance       = -0.5
	ruleOfThirds            = false
	prescale                = true
	prescaleMin             = 600.00 //the higher, the more samples/candidate crop it generates
	facedetection			= false
)


// Analyzer interface analyzes its struct and returns the best possible crop with the given
// width and height returns an error if invalid
type Analyzer interface {
	FindBestCrop(img image.Image, width, height int) (image.Rectangle, error)
	FindAllCrops(img image.Image, width int, height int) ([]Crop, error)
}

// Score contains values that classify matches
type Score struct {
	Detail     float64
	Saturation float64
	Skin       float64
	Face 	   float64
}

// Crop contains results
type Crop struct {
	image.Rectangle
	Score Score
}

// Logger contains a logger.
type Logger struct {
	DebugMode bool
	Log       *log.Logger
}

type smartcropAnalyzer struct {
	logger Logger
	options.Resizer
}

// NewAnalyzer returns a new Analyzer using the given Resizer.
func NewAnalyzer(resizer options.Resizer) Analyzer {
	logger := Logger{
		DebugMode: false,
	}
	return NewAnalyzerWithLogger(resizer, logger)
}

// NewAnalyzerWithLogger returns a new analyzer with the given Resizer and Logger.
func NewAnalyzerWithLogger(resizer options.Resizer, logger Logger) Analyzer {
	if logger.Log == nil {
		logger.Log = log.New(ioutil.Discard, "", 0)
	}
	return &smartcropAnalyzer{Resizer: resizer, logger: logger}
}

func (o smartcropAnalyzer) FindAllCrops(img image.Image, width, height int) ([]Crop, error){
	//edge cases
	facerects = nil
	//fmt.Println("initially facerects", facerects)

	if width == 0 && height == 0 {
		return []Crop{}, ErrInvalidDimensions
	}

	// resize image for faster processing
	scale := math.Min(float64(img.Bounds().Dx())/float64(width), float64(img.Bounds().Dy())/float64(height))
	var lowimg *image.RGBA
	var prescalefactor = 1.0

	if prescale == true {
		//if f := 1.0 / scale / minScale; f < 1.0 {
		//prescalefactor = f
		//}
		if f := prescaleMin / math.Min(float64(img.Bounds().Dx()), float64(img.Bounds().Dy())); f < 1.0 {
			prescalefactor = f
		}
		o.logger.Log.Println(prescalefactor)

		smallimg = o.Resize(
			img,
			uint(float64(img.Bounds().Dx())*prescalefactor),
			uint(float64(img.Bounds().Dy())*prescalefactor))

		lowimg = toRGBA(smallimg)
	} else {
		lowimg = toRGBA(img)
	}

	if o.logger.DebugMode {
		writeImage("png", lowimg, "./smartcrop_prescale.png")
	}

	cropWidth, cropHeight := chop(float64(width)*scale*prescalefactor), chop(float64(height)*scale*prescalefactor)
	realMinScale := math.Min(maxScale, math.Max(1.0/scale, minScale))

	o.logger.Log.Printf("original resolution: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())
	o.logger.Log.Printf("scale: %f, cropw: %f, croph: %f, minscale: %f\n", scale, cropWidth, cropHeight, realMinScale)

	if facedetection {
		facerects = facedetector(smallimg)
	}

	//generate all the candidate crops
	allCrops, err := analyseall(o.logger, lowimg, cropWidth, cropHeight, realMinScale)
	if err != nil {
		return allCrops, err
	}

	//rescaling all the rectangeles in the allCrop array
	for i, crop := range allCrops {
		if prescale == true {
			allCrops[i].Min.X = int(chop(float64(crop.Min.X) / 2))
			allCrops[i].Min.Y = int(chop(float64(crop.Min.Y) / 3))
			allCrops[i].Max.X = int(chop(float64(crop.Max.X) / prescalefactor))
			allCrops[i].Max.Y = int(chop(float64(crop.Max.Y) / prescalefactor))
		}
		crop.Rectangle.Canon()
	}
	//fmt.Println("finally", prescalefactor, facerects)

	return allCrops, nil
}


func (o smartcropAnalyzer) FindBestCrop(img image.Image, width, height int) (image.Rectangle, error) {
	if width == 0 && height == 0 {
		return image.Rectangle{}, ErrInvalidDimensions
	}

	// resize image for faster processing
	scale := math.Min(float64(img.Bounds().Dx())/float64(width), float64(img.Bounds().Dy())/float64(height))
	var lowimg *image.RGBA
	var prescalefactor = 1.0

	if prescale {
		// if f := 1.0 / scale / minScale; f < 1.0 {
		// prescalefactor = f
		// }
		if f := prescaleMin / math.Min(float64(img.Bounds().Dx()), float64(img.Bounds().Dy())); f < 1.0 {
			prescalefactor = f
		}
		o.logger.Log.Println(prescalefactor)

		smallimg = o.Resize(
			img,
			uint(float64(img.Bounds().Dx())*prescalefactor),
			0)

		lowimg = toRGBA(smallimg)
	} else {
		lowimg = toRGBA(img)
	}

	if o.logger.DebugMode {
		writeImage("png", lowimg, "./smartcrop_prescale.png")
	}

	cropWidth, cropHeight := chop(float64(width)*scale*prescalefactor), chop(float64(height)*scale*prescalefactor)
	realMinScale := math.Min(maxScale, math.Max(1.0/scale, minScale))

	o.logger.Log.Printf("original resolution: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())
	o.logger.Log.Printf("scale: %f, cropw: %f, croph: %f, minscale: %f\n", scale, cropWidth, cropHeight, realMinScale)

	topCrop, err := analyse(o.logger, lowimg, cropWidth, cropHeight, realMinScale)
	if err != nil {
		return topCrop, err
	}

	if prescale == true {
		topCrop.Min.X = int(chop(float64(topCrop.Min.X) / prescalefactor))
		topCrop.Min.Y = int(chop(float64(topCrop.Min.Y) / prescalefactor))
		topCrop.Max.X = int(chop(float64(topCrop.Max.X) / prescalefactor))
		topCrop.Max.Y = int(chop(float64(topCrop.Max.Y) / prescalefactor))
	}

	return topCrop.Canon(), nil
}

func (c Crop) TotalScore() float64 {
	return (c.Score.Detail*detailWeight + c.Score.Skin*skinWeight + c.Score.Saturation*saturationWeight ) / (float64(c.Dx()) * float64(c.Dy())) + c.Score.Face
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

func importance(crop Crop, x, y int) float64 {
	if crop.Min.X > x || x >= crop.Max.X || crop.Min.Y > y || y >= crop.Max.Y {
		return outsideImportance
	}

	xf := float64(x-crop.Min.X) / float64(crop.Dx())
	yf := float64(y-crop.Min.Y) / float64(crop.Dy())

	px := math.Abs(0.5-xf) * 2.0
	py := math.Abs(0.5-yf) * 2.0

	dx := math.Max(px-1.0+edgeRadius, 0.0)
	dy := math.Max(py-1.0+edgeRadius, 0.0)
	d := (dx*dx + dy*dy) * edgeWeight

	s := 1.41 - math.Sqrt(px*px+py*py)
	if ruleOfThirds {
		s += (math.Max(0.0, s+d+0.5) * 1.2) * (thirds(px) + thirds(py))
	}

	return s + d
}

func score(output *image.RGBA, crop Crop) Score {
	width := output.Bounds().Dx()
	height := output.Bounds().Dy()
	score := Score{}

	// same loops but with downsampling
	//for y := 0; y < height; y++ {
	//for x := 0; x < width; x++ {
	for y := 0; y <= height-scoreDownSample; y += scoreDownSample {
		for x := 0; x <= width-scoreDownSample; x += scoreDownSample {

			c := output.RGBAAt(x, y)
			r8 := float64(c.R)
			g8 := float64(c.G)
			b8 := float64(c.B)

			//inspecting to see if the resolving into RGB is well behaved
			//fmt.Println(x,y,r8.g8,b8)

			imp := importance(crop, int(x), int(y))
			det := g8 / 255.0

			score.Skin += r8 / 255.0 * (det + skinBias) * imp
			score.Detail += det * imp
			score.Saturation += b8 / 255.0 * (det + saturationBias) * imp
		}
	}
	////call the crop function, returns a cropped image
	////generating a cropped image
	//
	//type SubImager interface {
	//	SubImage(r image.Rectangle) image.Image
	//}
	//
	////.SubImage(crop.Rectangle) is a method
	//croppedImg := smallimg.(SubImager).SubImage(crop.Rectangle)
	//
	////transform to Mat object
	//matImg, _ := gocv.ImageToMatRGBA(croppedImg)
	//
	////xmlFile for face detection
	//xmlFile := "/Users/ryan.liew/Documents/pilotCV/haarcascade_frontalface_default.xml"
	//
	//classifier := gocv.NewCascadeClassifier()
	//defer classifier.Close()
	//
	//if !classifier.Load(xmlFile) {
	//	fmt.Printf("Error reading cascade file: %v\n", xmlFile)
	//	return score
	//}
	//
	//rects := classifier.DetectMultiScale(matImg)
	//
	//if len(rects) == 1 {
	//	score.Face = 20
	//}

	if facedetection { for _ , r := range facerects {
		//fmt.Println(i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y)
		//fmt.Println("checking if", r, "is in", crop.Rectangle)
		if r.In(crop.Rectangle) == true && r.Size().X * r.Size().Y >= 30000 {
			//fmt.Println("Yes! The" ,i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y, "is in", crop.Rectangle)
			score.Face = 1.5
			return score

		} else if r.In(crop.Rectangle) == true && r.Size().X * r.Size().Y >= 15000 && r.Size().X * r.Size().Y < 30000 {
			//fmt.Println("Yes! The" ,i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y, "is in", crop.Rectangle)
			score.Face = 1
			return score

		} else if r.In(crop.Rectangle) == true && r.Size().X * r.Size().Y >= 5000 && r.Size().X * r.Size().Y < 15000 {
			//fmt.Println("Yes! The" ,i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y, "is in", crop.Rectangle)
			score.Face = 0.05
			return score

		} else {
			score.Face = 0
			return score
		}
	}

	}
	// code for facial recognition, some room of improvement on tuning the score value


	//// code for facial recognition, some room of improvement on tuning the score value
	//for i , r := range newfacerects {
	//	fmt.Println(i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y)
	//	//fmt.Println("checking if", r, "is in", crop.Rectangle)
	//	if r.In(crop.Rectangle) == true && r.Size().X * r.Size().Y >= 30000 {
	//		//fmt.Println("Yes! The" ,i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y, "is in", crop.Rectangle)
	//		score.Face = 1.5
	//		return score
	//
	//	} else if r.In(crop.Rectangle) == true && r.Size().X * r.Size().Y >= 15000 && r.Size().X * r.Size().Y < 30000 {
	//		//fmt.Println("Yes! The" ,i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y, "is in", crop.Rectangle)
	//		score.Face = 1
	//		return score
	//
	//	} else if r.In(crop.Rectangle) == true && r.Size().X * r.Size().Y >= 5000 && r.Size().X * r.Size().Y < 15000 {
	//		//fmt.Println("Yes! The" ,i,"th face rectangle at", r, "with area", r.Size().X * r.Size().Y, "is in", crop.Rectangle)
	//		score.Face = 0.05
	//		return score
	//
	//	} else {
	//		score.Face = 0
	//		return score
	//	}
	//}

	return score
}

//please note that the image file being feeded in here is a lowimg (the resized version of the image). if you do not desire resampling you will need to modify it at the function that calls this function.
func analyseall(logger Logger, img *image.RGBA, cropWidth, cropHeight, realMinScale float64) ([]Crop, error) {
	//o is *RGBA object (low img)
	o := image.NewRGBA(img.Bounds())

	//these are debugging blocks
	now := time.Now()
	edgeDetect(img, o)
	logger.Log.Println("Time elapsed edge:", time.Since(now))
	debugOutput(logger.DebugMode, o, "edge")

	now = time.Now()
	skinDetect(img, o)
	logger.Log.Println("Time elapsed skin:", time.Since(now))
	debugOutput(logger.DebugMode, o, "skin")

	now = time.Now()
	saturationDetect(img, o)
	logger.Log.Println("Time elapsed sat:", time.Since(now))
	debugOutput(logger.DebugMode, o, "saturation")

	// this bit is calling the function to generate crops
	now = time.Now()
	cs := crops(o, cropWidth, cropHeight, realMinScale) // will potentially need the output of this line to run
	//detect and assign score if found a face
	logger.Log.Println("Time elapsed crops:", time.Since(now), len(cs))

	//evaluate the scores for each candidate crop, and updates the Score field of each "crop" object
	for i:= 0; i<len(cs); i++  {
		cs[i].Score = score(o, cs[i])
	}

	//// this block can potentially be removed
	//now = time.Now()
	//for _, crop := range cs {
	//	nowIn := time.Now()
	//	//o is *RGBA object (low img), crop is the one specific entry in cs array, which has Score and Rect field
	//	crop.Score = score(o, crop)
	//	logger.Log.Println("Time elapsed single-score:", time.Since(nowIn))
	//}

	logger.Log.Println("Time elapsed score:", time.Since(now))

	return cs, nil
}


func analyse(logger Logger, img *image.RGBA, cropWidth, cropHeight, realMinScale float64) (image.Rectangle, error) {
	o := image.NewRGBA(img.Bounds())

	now := time.Now()
	edgeDetect(img, o)
	logger.Log.Println("Time elapsed edge:", time.Since(now))
	debugOutput(logger.DebugMode, o, "edge")

	now = time.Now()
	skinDetect(img, o)
	logger.Log.Println("Time elapsed skin:", time.Since(now))
	debugOutput(logger.DebugMode, o, "skin")

	now = time.Now()
	saturationDetect(img, o)
	logger.Log.Println("Time elapsed sat:", time.Since(now))
	debugOutput(logger.DebugMode, o, "saturation")

	now = time.Now()
	var topCrop Crop
	topScore := -1.0
	cs := crops(o, cropWidth, cropHeight, realMinScale)
	logger.Log.Println("Time elapsed crops:", time.Since(now), len(cs))

	now = time.Now()
	for _, crop := range cs {
		nowIn := time.Now()
		crop.Score = score(o, crop)
		logger.Log.Println("Time elapsed single-score:", time.Since(nowIn))
		if crop.TotalScore() > topScore {
			topCrop = crop
			topScore = crop.TotalScore()
		}
	}
	fmt.Println(topScore)
	logger.Log.Println("Time elapsed score:", time.Since(now))

	if logger.DebugMode {
		drawDebugCrop(topCrop, o)
		debugOutput(true, o, "final")
	}

	return topCrop.Rectangle, nil
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

func edgeDetect(i *image.RGBA, o *image.RGBA) {
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

func skinDetect(i *image.RGBA, o *image.RGBA) {
	width := i.Bounds().Dx()
	height := i.Bounds().Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			lightness := cie(i.RGBAAt(x, y)) / 255.0
			skin := skinCol(i.RGBAAt(x, y))

			c := o.RGBAAt(x, y)
			if skin > skinThreshold && lightness >= skinBrightnessMin && lightness <= skinBrightnessMax {
				r := (skin - skinThreshold) * (255.0 / (1.0 - skinThreshold))
				nc := color.RGBA{uint8(bounds(r)), c.G, c.B, 255}
				o.SetRGBA(x, y, nc)
			} else {
				nc := color.RGBA{0, c.G, c.B, 255}
				o.SetRGBA(x, y, nc)
			}
		}
	}
}

func saturationDetect(i *image.RGBA, o *image.RGBA) {
	width := i.Bounds().Dx()
	height := i.Bounds().Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			lightness := cie(i.RGBAAt(x, y)) / 255.0
			saturation := saturation(i.RGBAAt(x, y))

			c := o.RGBAAt(x, y)
			if saturation > saturationThreshold && lightness >= saturationBrightnessMin && lightness <= saturationBrightnessMax {
				b := (saturation - saturationThreshold) * (255.0 / (1.0 - saturationThreshold))
				nc := color.RGBA{c.R, c.G, uint8(bounds(b)), 255}
				o.SetRGBA(x, y, nc)
			} else {
				nc := color.RGBA{c.R, c.G, 0, 255}
				o.SetRGBA(x, y, nc)
			}
		}
	}
}

func facedetector (smallimg image.Image) []image.Rectangle {
	//fmt.Println("Currently processing", picture)
	img, _ := gocv.ImageToMatRGBA(smallimg)

	//remember to define full path here
	// or you can use os.Args[1] but input the xml file in command line
	xmlFile := "/Users/ryan.liew/Documents/pilotCV/haarcascade_frontalface_default.xml"

	// load classifier to recognize faces, you can find the different classifier files from the opencv library
	classifier := gocv.NewCascadeClassifier()
	defer classifier.Close()

	if !classifier.Load(xmlFile) {
		fmt.Printf("Error reading cascade file: %v\n", xmlFile)
		return []image.Rectangle{}
	}

	rects := classifier.DetectMultiScale(img)

	//fmt.Printf("Program found %d faces\n", len(rects))

	//filter out the rects with too small area (unlikely to be a face)
	//defining the threshold can be tricky and dependent on the pixels of the image, to have a general solution usually we downsample it. Or you can run the first few image to get an average, but you will need to have that function at a higher level.

	for _, r := range rects {
		if r.Size().X * r.Size().Y > 5000 {
			facerects = append(facerects, r)
		}
	}

	//sorting it by area of the rectangle
	sort.Slice(facerects, func(i, j int) bool {
		return (facerects[i].Size().X * facerects[i].Size().Y ) > (facerects[j].Size().X * facerects[j].Size().Y )
	})


	//for i, r := range facerects {
	//	fmt.Println("The", i, "rectangle", r, "has area", r.Size().X * r.Size().Y )
	//}

	////draw on the image to see what the algorithm is actually doing
	//boxColor := color.RGBA{255, 0, 0, 0}
	//boxColor2 := color.RGBA{0, 255, 0, 0}
	//boxColor3 := color.RGBA{0, 0, 255, 0}
	//
	//for i, r := range facerects {
	//	if i == 0 {
	//		gocv.Rectangle(&img, r, boxColor, 4)
	//		size := gocv.GetTextSize("First", gocv.FontItalic, 2, 4)
	//		pt := image.Pt(r.Min.X+10, r.Min.Y+size.Y+50) //coordinate of the caption text
	//		gocv.PutText(&img, "First", pt, gocv.FontItalic, 2, boxColor, 4)
	//	} else if i == 1 {
	//		gocv.Rectangle(&img, r, boxColor2, 4)
	//		size := gocv.GetTextSize("Second", gocv.FontHersheyPlain, 2, 4)
	//		pt := image.Pt(r.Min.X+10, r.Min.Y+size.Y+30)
	//		gocv.PutText(&img, "Second", pt, gocv.FontHersheyPlain, 2, boxColor2, 4)
	//	} else if i == 2 {
	//		gocv.Rectangle(&img, r, boxColor3, 4)
	//		size := gocv.GetTextSize("Third", gocv.FontHersheyPlain, 2, 4)
	//		pt := image.Pt(r.Min.X+10, r.Min.Y+size.Y+10)
	//		gocv.PutText(&img, "Third", pt, gocv.FontHersheyPlain, 2, boxColor3, 4)
	//	}
	//}
	//
	//gocv.IMWrite("facepic.jpeg", img)

	return facerects
}

func crops(i image.Image, cropWidth, cropHeight, realMinScale float64) []Crop {
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

	for scale := maxScale; scale >= realMinScale; scale -= scaleStep {
		for y := 0; float64(y)+cropH*scale <= float64(height); y += step {
			for x := 0; float64(x)+cropW*scale <= float64(width); x += step {
				res = append(res, Crop{
					Rectangle: image.Rect(x, y, x+int(cropW*scale), y+int(cropH*scale)),
				})
			}
		}
		fmt.Println(scale)
	}

	return res
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
