package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// trayIconPNG draws the tray icon at runtime (no binary assets): a rounded pass outline with a keyhole.
// template = true gives the macOS template image (44x44, black + alpha, the system tints it); false gives the
// coloured 32x32 icon used on Windows.
func trayIconPNG(template bool) []byte {
	size := 32
	if template {
		size = 44
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersampling per axis
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var body, hole float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u := (float64(x) + (float64(sx)+0.5)/ss) / float64(size)
					v := (float64(y) + (float64(sy)+0.5)/ss) / float64(size)
					inBody, inHole := iconShapes(u, v, template)
					if inBody {
						body++
					}
					if inHole {
						hole++
					}
				}
			}
			n := float64(ss * ss)
			body, hole = body/n, hole/n
			if template {
				a := math.Max(body, hole)
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(a*255 + 0.5)})
				continue
			}
			// coloured: blue body, white keyhole on top
			if body == 0 && hole == 0 {
				continue
			}
			br, bg, bb := 47.0, 111.0, 235.0
			r, g, b := br, bg, bb
			if hole > 0 {
				r, g, b = br+(255-br)*hole, bg+(255-bg)*hole, bb+(255-bb)*hole
			}
			a := math.Max(body, hole)
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(r + 0.5), G: uint8(g + 0.5), B: uint8(b + 0.5), A: uint8(a*255 + 0.5)})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// iconShapes reports, for a point in the unit square, whether it lies on the pass body (outline for the
// template, filled for the coloured icon) and whether it lies on the keyhole.
func iconShapes(u, v float64, template bool) (body, hole bool) {
	// pass: rounded rectangle, 70% wide, 86% high, centred
	d := roundRectDist(u-0.5, v-0.5, 0.30, 0.40, 0.10)
	if template {
		body = d <= 0 && d >= -0.075 // outline
	} else {
		body = d <= 0
	}
	// keyhole: circle on top of a tapered slot
	cx, cy, r := 0.5, 0.42, 0.085
	if math.Hypot(u-cx, v-cy) <= r {
		hole = true
	}
	if v >= cy && v <= 0.68 {
		t := (v - cy) / (0.68 - cy)
		half := 0.035 + 0.03*t
		if math.Abs(u-cx) <= half {
			hole = true
		}
	}
	return body, hole
}

// roundRectDist is the signed distance to a rounded rectangle centred at the origin (negative inside).
func roundRectDist(px, py, hw, hh, r float64) float64 {
	qx := math.Abs(px) - (hw - r)
	qy := math.Abs(py) - (hh - r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}
