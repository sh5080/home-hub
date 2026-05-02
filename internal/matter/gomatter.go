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

// SetLiftPercent moves the covering to p (0..100).
//
// NOTE: HomeKit and Matter disagree on orientation — HomeKit TargetPosition uses
// 0=closed/100=open, while Matter's lift percent uses 0=open/100=closed. If the
// caller passes a HomeKit-oriented value, invert here (100-p). Kept as a direct
// mapping until validated against the real blind.
func (d *GoMatterDriver) SetLiftPercent(p int) error {
	cmd, err := cluster.GoToLiftPercentage(d.endpoint, float64(p))
	if err != nil {
		return err
	}
	return d.invoke(cmd)
}

// LiftPercent reads the current lift position (0..100).
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
	return int(pct), nil
}
