package matter

import (
	"context"
	"errors"
	"log/slog"
	"time"

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
			if errors.Is(err, cluster.ErrNull) {
				continue // position unknown (e.g. mid-motion): no sample
			}
			if err != nil {
				if log != nil {
					log.Error("matter subscription decode", "device", deviceID, "err", err)
				}
				continue
			}
			// Subscription reports carry raw Matter lift (0=open/100=closed);
			// invert to the domain orientation, same as GoMatterDriver does.
			publish(domain.Event{
				DeviceID: deviceID,
				Kind:     domain.EventStateChanged,
				State:    domain.State{Position: domain.IntPtr(100 - int(pct))},
			})
		}
	}
	emit(initial)
	return listen(ctx, emit)
}

// WatchGoMatter maintains a push subscription to a native device for the life
// of ctx: dial → subscribe → stream reports as bus events; when the stream
// breaks (device reboot, network outage) it backs off and re-subscribes, so a
// device that disappears comes back on its own. Blocks; run in a goroutine.
func WatchGoMatter(ctx context.Context, cfg GoMatterConfig, deviceID string, publish func(domain.Event), log *slog.Logger) {
	backoff := 5 * time.Second
	const maxBackoff = 5 * time.Minute
	for ctx.Err() == nil {
		dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		sess, sub, err := SubscribeGoMatter(dialCtx, cfg)
		cancel()
		if err == nil {
			started := time.Now()
			err = PublishReports(ctx, sub.Initial, sub.Listen, deviceID, publish, log)
			sess.Close()
			if time.Since(started) > time.Minute {
				backoff = 5 * time.Second // it was healthy for a while: reset
			}
		}
		if ctx.Err() != nil {
			return
		}
		if log != nil {
			log.Warn("matter subscription down; retrying", "device", deviceID, "err", err, "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
