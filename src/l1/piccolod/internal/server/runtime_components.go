package server

import (
	"context"
	"log"

	"piccolod/internal/events"
	"piccolod/internal/runtime/supervisor"
)

// newLeadershipObserver registers a supervisor component that logs leadership events.
func newLeadershipObserver(bus *events.Bus) supervisor.Component {
	observer := &leadershipObserver{bus: bus}
	return supervisor.NewComponent("leadership-observer", observer.start, observer.stop)
}

type leadershipObserver struct {
	bus    *events.Bus
	cancel context.CancelFunc
}

func (o *leadershipObserver) start(ctx context.Context) error {
	if o.bus == nil {
		return nil
	}
	ch := o.bus.Subscribe(events.TopicLeadershipRoleChanged, 16)
	runCtx, cancel := context.WithCancel(ctx)
	o.cancel = cancel
	go func() {
		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return
				}
				payload, ok := evt.Payload.(events.LeadershipChanged)
				if !ok {
					log.Printf("WARN: leadership-observer received unexpected payload: %#v", evt.Payload)
					continue
				}
				log.Printf("INFO: leadership change resource=%s role=%s", payload.Resource, payload.Role)
			case <-runCtx.Done():
				return
			}
		}
	}()
	return nil
}

func (o *leadershipObserver) stop(ctx context.Context) error {
	if o.cancel != nil {
		o.cancel()
	}
	return nil
}
