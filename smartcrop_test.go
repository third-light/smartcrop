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

package smartcrop

import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/third-light/smartcrop/nfnt"
)

var (
	testFile             = "./examples/gopher_test.jpg"
	faceTestFile         = "./examples/face_test.jpg"
	faceDetectClassifier = "./resources/haarcascade_frontalface_default.xml"
)

// Moved here and unexported to decouple the resizer implementation.
func smartCrop(img image.Image, width, height int) (image.Rectangle, error) {
	analyzer := NewAnalyzer(DefaultConfig, nfnt.NewDefaultResizer())
	return analyzer.FindBestCrop(img, width, height)
}

func allCrops(img image.Image, width, height int) ([]Crop, error) {
	analyzer := NewAnalyzer(DefaultConfig, nfnt.NewDefaultResizer())
	return analyzer.FindAllCrops(img, width, height)
}

func faces(img image.Image) []image.Rectangle {
	cfg := FaceDetectConfig
	cfg.FaceDetectClassifierFile = faceDetectClassifier
	analyzer := NewAnalyzer(cfg, nfnt.NewDefaultResizer())
	return analyzer.FindFaces(img)
}

type SubImager interface {
	SubImage(r image.Rectangle) image.Image
}

func TestFace(t *testing.T) {
	fi, _ := os.Open(faceTestFile)
	defer fi.Close()

	img, _, err := image.Decode(fi)
	if err != nil {
		t.Fatal(err)
	}

	rects := faces(img)
	sort.Slice(rects, func(i, j int) bool {
		return rects[i].Min.X < rects[j].Min.X
	})
	expected := []image.Rectangle{
		image.Rect(877, 492, 1518, 1133),
		image.Rect(1427, 271, 1937, 781),
		image.Rect(2207, 997, 2233, 1023),
		image.Rect(2234, 1396, 2336, 1498),
	}
	matched := false
	if len(rects) == len(expected) {
		matched = true
		for i, r := range rects {
			if r != expected[i] {
				matched = false
				break
			}
		}
	}
	if !matched {
		t.Fatalf("expected %v, got %v", expected, rects)
	}
}

func TestCrop(t *testing.T) {
	fi, _ := os.Open(testFile)
	defer fi.Close()

	img, _, err := image.Decode(fi)
	if err != nil {
		t.Fatal(err)
	}

	topCrop, err := smartCrop(img, 250, 250)
	if err != nil {
		t.Fatal(err)
	}
	expected := image.Rect(120, 0, 404, 284)
	if topCrop != expected {
		t.Fatalf("expected %v, got %v", expected, topCrop)
	}

	// test top 3 from allCrops
	allCrops, err := allCrops(img, 250, 250)
	if err != nil {
		t.Fatal(err)
	}
	// Sort by score desc
	sort.Slice(allCrops, func(i, j int) bool {
		return allCrops[i].Score.Total > allCrops[j].Score.Total
	})
	expectedTop3 := []image.Rectangle{
		image.Rect(120, 0, 404, 284),
		image.Rect(112, 0, 396, 284),
		image.Rect(128, 8, 383, 263),
	}
	for i, gotCrop := range allCrops[:3] {
		if gotCrop.Rectangle != expectedTop3[i] {
			t.Fatalf("failed on allCrops in pos %d: expected %v, got %v", i, expectedTop3[i], gotCrop.Rectangle)
		}
	}

	sub, ok := img.(SubImager)
	if ok {
		cropImage := sub.SubImage(topCrop)
		writeImage("jpeg", cropImage, "./smartcrop.jpg")
	} else {
		t.Error(errors.New("No SubImage support"))
	}
}

func BenchmarkCrop(b *testing.B) {
	fi, err := os.Open(testFile)
	if err != nil {
		b.Fatal(err)
	}
	defer fi.Close()

	img, _, err := image.Decode(fi)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := smartCrop(img, 250, 250); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkEdge(b *testing.B) {
	logger := Logger{
		DebugMode: false,
		Log:       log.New(ioutil.Discard, "", 0),
	}
	analyzer := smartcropAnalyzer{
		Resizer: nfnt.NewDefaultResizer(),
		logger:  logger,
		config:  DefaultConfig,
	}
	fi, err := os.Open(testFile)
	if err != nil {
		b.Fatal(err)
	}
	defer fi.Close()

	img, _, err := image.Decode(fi)
	if err != nil {
		b.Fatal(err)
	}

	rgbaImg := toRGBA(img)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := image.NewRGBA(img.Bounds())
		analyzer.edgeDetect(rgbaImg, o)
	}
}

func BenchmarkImageDir(b *testing.B) {
	files, err := ioutil.ReadDir("./examples")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for _, file := range files {
		if strings.Contains(file.Name(), ".jpg") {
			fi, _ := os.Open("./examples/" + file.Name())
			defer fi.Close()

			img, _, err := image.Decode(fi)
			if err != nil {
				b.Error(err)
				continue
			}

			topCrop, err := smartCrop(img, 220, 220)
			if err != nil {
				b.Error(err)
				continue
			}
			fmt.Printf("Top crop: %+v\n", topCrop)

			sub, ok := img.(SubImager)
			if ok {
				cropImage := sub.SubImage(topCrop)
				// cropImage := sub.SubImage(image.Rect(topCrop.X, topCrop.Y, topCrop.Width+topCrop.X, topCrop.Height+topCrop.Y))
				writeImage("jpeg", cropImage, "/tmp/smartcrop/smartcrop-"+file.Name())
			} else {
				b.Error(errors.New("No SubImage support"))
			}
		}
	}
	// fmt.Println("average time/image:", b.t)
}
