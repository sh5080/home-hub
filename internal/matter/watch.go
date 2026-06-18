package matter

import (
	"context"
	"log/slog"

	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/home-hub/internal/domain"
)

// ReportListener streams attribute reports to a callback until ctx ends. It is
// satisfied by controller.Subscription.Listen.
type ReportListener func(ctx context.Context, onReport func([]im.AttributeReport)) error

// PublishReports turns a window-covering subscription into bus state events: it
// emits the priming values immediately, then streams each subsequent report
// until ctx is cancelled or the subscription ends. It blocks; run it in a
// goroutine. Push updates replace polling for devices that support subscribe.
func PublishReports(ctx context.Context, initial []im.AttributeReport, listen ReportListener, deviceID string, publish func(domain.Event), log *slog.Logger) error {
	emit := func(reports []im.AttributeReport) {
		for _, r := range reports {
			if r.Status != nil {
				continue // status-only entry carries no value
			}
			pct, err := cluster.DecodeLiftPercent(r.Data)
			if err != nil {
				if log != nil {
					log.Error("matter subscription decode", "device", deviceID, "err", err)
				}
				continue
			}
			// NOTE: direct lift->Position mapping, consistent with the poller and
			// GoMatterDriver; revisit orientation once validated on hardware.
			publish(domain.Event{
				DeviceID: deviceID,
				Kind:     domain.EventStateChanged,
				State:    domain.State{Position: domain.IntPtr(int(pct))},
			})
		}
	}
	emit(initial)
	return listen(ctx, emit)
}
