package digest

import (
	"context"
	"time"
)

const runHour = 9

func NextRunAt(now time.Time) time.Time {
	now = now.In(time.Local)
	next := time.Date(now.Year(), now.Month(), now.Day(), runHour, 0, 0, 0, time.Local)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func StartScheduler(ctx context.Context, p *Pipeline) {
	go func() {
		for {
			wait := time.Until(NextRunAt(p.now()))
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				if _, err := p.Run(ctx); err != nil {
					p.logf("digest: scheduled run failed: %v", err)
				}
			}
		}
	}()
}
