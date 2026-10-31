package web

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"glucose-dashboard/internal/model"
)

const (
	chartWindow     = 12 * time.Hour
	forecastHorizon = time.Hour
	slopeWindow     = 15 * time.Minute
	staleAfter      = 10 * time.Minute

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

// NowLine marks the current time between history and the forecast hour.
type NowLine struct {
	X1 string
	Y1 string
	X2 string
	Y2 string
}

// Forecast is the math-only projection for the hour after the latest reading.
// A fall that reaches the low target inside that hour stops there. A crossing
// of the high target is reported beside the projection.
type Forecast struct {
	Show     bool
	HitsLow  bool
	Low      int
	Mgdl     int
	At       string
	HitsHigh bool
	High     int
	HighAt   string
	Paths    []Path
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
	Now       NowLine
	Forecast  Forecast
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

// BuildChart maps readings onto a 12-hour history plus a one-hour forecast.
// Readings outside the history window are ignored. Every reading in the window
// is joined by one line. A window with a single reading is a dot.
// Stats use measured readings only.
func BuildChart(readings []model.Reading, now time.Time, loc *time.Location, targetLow, targetHigh int) Chart {
	if loc == nil {
		loc = time.UTC
	}
	now = now.UTC()
	start := now.Add(-chartWindow)
	end := now.Add(forecastHorizon)

	points := make([]model.Reading, 0, len(readings))
	for _, reading := range readings {
		ts := reading.TS.UTC()
		if ts.Before(start) || ts.After(now) {
			continue
		}
		points = append(points, model.Reading{TS: ts, Mgdl: reading.Mgdl, Source: reading.Source})
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
		chart.XGrid = xGrid(start, end, loc)
		chart.YGrid = yGrid(chart.YMin, chart.YMax)
		return chart
	}

	latest := points[len(points)-1]
	forecast := projectForecast(points, targetLow, targetHigh, loc)
	domain := append([]model.Reading{}, points...)
	if forecast.Show {
		domain = append(domain, model.Reading{Mgdl: forecast.Mgdl})
	}

	chart.HasPoints = true
	chart.YMin, chart.YMax = yDomain(domain)
	chart.Stats = statsFor(points, targetLow, targetHigh)
	chart.AriaLabel = forecastLabel(latest.Mgdl, chart.Stats, forecast.Forecast)
	chart.Band = targetBand(chart.YMin, chart.YMax, targetLow, targetHigh)
	chart.XGrid = xGrid(start, end, loc)
	chart.YGrid = yGrid(chart.YMin, chart.YMax)
	nowX := xOf(start, end, now)
	chart.Now = NowLine{
		X1: coord(nowX),
		Y1: coord(padT),
		X2: coord(nowX),
		Y2: coord(padT + plotHeight()),
	}

	mapped := mapReadings(points, start, end, chart.YMin, chart.YMax)
	chart.Paths, chart.Dots = seriesGeometry(mapped)
	last := mapped[len(mapped)-1]
	chart.Marker = Marker{Show: true, X: coord(last.x), Y: coord(last.y)}

	if forecast.Show {
		chart.Forecast = forecast.Forecast
		chart.Forecast.Paths = []Path{{D: pathData([]plotPoint{
			last,
			{
				ts:   forecast.endpoint,
				mgdl: forecast.Mgdl,
				x:    xOf(start, end, forecast.endpoint),
				y:    yOf(chart.YMin, chart.YMax, forecast.Mgdl),
			},
		})}}
	}
	return chart
}

func forecastLabel(latest int, stats Stats, forecast Forecast) string {
	label := fmt.Sprintf(
		"Glucose chart for the last 12 hours and the next hour. Latest %d mg/dL. Minimum %d, maximum %d.",
		latest, stats.Min, stats.Max,
	)
	if !forecast.Show {
		return label + " Forecast plus 1 hour unavailable."
	}
	if forecast.HitsLow {
		label += fmt.Sprintf(" Hits %d mg/dL at %s.", forecast.Low, forecast.At)
	} else {
		label += fmt.Sprintf(" Forecast plus 1 hour %d mg/dL.", forecast.Mgdl)
	}
	if forecast.HitsHigh {
		label += fmt.Sprintf(" Passes %d mg/dL at %s.", forecast.High, forecast.HighAt)
	}
	return label
}

// projectedForecast carries the clock instant of the drawn endpoint. That
// instant is not part of the page data.
type projectedForecast struct {
	Forecast
	endpoint time.Time
}

func projectForecast(points []model.Reading, targetLow, targetHigh int, loc *time.Location) projectedForecast {
	latest := points[len(points)-1]
	rate, ok := readingSlope(points)
	if !ok {
		return projectedForecast{}
	}
	raw := float64(latest.Mgdl) + rate*forecastHorizon.Minutes()
	endpointMgdl := roundHalfUp(raw)
	endpointAt := latest.TS.Add(forecastHorizon)
	forecast := Forecast{Show: true, Mgdl: endpointMgdl}

	if latest.Mgdl > targetLow {
		if d, hit := crossesWithin(latest.Mgdl, rate, targetLow); hit {
			forecast.HitsLow = true
			forecast.Low = targetLow
			forecast.Mgdl = targetLow
			endpointMgdl = targetLow
			endpointAt = latest.TS.Add(d)
		}
	}
	if !forecast.HitsLow && endpointMgdl < 0 {
		forecast.Mgdl = 0
		endpointMgdl = 0
		if d, hit := crossesWithin(latest.Mgdl, rate, 0); hit {
			endpointAt = latest.TS.Add(d)
		} else {
			endpointAt = latest.TS
		}
	}
	if d, hit := crossesWithin(latest.Mgdl, rate, targetHigh); hit {
		forecast.HitsHigh = true
		forecast.High = targetHigh
		forecast.HighAt = latest.TS.Add(d).In(loc).Format("15:04")
	}
	forecast.At = endpointAt.In(loc).Format("15:04")
	forecast.Mgdl = endpointMgdl
	return projectedForecast{Forecast: forecast, endpoint: endpointAt}
}

func readingSlope(points []model.Reading) (float64, bool) {
	latest := points[len(points)-1]
	cutoff := latest.TS.Add(-slopeWindow)
	earliest := latest
	count := 0
	for _, point := range points {
		if point.TS.Before(cutoff) {
			continue
		}
		count++
		if point.TS.Before(earliest.TS) {
			earliest = point
		}
	}
	minutes := latest.TS.Sub(earliest.TS).Minutes()
	if count < 2 || minutes <= 0 {
		return 0, false
	}
	return float64(latest.Mgdl-earliest.Mgdl) / minutes, true
}

// crossesWithin reports how long until a constant slope reaches target, when
// that happens after the latest reading and before the one-hour mark.
func crossesWithin(latest int, rate float64, target int) (time.Duration, bool) {
	if rate == 0 {
		return 0, false
	}
	minutes := float64(target-latest) / rate
	if minutes <= 0 || minutes >= forecastHorizon.Minutes() {
		return 0, false
	}
	return time.Duration(math.Round(minutes * float64(time.Minute))), true
}

func roundHalfUp(value float64) int {
	return int(math.Floor(value + 0.5))
}

func mapReadings(points []model.Reading, start, end time.Time, yMin, yMax int) []plotPoint {
	mapped := make([]plotPoint, len(points))
	for i, reading := range points {
		mapped[i] = plotPoint{
			ts:   reading.TS.UTC(),
			mgdl: reading.Mgdl,
			x:    xOf(start, end, reading.TS.UTC()),
			y:    yOf(yMin, yMax, reading.Mgdl),
		}
	}
	return mapped
}

func seriesGeometry(points []plotPoint) ([]Path, []Dot) {
	if len(points) == 0 {
		return nil, nil
	}
	if len(points) == 1 {
		return nil, []Dot{{X: coord(points[0].x), Y: coord(points[0].y)}}
	}
	return []Path{{D: pathData(points)}}, nil
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
		yMin = snapDown(yMin, yStep)
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

func xGrid(start, end time.Time, loc *time.Location) []GridLine {
	localStart := start.In(loc)
	tick := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), localStart.Hour(), 0, 0, 0, loc)
	if tick.Before(start) {
		tick = tick.Add(time.Hour)
	}
	top := padT
	bottom := padT + plotHeight()
	var lines []GridLine
	for !tick.After(end) {
		x := xOf(start, end, tick.UTC())
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

func xOf(start, end, ts time.Time) float64 {
	span := end.Sub(start).Seconds()
	return padL + ts.Sub(start).Seconds()/span*plotWidth()
}

func snapDown(value, step int) int {
	q := value / step
	if value < 0 && value%step != 0 {
		q--
	}
	return q * step
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
