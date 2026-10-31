package web

import (
	"strings"
	"testing"
	"time"

	"glucose-dashboard/internal/model"
)

func TestChartSplitsOnGaps(t *testing.T) {
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	start := now.Add(-2 * time.Hour)

	connected := BuildChart([]model.Reading{
		{TS: start, Mgdl: 100},
		{TS: start.Add(5 * time.Minute), Mgdl: 110},
		{TS: start.Add(9 * time.Minute), Mgdl: 120},
	}, now, time.UTC, 70, 180)
	if len(connected.Paths) != 1 || len(connected.Dots) != 0 || !connected.Marker.Show {
		t.Fatalf("5 minute gap paths=%d dots=%d marker=%v", len(connected.Paths), len(connected.Dots), connected.Marker.Show)
	}

	split := BuildChart([]model.Reading{
		{TS: start, Mgdl: 100},
		{TS: start.Add(time.Minute), Mgdl: 105},
		{TS: start.Add(7 * time.Minute), Mgdl: 130},
	}, now, time.UTC, 70, 180)
	if len(split.Paths) != 1 || len(split.Dots) != 1 {
		t.Fatalf("gap over 5 minutes paths=%d dots=%d", len(split.Paths), len(split.Dots))
	}

	broken := BuildChart([]model.Reading{
		{TS: start, Mgdl: 100},
		{TS: start.Add(5*time.Minute + time.Second), Mgdl: 110},
	}, now, time.UTC, 70, 180)
	if len(broken.Paths) != 0 || len(broken.Dots) != 2 {
		t.Fatalf("just over 5 minutes paths=%d dots=%d", len(broken.Paths), len(broken.Dots))
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
}
