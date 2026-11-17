package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const taskNotifyWaterDuration = 1150 * time.Millisecond

type taskWateringAnim struct {
	Label     string
	Started   int64
	Ends      int64
	StartFrac float32
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func easeOutCubic(t float32) float32 {
	t = clamp01(t)
	u := 1 - t
	return 1 - u*u*u
}

func easeInOutCubic(t float32) float32 {
	t = clamp01(t)
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - (u*u*u)/2
}

func lerpF(a, b, t float32) float32 {
	return a + (b-a)*clamp01(t)
}

func lerpColor(a, b ui.Color, t float32) ui.Color {
	return a.Mix(b, clamp01(t))
}

func withAlpha(c ui.Color, a float32) ui.Color {
	return c.Alpha(clamp01(a))
}

func rotatePt(x, y, cx, cy, rad float32) (float32, float32) {
	dx, dy := x-cx, y-cy
	sn, cs := float32(math.Sin(float64(rad))), float32(math.Cos(float64(rad)))
	return cx + dx*cs - dy*sn, cy + dx*sn + dy*cs
}

func addArcPath(path *ui.Path, cx, cy, r, startRad, sweepRad float32) {
	if sweepRad <= 0.001 {
		return
	}
	steps := max(8, int(math.Ceil(float64(sweepRad)/(math.Pi/24))))
	for i := 0; i <= steps; i++ {
		ang := startRad + sweepRad*float32(i)/float32(steps)
		x := cx + r*float32(math.Cos(float64(ang)))
		y := cy + r*float32(math.Sin(float64(ang)))
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
}

// paintEcoWateringBadge renders Option C's 62x62 circular eco-window containing:
// 1. An outer countdown / watering-progress ring.
// 2. A small tree that bounces and sprouts fresh leaves when watered.
// 3. A top-right watering bucket that tilts and pours water droplets on confirmation.
func paintEcoWateringBadge(p *ui.Painter, r ui.Rect, remainFrac float32, waterAnim *taskWateringAnim, now time.Time) {
	scale := min(r.W, r.H) / 64.0
	if scale <= 0 {
		return
	}
	ox := r.X + (r.W-64*scale)/2
	oy := r.Y + (r.H-64*scale)/2
	px := func(x float32) float32 { return ox + x*scale }
	py := func(y float32) float32 { return oy + y*scale }
	ps := func(v float32) float32 { return v * scale }

	remainFrac = clamp01(remainFrac)
	watering := waterAnim != nil
	var w float32
	if watering {
		dur := float32(max(1, waterAnim.Ends-waterAnim.Started))
		w = clamp01(float32(now.UnixNano()-waterAnim.Started) / dur)
	}

	// Request smooth frames while watering or idling with the countdown ring.
	p.AnimationFrame()

	// 1. Eco window background + countdown ring.
	var bgFill ui.Color
	if watering {
		bgFill = lerpColor(ui.Hex("#f4faf5"), ui.Hex("#ebf8ef"), easeOutCubic(w/0.5))
	} else {
		urgency := clamp01(1 - remainFrac/0.35)
		bgFill = lerpColor(ui.Hex("#f4faf5"), ui.Hex("#fbf7ef"), urgency*0.7)
	}
	var bgCircle ui.Path
	bgCircle.Circle(px(32), py(32), ps(28.5))
	p.FillPath(&bgCircle, bgFill)

	var trackCircle ui.Path
	trackCircle.Circle(px(32), py(32), ps(28.5))
	p.StrokePath(&trackCircle, ps(2.8), ui.Hex("#e2ece4"))

	var ringFrac float32
	var ringColor ui.Color
	if watering {
		ringFrac = lerpF(waterAnim.StartFrac, 1.0, easeOutCubic(w/0.45))
		if w < 0.52 {
			ringColor = ui.Hex("#3b9ae1")
		} else {
			ringColor = lerpColor(ui.Hex("#3b9ae1"), ui.Hex("#2f855a"), (w-0.52)/0.28)
		}
	} else {
		ringFrac = remainFrac
		if remainFrac > 0.34 {
			ringColor = ui.Hex("#2e87dc")
		} else {
			ringColor = lerpColor(ui.Hex("#2e87dc"), ui.Hex("#d97706"), (0.34-remainFrac)/0.22)
		}
	}
	if ringFrac > 0.005 {
		var arc ui.Path
		arc.Cap(ui.RoundCap)
		addArcPath(&arc, px(32), py(32), ps(28.5), -math.Pi/2, 2*math.Pi*ringFrac)
		p.StrokePath(&arc, ps(3.0), ringColor)
	}

	// 2. Soil shadow & watering ripple.
	soilColor := ui.Hex("#cfe4d2")
	if watering {
		soilColor = lerpColor(ui.Hex("#cfe4d2"), ui.Hex("#bfe0c4"), easeOutCubic(w))
	}
	p.Fill(ui.Rect{X: px(15.5), Y: py(48.8), W: ps(27), H: ps(5.0)}, soilColor, ps(2.5))

	if watering && w >= 0.25 && w <= 0.92 {
		rp := float32(math.Mod(float64(w-0.25)*2.4, 1.0))
		rw := lerpF(6, 26, easeOutCubic(rp))
		rh := lerpF(1.6, 5.4, easeOutCubic(rp))
		ra := (1 - rp) * 0.75
		p.Stroke(ui.Rect{X: px(29 - rw/2), Y: py(51.2 - rh/2), W: ps(rw), H: ps(rh)}, withAlpha(ui.Hex("#4da3e8"), ra), ps(rh/2), ps(1.1))
	}

	// 3. Tree (trunk, branches, 3-lobe canopy, fresh sprout).
	var sx, sy float32 = 1, 1
	var freshness float32
	if watering {
		freshness = easeOutCubic((w - 0.32) / 0.42)
		switch {
		case w >= 0.32 && w < 0.46:
			t := easeInOutCubic((w - 0.32) / 0.14)
			sx = lerpF(1, 1.07, t)
			sy = lerpF(1, 0.92, t)
		case w >= 0.46 && w < 0.68:
			t := easeOutCubic((w - 0.46) / 0.22)
			sx = lerpF(1.07, 0.96, t)
			sy = lerpF(0.92, 1.08, t)
		case w >= 0.68 && w < 0.90:
			t := easeOutCubic((w - 0.68) / 0.22)
			sx = lerpF(0.96, 1.0, t)
			sy = lerpF(1.08, 1.0, t)
		}
	}
	tx := func(x float32) float32 { return px(29 + (x-29)*sx) }
	ty := func(y float32) float32 { return py(50 + (y-50)*sy) }

	// Trunk + branches
	var trunk ui.Path
	trunk.MoveTo(tx(27.0), ty(50.2)).
		LineTo(tx(28.0), ty(35.0)).
		LineTo(tx(30.2), ty(35.0)).
		LineTo(tx(31.2), ty(50.2)).
		Close()
	p.FillPath(&trunk, ui.Hex("#855e42"))

	var branches ui.Path
	branches.Cap(ui.RoundCap)
	branches.MoveTo(tx(28.8), ty(43.5)).LineTo(tx(24.5), ty(38.5))
	branches.MoveTo(tx(29.5), ty(41.5)).LineTo(tx(33.8), ty(37.2))
	p.StrokePath(&branches, ps(1.6), ui.Hex("#855e42"))

	// Canopy lobes (muted sage when thirsty -> vibrant Tree green when watered)
	cLeft := lerpColor(ui.Hex("#83aa70"), ui.Hex("#58a55c"), freshness)
	cRight := lerpColor(ui.Hex("#739a60"), ui.Hex("#47914c"), freshness)
	cTop := lerpColor(ui.Hex("#93b87e"), ui.Hex("#6dc06d"), freshness)
	cSpec := lerpColor(ui.Hex("#add098"), ui.Hex("#9ce09a"), freshness)

	var leftLobe, rightLobe, topLobe, specLobe ui.Path
	leftLobe.Circle(tx(23.2), ty(34.5), ps(7.3*(sx+sy)*0.5))
	rightLobe.Circle(tx(34.8), ty(34.5), ps(6.8*(sx+sy)*0.5))
	topLobe.Circle(tx(29.0), ty(26.8), ps(8.9*(sx+sy)*0.5))
	specLobe.Circle(tx(26.6), ty(23.8), ps(3.1*(sx+sy)*0.5))
	p.FillPath(&leftLobe, cLeft)
	p.FillPath(&rightLobe, cRight)
	p.FillPath(&topLobe, cTop)
	p.FillPath(&specLobe, withAlpha(cSpec, 0.55))

	// New sprout leaves popping at the top of the tree crown when watered
	if watering && w >= 0.48 {
		sp := easeOutCubic((w - 0.48) / 0.28)
		sproutScale := lerpF(0.1, 1.0, sp)
		if w >= 0.65 && w < 0.82 {
			sproutScale = lerpF(1.16, 1.0, (w-0.65)/0.17)
		}
		bx, by := tx(29.0), ty(18.2)
		var rightLeaf ui.Path
		rightLeaf.MoveTo(bx, by).
			QuadTo(bx+ps(2.2*sproutScale), by-ps(5.8*sproutScale), bx+ps(6.0*sproutScale), by-ps(5.2*sproutScale)).
			QuadTo(bx+ps(4.6*sproutScale), by-ps(1.0*sproutScale), bx, by).
			Close()
		p.FillPath(&rightLeaf, ui.Hex("#38a169"))

		var leftLeaf ui.Path
		leftLeaf.MoveTo(bx, by).
			QuadTo(bx-ps(1.8*sproutScale), by-ps(4.6*sproutScale), bx-ps(4.8*sproutScale), by-ps(3.8*sproutScale)).
			QuadTo(bx-ps(3.5*sproutScale), by-ps(0.6*sproutScale), bx, by).
			Close()
		p.FillPath(&leftLeaf, ui.Hex("#68d391"))
	}

	// 4. Water droplets pouring from tilted bucket onto the tree (0.18 <= w <= 0.74).
	if watering && w >= 0.18 && w <= 0.74 {
		dropStream := (w - 0.18) / 0.56
		for i := range 3 {
			phase := float32(math.Mod(float64(dropStream*2.1)+float64(i)*0.33, 1.0))
			alpha := float32(1.0)
			if phase < 0.15 {
				alpha = phase / 0.15
			} else if phase > 0.82 {
				alpha = (1 - phase) / 0.18
			}
			// Fade whole stream cleanly at start/end of pour window.
			if dropStream < 0.12 {
				alpha *= dropStream / 0.12
			} else if dropStream > 0.85 {
				alpha *= (1 - dropStream) / 0.15
			}
			dx := lerpF(35.0-float32(i)*1.4, 28.5+float32(i)*1.8, phase)
			dy := lerpF(18.5+float32(i)*0.8, 43.0, phase*phase*0.4+phase*0.6)
			dr := lerpF(1.3, 1.85, phase)
			var drop ui.Path
			drop.Circle(px(dx), py(dy), ps(dr))
			p.FillPath(&drop, withAlpha(ui.Hex("#3b9ae1"), alpha*0.95))
		}
	}

	// 5. Watering bucket (top-right inside badge).
	var bx, by float32 = 46.5, 19.0
	var bRotDeg float32 = -6
	var bScale float32 = 1.0
	var bAlpha float32 = 1.0

	if !watering {
		idleSec := float64(now.UnixNano()%100_000_000_000) / 1e9
		by += float32(math.Sin(idleSec*2.7)) * 1.35
	} else {
		switch {
		case w < 0.24:
			t := easeOutCubic(w / 0.24)
			bx = lerpF(46.5, 39.2, t)
			by = lerpF(19.0, 15.2, t)
			bRotDeg = lerpF(-6, -44, t)
		case w < 0.72:
			t := (w - 0.24) / 0.48
			bx = 39.2
			by = 15.2 + float32(math.Sin(float64(t)*math.Pi*3))*0.55
			bRotDeg = -44 + float32(math.Sin(float64(t)*math.Pi*3))*2.5
		default:
			t := easeInOutCubic((w - 0.72) / 0.28)
			bx = lerpF(39.2, 47.2, t)
			by = lerpF(15.2, 20.5, t)
			bRotDeg = lerpF(-44, 0, t)
			bScale = lerpF(1.0, 0.86, t)
			bAlpha = lerpF(1.0, 0.45, t)
		}
	}

	rad := bRotDeg * math.Pi / 180
	bpt := func(lx, ly float32) (float32, float32) {
		sxPt, syPt := bx+lx*bScale, by+ly*bScale
		rx, ry := rotatePt(sxPt, syPt, bx, by, rad)
		return px(rx), py(ry)
	}

	// Wire handle above bucket rim
	var handle ui.Path
	handle.Cap(ui.RoundCap)
	for i := 0; i <= 10; i++ {
		ang := math.Pi + math.Pi*float32(i)/10.0
		hx := float32(math.Cos(float64(ang))) * 4.4
		hy := -4.2 + float32(math.Sin(float64(ang)))*4.4
		txp, typ := bpt(hx, hy)
		if i == 0 {
			handle.MoveTo(txp, typ)
		} else {
			handle.LineTo(txp, typ)
		}
	}
	p.StrokePath(&handle, ps(1.25*bScale), withAlpha(ui.Hex("#64748b"), bAlpha))

	// Bucket body trapezoid
	x1, y1 := bpt(-5.0, -4.2)
	x2, y2 := bpt(5.0, -4.2)
	x3, y3 := bpt(3.7, 5.2)
	x4, y4 := bpt(-3.7, 5.2)
	var body ui.Path
	body.Join(ui.RoundJoin)
	body.MoveTo(x1, y1).LineTo(x2, y2).LineTo(x3, y3).LineTo(x4, y4).Close()
	p.FillPath(&body, withAlpha(ui.Hex("#5aa9e6"), bAlpha))
	p.StrokePath(&body, ps(1.0*bScale), withAlpha(ui.Hex("#2e73b8"), bAlpha))

	// Bucket rim & subtle water highlight band
	rx1, ry1 := bpt(-5.4, -4.2)
	rx2, ry2 := bpt(5.4, -4.2)
	var rim ui.Path
	rim.Cap(ui.RoundCap)
	rim.MoveTo(rx1, ry1).LineTo(rx2, ry2)
	p.StrokePath(&rim, ps(2.0*bScale), withAlpha(ui.Hex("#93c5fd"), bAlpha))

	bx1, by1 := bpt(-4.1, 0.2)
	bx2, by2 := bpt(4.1, 0.2)
	var band ui.Path
	band.Cap(ui.RoundCap)
	band.MoveTo(bx1, by1).LineTo(bx2, by2)
	p.StrokePath(&band, ps(1.1*bScale), withAlpha(ui.Hex("#bfdbfe"), bAlpha*0.85))
}
