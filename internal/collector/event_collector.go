package collector

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func (c *Collector) runEventCollector(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("event-collector")

	ch := make(chan *store.Event, 256)
	go runBatcher(ctx, ch, c.Config.EventBatchSize, c.Config.EventFlushPeriod, func(batch []*store.Event) {
		for _, e := range batch {
			if err := c.Store.UpsertEvent(ctx, e); err != nil {
				logger.Error(err, "upsert event failed", "uid", e.UID)
			}
		}
	})

	for {
		if err := c.watchEvents(ctx, ch); err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error(err, "watch events error, retrying in 5s")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (c *Collector) watchEvents(ctx context.Context, ch chan<- *store.Event) error {
	logger := log.FromContext(ctx).WithName("event-collector")

	// List existing Warning events first.
	list, err := c.Client.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
	})
	if err != nil {
		return err
	}
	for i := range list.Items {
		if e := kubeEventToStore(&list.Items[i]); e != nil {
			select {
			case ch <- e:
			case <-ctx.Done():
				return nil
			}
		}
	}

	// Watch for new events from the current resource version.
	watcher, err := c.Client.CoreV1().Events("").Watch(ctx, metav1.ListOptions{
		FieldSelector:   "type=Warning",
		ResourceVersion: list.ResourceVersion,
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	logger.Info("watching K8s Warning events", "rv", list.ResourceVersion)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-watcher.ResultChan():
			if !ok {
				return nil // reconnect
			}
			if ev.Type == watch.Error {
				logger.Error(nil, "watch error event received")
				return nil
			}
			if ev.Type == watch.Added || ev.Type == watch.Modified {
				ke, ok := ev.Object.(*corev1.Event)
				if !ok {
					continue
				}
				if e := kubeEventToStore(ke); e != nil {
					select {
					case ch <- e:
					case <-ctx.Done():
						return nil
					}
				}
			}
		}
	}
}

func kubeEventToStore(ke *corev1.Event) *store.Event {
	if ke == nil {
		return nil
	}
	var firstTime, lastTime time.Time
	if !ke.FirstTimestamp.IsZero() {
		firstTime = ke.FirstTimestamp.Time
	} else {
		firstTime = ke.CreationTimestamp.Time
	}
	if !ke.LastTimestamp.IsZero() {
		lastTime = ke.LastTimestamp.Time
	} else {
		lastTime = firstTime
	}
	return &store.Event{
		UID:       string(ke.UID),
		Namespace: ke.Namespace,
		Kind:      ke.InvolvedObject.Kind,
		Name:      ke.InvolvedObject.Name,
		Reason:    ke.Reason,
		Message:   ke.Message,
		Type:      ke.Type,
		Count:     ke.Count,
		FirstTime: firstTime,
		LastTime:  lastTime,
	}
}
