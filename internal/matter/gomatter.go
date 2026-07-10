package matter

import (
	"context"
	"fmt"
	"time"

	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/controller"
	"github.com/sh5080/go-matter/im"
)

// GoMatterDriver drives a Matter window covering natively, over a CASE session
// established by the go-matter controller. It implements Driver and replaces the
// HomeKit-delegated stub once the hub owns the device directly.
type GoMatterDriver struct {
	session  *controller.Session
	endpoint uint16
	timeout  time.Duration
}

// NewGoMatterDriver wraps an established controller session for the given
// window-covering endpoint.
func NewGoMatterDriver(session *controller.Session, endpoint uint16) *GoMatterDriver {
	return &GoMatterDriver{session: session, endpoint: endpoint, timeout: 5 * time.Second}
}

var _ Driver = (*GoMatterDriver)(nil)

func (d *GoMatterDriver) invoke(cmd im.InvokeCommand) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	res, err := d.session.Invoke(ctx, cmd)
	if err != nil {
		return err
	}
	if res.Status != nil && res.Status.Status != im.StatusSuccess {
		return fmt.Errorf("matter: command failed with IM status 0x%02x", res.Status.Status)
	}
	return nil
}

// Open moves the covering toward open.
func (d *GoMatterDriver) Open() error { return d.invoke(cluster.UpOrOpen(d.endpoint)) }

// Close moves the covering toward closed.
func (d *GoMatterDriver) Close() error { return d.invoke(cluster.DownOrClose(d.endpoint)) }

// SetLiftPercent moves the covering to p in the hub's domain orientation
// (HomeKit convention: 0 = fully closed, 100 = fully open).
//
// Matter's lift percent runs the other way (Spec 5.3: 0 = fully open,
// 100 = fully closed), so the driver inverts at this boundary. This keeps every
// caller — applyMatter, the poller, subscriptions — in one consistent
// orientation. If a specific covering turns out to be mounted/calibrated in
// reverse, flip it in device config or here, in ONE place, after validating on
// the hardware.
func (d *GoMatterDriver) SetLiftPercent(p int) error {
	cmd, err := cluster.GoToLiftPercentage(d.endpoint, float64(100-p))
	if err != nil {
		return err
	}
	return d.invoke(cmd)
}

// Shutdown releases the underlying CASE session and its transport.
func (d *GoMatterDriver) Shutdown() error { return d.session.Close() }

// LiftPercent reads the current position in domain orientation (0 = closed,
// 100 = open), inverting Matter's lift percent. A cluster.ErrNull passes
// through untouched: the covering does not know its position right now (e.g.
// mid-motion) and the caller should skip the sample.
func (d *GoMatterDriver) LiftPercent() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	rep, err := d.session.ReadAttribute(ctx, cluster.LiftPositionAttribute(d.endpoint))
	if err != nil {
		return 0, err
	}
	pct, err := cluster.DecodeLiftPercent(rep.Data)
	if err != nil {
		return 0, err
	}
	return 100 - int(pct), nil
}
