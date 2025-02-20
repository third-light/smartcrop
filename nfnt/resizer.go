/*
 * Copyright (c) 2014-2018 Christian Muehlhaeuser
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

package nfnt

import (
	"image"

	"github.com/third-light/smartcrop/options"
	"github.com/nfnt/resize"
)

type nfntResizer struct {
	interpolationType resize.InterpolationFunction
}

func (r nfntResizer) Resize(img image.Image, width, height uint) image.Image {
	return resize.Resize(width, height, img, r.interpolationType)
}

// NewResizer creates a new Resizer with the given interpolation type.
func NewResizer(interpolationType resize.InterpolationFunction) options.Resizer {
	return nfntResizer{interpolationType: interpolationType}
}

// NewDefaultResizer creates a new Resizer with the default interpolation type.
func NewDefaultResizer() options.Resizer {
	return NewResizer(resize.Bicubic)
}
