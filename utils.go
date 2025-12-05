package main

import (
	imgcolor "image/color"
	"image/draw"
)

// drawCircle draws a filled circle on the image at (cx, cy) with radius r and color c.
func drawCircle(img draw.Image, cx, cy, r int, c imgcolor.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				img.Set(cx+x, cy+y, c)
			}
		}
	}
}
