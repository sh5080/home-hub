package matter

import (
	"context"
	"testing"

	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/tlv"
	"github.com/sh5080/home-hub/internal/domain"
)

// liftReport builds an AttributeReport carrying a raw Matter lift position
// (percent, 0=open/100=closed) as a device would report it.
func liftReport(matterPercent int) im.AttributeReport {
	w := tlv.NewWriter()
	w.PutUint(tlv.Anonymous(), uint64(matterPercent*100)) // 100ths
	data, _ := w.Bytes()
	return im.AttributeReport{Path: cluster.LiftPositionAttribute(1), DataVersion: 1, Data: data}
}

// nullLiftReport is a report with a null position (covering in motion).
func nullLiftReport() im.AttributeReport {
	w := tlv.NewWriter()
	w.PutNull(tlv.Anonymous())
	data, _ := w.Bytes()
	return im.AttributeReport{Path: cluster.LiftPositionAttribute(1), DataVersion: 1, Data: data}
}

func TestPublishReports(t *testing.T) {
	initial := []im.AttributeReport{liftReport(37)}
	streamed := [][]im.AttributeReport{{liftReport(50)}, {nullLiftReport()}, {liftReport(25)}}
	listen := func(_ context.Context, on func([]im.AttributeReport)) error {
		for _, r := range streamed {
			on(r)
		}
		return nil
	}

	var events []domain.Event
	err := PublishReports(context.Background(), initial, listen, "blind1", func(e domain.Event) {
		events = append(events, e)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Matter lift is inverted into domain positions (100-x); the null sample is
	// skipped entirely.
	want := []int{63, 50, 75}
	if len(events) != len(want) {
		t.Fatalf("events = %d, want %d", len(events), len(want))
	}
	for i, w := range want {
		e := events[i]
		if e.DeviceID != "blind1" || e.Kind != domain.EventStateChanged || e.State.Position == nil || *e.State.Position != w {
			t.Fatalf("event %d = %+v, want position %d", i, e, w)
		}
	}
}

func TestPublishReportsSkipsStatus(t *testing.T) {
	status := uint8(im.StatusUnsupportedCluster)
	initial := []im.AttributeReport{{Path: cluster.LiftPositionAttribute(1), Status: &status}}
	listen := func(_ context.Context, _ func([]im.AttributeReport)) error { return nil }

	var events []domain.Event
	PublishReports(context.Background(), initial, listen, "blind1", func(e domain.Event) {
		events = append(events, e)
	}, nil)
	if len(events) != 0 {
		t.Fatalf("status report should not publish state: %+v", events)
	}
}
