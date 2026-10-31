package web

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"glucose-dashboard/internal/model"
)

const (
	chartWindow = 12 * time.Hour
	gapLimit    = 5 * time.Minute
	staleAfter  = 10 * time.Minute

	vbW  = 800.0
	vbH  = 360.0
	padL = 56.0
	padR = 16.0
	padT = 16.0
	padB = 36.0

	defaultYMin = 40
	defaultYMax = 400
	yStep       = 50
)

// Path is an SVG path for a continuous run of readings.
type Path struct {
	D string
}

// Dot is an isolated reading.
type Dot struct {
	X string
	Y string
}

// Marker highlights the newest reading in the window.
type Marker struct {
	Show bool
	X    string
	Y    string
}

// GridLine is one axis gridline and its label.
type GridLine struct {
	X1     string
	Y1     string
	X2     string
	Y2     string
	Label  string
	LabelX string
	LabelY string
	Anchor string
}

// Band is the shaded target range.
type Band struct {
	X string
	Y string
	W string
	H string
}

// Frame is the plot rectangle.
type Frame struct {
	X string
	Y string
	W string
	H string
}

// Stats summarizes the readings inside the chart window.
type Stats struct {
	Min     int
	Max     int
	Average float64
	Below   float64
	InRange float64
	Above   float64
}

// Chart is the geometry for one server-rendered glucose chart.
type Chart struct {
	ViewBox   string
	AriaLabel string
	YMin      int
	YMax      int
	HasPoints bool
	Paths     []Path
	Dots      []Dot
	Marker    Marker
	XGrid     []GridLine
	YGrid     []GridLine
	Band      *Band
	Frame     Frame
	Stats     Stats
}

type plotPoint struct {
	ts   time.Time
	mgdl int
	x    float64
	y    float64
}

// BuildChart maps readings onto a 12-hour chart ending at now.
// Readings outside the window are ignored. A gap longer than five minutes
// starts a new path, and a run of one reading is a dot.
func BuildChart(readings []model.Reading, now time.Time, loc *time.Location, targetLow, targetHigh int) Chart {
	if loc == nil {
		loc = time.UTC
	}
	now = now.UTC()
	start := now.Add(-chartWindow)

	points := make([]model.Reading, 0, len(readings))
	for _, reading := range readings {
		ts := reading.TS.UTC()
		if ts.Before(start) || ts.After(now) {
			continue
		}
		points = append(points, reading)
	}
	sort.SliceStable(points, func(i, j int) bool {
		return points[i].TS.Before(points[j].TS)
	})

	chart := Chart{
		ViewBox:   fmt.Sprintf("0 0 %.0f %.0f", vbW, vbH),
		YMin:      defaultYMin,
		YMax:      defaultYMax,
		AriaLabel: "Glucose chart for the last 12 hours. No readings in this window.",
		Frame: Frame{
			X: coord(padL),
			Y: coord(padT),
			W: coord(plotWidth()),
			H: coord(plotHeight()),
		},
	}
	if len(points) == 0 {
		chart.Band = targetBand(chart.YMin, chart.YMax, targetLow, targetHigh)
		chart.XGrid = xGrid(start, now, loc)
		chart.YGrid = yGrid(chart.YMin, chart.YMax)
		return chart
	}

	chart.HasPoints = true
	chart.YMin, chart.YMax = yDomain(points)
	chart.Stats = statsFor(points, targetLow, targetHigh)
	latest := points[len(points)-1]
	chart.AriaLabel = fmt.Sprintf(
		"Glucose chart for the last 12 hours. Latest %d mg/dL. Minimum %d, maximum %d.",
		latest.Mgdl, chart.Stats.Min, chart.Stats.Max,
	)
	chart.Band = targetBand(chart.YMin, chart.YMax, targetLow, targetHigh)
	chart.XGrid = xGrid(start, now, loc)
	chart.YGrid = yGrid(chart.YMin, chart.YMax)

	mapped := make([]plotPoint, len(points))
	for i, reading := range points {
		mapped[i] = plotPoint{
			ts:   reading.TS.UTC(),
			mgdl: reading.Mgdl,
			x:    xOf(start, reading.TS.UTC()),
			y:    yOf(chart.YMin, chart.YMax, reading.Mgdl),
		}
	}

	var run []plotPoint
	flush := func() {
		if len(run) == 0 {
			return
		}
		if len(run) == 1 {
			chart.Dots = append(chart.Dots, Dot{X: coord(run[0].x), Y: coord(run[0].y)})
		} else {
			chart.Paths = append(chart.Paths, Path{D: pathData(run)})
		}
		run = nil
	}
	for _, point := range mapped {
		if len(run) > 0 && point.ts.Sub(run[len(run)-1].ts) > gapLimit {
			flush()
		}
		run = append(run, point)
	}
	flush()

	last := mapped[len(mapped)-1]
	chart.Marker = Marker{Show: true, X: coord(last.x), Y: coord(last.y)}
	return chart
}

func yDomain(points []model.Reading) (int, int) {
	yMin, yMax := defaultYMin, defaultYMax
	for _, point := range points {
		if point.Mgdl < yMin {
			yMin = point.Mgdl
		}
		if point.Mgdl > yMax {
			yMax = point.Mgdl
		}
	}
	if yMin < defaultYMin {
		yMin = (yMin / yStep) * yStep
	}
	if yMax > defaultYMax && yMax%yStep != 0 {
		yMax = ((yMax / yStep) + 1) * yStep
	}
	return yMin, yMax
}

func statsFor(points []model.Reading, targetLow, targetHigh int) Stats {
	stats := Stats{Min: points[0].Mgdl, Max: points[0].Mgdl}
	sum := 0
	below, inRange, above := 0, 0, 0
	for _, point := range points {
		sum += point.Mgdl
		if point.Mgdl < stats.Min {
			stats.Min = point.Mgdl
		}
		if point.Mgdl > stats.Max {
			stats.Max = point.Mgdl
		}
		switch {
		case point.Mgdl < targetLow:
			below++
		case point.Mgdl > targetHigh:
			above++
		default:
			inRange++
		}
	}
	n := float64(len(points))
	stats.Average = float64(sum) / n
	stats.Below = 100 * float64(below) / n
	stats.InRange = 100 * float64(inRange) / n
	stats.Above = 100 * float64(above) / n
	return stats
}

func xGrid(start, now time.Time, loc *time.Location) []GridLine {
	localStart := start.In(loc)
	tick := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), localStart.Hour(), 0, 0, 0, loc)
	if tick.Before(start) {
		tick = tick.Add(time.Hour)
	}
	top := padT
	bottom := padT + plotHeight()
	var lines []GridLine
	for !tick.After(now) {
		x := xOf(start, tick.UTC())
		lines = append(lines, GridLine{
			X1:     coord(x),
			Y1:     coord(top),
			X2:     coord(x),
			Y2:     coord(bottom),
			Label:  tick.In(loc).Format("15:04"),
			LabelX: coord(x),
			LabelY: coord(vbH - 12),
			Anchor: "middle",
		})
		next := tick.Add(time.Hour)
		if !next.After(tick) {
			break
		}
		tick = next
	}
	return lines
}

func yGrid(yMin, yMax int) []GridLine {
	left := padL
	right := padL + plotWidth()
	var lines []GridLine
	for value := ceilStep(yMin, yStep); value <= yMax; value += yStep {
		y := yOf(yMin, yMax, value)
		lines = append(lines, GridLine{
			X1:     coord(left),
			Y1:     coord(y),
			X2:     coord(right),
			Y2:     coord(y),
			Label:  strconv.Itoa(value),
			LabelX: coord(left - 8),
			LabelY: coord(y + 4),
			Anchor: "end",
		})
	}
	return lines
}

func targetBand(yMin, yMax, targetLow, targetHigh int) *Band {
	low, high := targetLow, targetHigh
	if low < yMin {
		low = yMin
	}
	if high > yMax {
		high = yMax
	}
	if high <= low {
		return nil
	}
	top := yOf(yMin, yMax, high)
	bottom := yOf(yMin, yMax, low)
	return &Band{
		X: coord(padL),
		Y: coord(top),
		W: coord(plotWidth()),
		H: coord(bottom - top),
	}
}

func xOf(start, ts time.Time) float64 {
	return padL + ts.Sub(start).Seconds()/chartWindow.Seconds()*plotWidth()
}

func yOf(yMin, yMax, mgdl int) float64 {
	span := float64(yMax - yMin)
	return padT + (float64(yMax-mgdl)/span)*plotHeight()
}

func plotWidth() float64  { return vbW - padL - padR }
func plotHeight() float64 { return vbH - padT - padB }

func ceilStep(value, step int) int {
	if value%step == 0 {
		return value
	}
	return ((value / step) + 1) * step
}

func pathData(points []plotPoint) string {
	var b strings.Builder
	for i, point := range points {
		if i == 0 {
			fmt.Fprintf(&b, "M %s %s", coord(point.x), coord(point.y))
			continue
		}
		fmt.Fprintf(&b, " L %s %s", coord(point.x), coord(point.y))
	}
	return b.String()
}

func coord(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
