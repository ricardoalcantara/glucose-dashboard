package web

import (
	"strings"
	"testing"
	"time"

	"glucose-dashboard/internal/model"
)

func TestChartConnectsAllReadings(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	start := now.Add(-2 * time.Hour)
	chart := BuildChart([]model.Reading{
		{TS: start, Mgdl: 100},
		{TS: start.Add(time.Minute), Mgdl: 105},
		{TS: start.Add(2 * time.Hour), Mgdl: 130},
	}, now, time.UTC, 70, 180)
	if len(chart.Paths) != 1 || len(chart.Dots) != 0 || !chart.Marker.Show {
		t.Fatalf("paths=%d dots=%d marker=%v", len(chart.Paths), len(chart.Dots), chart.Marker.Show)
	}
	axisStart, axisEnd := now.Add(-chartWindow), now.Add(forecastHorizon)
	for _, sample := range []struct {
		ts   time.Time
		mgdl int
	}{
		{start, 100},
		{start.Add(time.Minute), 105},
		{start.Add(2 * time.Hour), 130},
	} {
		needle := coord(xOf(axisStart, axisEnd, sample.ts)) + " " + coord(yOf(chart.YMin, chart.YMax, sample.mgdl))
		if !strings.Contains(chart.Paths[0].D, needle) {
			t.Fatalf("line misses %s", needle)
		}
	}
}

func TestChartYDomain(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	inside := BuildChart([]model.Reading{
		{TS: now.Add(-time.Hour), Mgdl: 100},
		{TS: now.Add(-30 * time.Minute), Mgdl: 150},
	}, now, time.UTC, 70, 180)
	if inside.YMin != 40 || inside.YMax != 400 {
		t.Fatalf("inside domain = %d..%d", inside.YMin, inside.YMax)
	}

	low := BuildChart([]model.Reading{{TS: now.Add(-time.Minute), Mgdl: 30}}, now, time.UTC, 70, 180)
	if low.YMin != 0 || low.YMax != 400 {
		t.Fatalf("low domain = %d..%d", low.YMin, low.YMax)
	}

	high := BuildChart([]model.Reading{{TS: now.Add(-time.Minute), Mgdl: 450}}, now, time.UTC, 70, 180)
	if high.YMin != 40 || high.YMax != 450 {
		t.Fatalf("high domain = %d..%d", high.YMin, high.YMax)
	}

	snapped := BuildChart([]model.Reading{{TS: now.Add(-time.Minute), Mgdl: 401}}, now, time.UTC, 70, 180)
	if snapped.YMax != 450 {
		t.Fatalf("snapped max = %d", snapped.YMax)
	}
}

func TestChartEmptyAndSinglePoint(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	empty := BuildChart(nil, now, time.UTC, 70, 180)
	if empty.HasPoints || len(empty.Paths) != 0 || len(empty.Dots) != 0 || empty.Marker.Show {
		t.Fatalf("empty = %+v", empty)
	}
	if empty.YMin != 40 || empty.YMax != 400 {
		t.Fatalf("empty domain = %d..%d", empty.YMin, empty.YMax)
	}
	if !strings.Contains(empty.AriaLabel, "No readings") {
		t.Fatalf("aria = %q", empty.AriaLabel)
	}

	outside := BuildChart([]model.Reading{{TS: now.Add(-chartWindow - time.Minute), Mgdl: 100}}, now, time.UTC, 70, 180)
	if outside.HasPoints {
		t.Fatal("point outside the window was kept")
	}

	one := BuildChart([]model.Reading{{TS: now.Add(-2 * time.Minute), Mgdl: 112}}, now, time.UTC, 70, 180)
	if len(one.Paths) != 0 || len(one.Dots) != 1 || !one.Marker.Show {
		t.Fatalf("single paths=%d dots=%d marker=%v", len(one.Paths), len(one.Dots), one.Marker.Show)
	}
	if one.Dots[0].X != one.Marker.X || one.Dots[0].Y != one.Marker.Y {
		t.Fatalf("marker and dot differ: %+v %+v", one.Dots[0], one.Marker)
	}
}

func TestChartStatsAndLocalHourLabels(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-2 * time.Hour), Mgdl: 60},
		{TS: now.Add(-time.Hour), Mgdl: 200},
	}, now, loc, 70, 180)
	if chart.Stats.Min != 60 || chart.Stats.Max != 200 || chart.Stats.Average != 130 {
		t.Fatalf("stats = %+v", chart.Stats)
	}
	if chart.Stats.Below != 50 || chart.Stats.InRange != 0 || chart.Stats.Above != 50 {
		t.Fatalf("percents = %+v", chart.Stats)
	}

	labels := make([]string, 0, len(chart.XGrid))
	for _, line := range chart.XGrid {
		labels = append(labels, line.Label)
		if len(line.Label) != 5 || line.Label[2] != ':' {
			t.Fatalf("label %q", line.Label)
		}
	}
	joined := strings.Join(labels, ",")
	if !strings.Contains(joined, "00:00") || !strings.Contains(joined, "12:00") {
		t.Fatalf("labels = %s", joined)
	}
	if chart.Band == nil {
		t.Fatal("expected a target band")
	}
	edge := now.Add(-chartWindow)
	kept := BuildChart([]model.Reading{{TS: edge, Mgdl: 99}}, now, time.UTC, 70, 180)
	if !kept.HasPoints {
		t.Fatal("reading at the start of the window was dropped")
	}
	if !strings.Contains(joined, "13:00") {
		t.Fatalf("future hour missing from labels = %s", joined)
	}
}

func TestChartShowsOneHourAheadWithoutCopyingHistory(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	recent := now.Add(-30 * time.Minute)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-90 * time.Minute), Mgdl: 80},
		{TS: recent, Mgdl: 100},
		{TS: now, Mgdl: 110},
	}, now, time.UTC, 70, 180)

	if chart.Forecast.Show || len(chart.Forecast.Paths) != 0 {
		t.Fatalf("history was copied ahead: %+v", chart.Forecast)
	}
	start, end := now.Add(-chartWindow), now.Add(forecastHorizon)
	needle := coord(xOf(start, end, recent.Add(time.Hour))) + " " + coord(yOf(chart.YMin, chart.YMax, 100))
	for _, path := range chart.Paths {
		if strings.Contains(path.D, needle) {
			t.Fatal("a past reading was drawn one hour later")
		}
	}
	if chart.Now.X1 != coord(xOf(start, end, now)) || chart.Now.X1 != chart.Now.X2 {
		t.Fatalf("now line = %+v", chart.Now)
	}
	labels := make([]string, 0, len(chart.XGrid))
	for _, line := range chart.XGrid {
		labels = append(labels, line.Label)
	}
	if !strings.Contains(strings.Join(labels, ","), "16:00") {
		t.Fatalf("axis does not reach the next hour: %s", strings.Join(labels, ","))
	}
}

func TestSlopeForecastProjectsOneHour(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-2 * time.Hour), Mgdl: 90},
		{TS: now.Add(-15 * time.Minute), Mgdl: 100},
		{TS: now, Mgdl: 115},
	}, now, time.UTC, 70, 180)

	if !chart.Forecast.Show || chart.Forecast.HitsLow || chart.Forecast.HitsHigh || chart.Forecast.Mgdl != 175 || chart.Forecast.At != "16:00" {
		t.Fatalf("forecast = %+v", chart.Forecast)
	}
	if len(chart.Forecast.Paths) != 1 {
		t.Fatalf("forecast paths = %d", len(chart.Forecast.Paths))
	}
	start, end := now.Add(-chartWindow), now.Add(forecastHorizon)
	if !forecastHas(chart.Forecast, start, end, chart.YMin, chart.YMax, now.Add(time.Hour), 175) {
		t.Fatal("slope endpoint was not drawn one hour after the latest reading")
	}
	if chart.Stats.Min != 90 || chart.Stats.Max != 115 || chart.Stats.Average != 101.66666666666667 {
		t.Fatalf("stats included the forecast: %+v", chart.Stats)
	}
	if chart.YMax != 400 {
		t.Fatalf("in-range forecast changed the domain: %d", chart.YMax)
	}
}

func TestSlopeForecastRoundsHalfUp(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-8 * time.Minute), Mgdl: 100},
		{TS: now, Mgdl: 101},
	}, now, time.UTC, 70, 180)
	if !chart.Forecast.Show || chart.Forecast.Mgdl != 109 {
		t.Fatalf("half-up forecast = %+v", chart.Forecast)
	}
	if chart.Stats.Max != 101 {
		t.Fatalf("stats max = %d", chart.Stats.Max)
	}
}

func TestSlopeForecastNeedsTwoRecentPoints(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-2 * time.Minute), Mgdl: 112},
	}, now, time.UTC, 70, 180)
	if chart.Forecast.Show || len(chart.Forecast.Paths) != 0 {
		t.Fatalf("single point set a forecast: %+v", chart.Forecast)
	}
	if !strings.Contains(chart.AriaLabel, "Forecast plus 1 hour unavailable") {
		t.Fatalf("aria = %q", chart.AriaLabel)
	}
}

func TestForecastExtendsYDomain(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-15 * time.Minute), Mgdl: 350},
		{TS: now, Mgdl: 400},
	}, now, time.UTC, 70, 180)
	if chart.Forecast.Mgdl != 600 || chart.Forecast.HitsHigh || chart.Forecast.HitsLow {
		t.Fatalf("forecast = %+v", chart.Forecast)
	}
	if chart.YMax != 600 {
		t.Fatalf("domain max = %d", chart.YMax)
	}
	if chart.Stats.Max != 400 {
		t.Fatalf("stats max = %d", chart.Stats.Max)
	}
}

func TestSlopeForecastStopsAtLowTarget(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-15 * time.Minute), Mgdl: 130},
		{TS: now, Mgdl: 100},
	}, now, time.UTC, 70, 180)

	if !chart.Forecast.Show || !chart.Forecast.HitsLow || chart.Forecast.Low != 70 || chart.Forecast.Mgdl != 70 || chart.Forecast.At != "15:15" {
		t.Fatalf("forecast = %+v", chart.Forecast)
	}
	if chart.Forecast.HitsHigh {
		t.Fatal("drop from 100 was treated as passing 180")
	}
	if chart.YMin != 40 || chart.YMax != 400 {
		t.Fatalf("domain = %d..%d", chart.YMin, chart.YMax)
	}
	start, end := now.Add(-chartWindow), now.Add(forecastHorizon)
	if !forecastHas(chart.Forecast, start, end, chart.YMin, chart.YMax, now.Add(15*time.Minute), 70) {
		t.Fatal("forecast line did not end at 70")
	}
	if strings.Contains(chart.AriaLabel, "Forecast plus 1 hour") || !strings.Contains(chart.AriaLabel, "Hits 70 mg/dL at 15:15.") {
		t.Fatalf("aria = %q", chart.AriaLabel)
	}
}

func TestSlopeForecastFloorsNegativeAtZero(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-15 * time.Minute), Mgdl: 55},
		{TS: now, Mgdl: 40},
	}, now, time.UTC, 70, 180)

	if !chart.Forecast.Show || chart.Forecast.HitsLow || chart.Forecast.Mgdl != 0 || chart.Forecast.At != "15:40" {
		t.Fatalf("forecast = %+v", chart.Forecast)
	}
	if chart.YMin != 0 {
		t.Fatalf("domain min = %d", chart.YMin)
	}
	start, end := now.Add(-chartWindow), now.Add(forecastHorizon)
	if !forecastHas(chart.Forecast, start, end, chart.YMin, chart.YMax, now.Add(40*time.Minute), 0) {
		t.Fatal("forecast line did not end at 0")
	}
	if strings.Contains(chart.AriaLabel, "-") || !strings.Contains(chart.AriaLabel, "Forecast plus 1 hour 0 mg/dL.") {
		t.Fatalf("aria = %q", chart.AriaLabel)
	}
}

func TestSlopeForecastPassesHighTarget(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-15 * time.Minute), Mgdl: 100},
		{TS: now, Mgdl: 160},
	}, now, time.UTC, 70, 180)

	if !chart.Forecast.Show || chart.Forecast.HitsLow || chart.Forecast.Mgdl != 400 || chart.Forecast.At != "16:00" {
		t.Fatalf("forecast = %+v", chart.Forecast)
	}
	if !chart.Forecast.HitsHigh || chart.Forecast.High != 180 || chart.Forecast.HighAt != "15:05" {
		t.Fatalf("high crossing = %+v", chart.Forecast)
	}
	start, end := now.Add(-chartWindow), now.Add(forecastHorizon)
	if !forecastHas(chart.Forecast, start, end, chart.YMin, chart.YMax, now.Add(time.Hour), 400) {
		t.Fatal("forecast line did not run to the one-hour endpoint")
	}
	if !strings.Contains(chart.AriaLabel, "Forecast plus 1 hour 400 mg/dL.") || !strings.Contains(chart.AriaLabel, "Passes 180 mg/dL at 15:05.") {
		t.Fatalf("aria = %q", chart.AriaLabel)
	}
}

func TestSlopeForecastDropCrossesBothTargets(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	chart := BuildChart([]model.Reading{
		{TS: now.Add(-12 * time.Minute), Mgdl: 250},
		{TS: now, Mgdl: 190},
	}, now, time.UTC, 70, 180)

	if !chart.Forecast.HitsLow || chart.Forecast.At != "15:24" || chart.Forecast.Mgdl != 70 {
		t.Fatalf("forecast = %+v", chart.Forecast)
	}
	if !chart.Forecast.HitsHigh || chart.Forecast.HighAt != "15:02" {
		t.Fatalf("high crossing = %+v", chart.Forecast)
	}
	if chart.YMin != 40 {
		t.Fatalf("domain min = %d", chart.YMin)
	}
	start, end := now.Add(-chartWindow), now.Add(forecastHorizon)
	if !forecastHas(chart.Forecast, start, end, chart.YMin, chart.YMax, now.Add(24*time.Minute), 70) {
		t.Fatal("forecast line did not stop at 70")
	}
	if !strings.Contains(chart.AriaLabel, "Hits 70 mg/dL at 15:24.") || !strings.Contains(chart.AriaLabel, "Passes 180 mg/dL at 15:02.") {
		t.Fatalf("aria = %q", chart.AriaLabel)
	}
}

func forecastHas(forecast Forecast, start, end time.Time, yMin, yMax int, ts time.Time, mgdl int) bool {
	needle := coord(xOf(start, end, ts)) + " " + coord(yOf(yMin, yMax, mgdl))
	for _, path := range forecast.Paths {
		if strings.Contains(path.D, needle) {
			return true
		}
	}
	return false
}
