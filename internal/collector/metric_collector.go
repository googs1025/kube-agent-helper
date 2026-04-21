package collector

import (
	"context"
	"encoding/json"
	"time"

	prometheusapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

const (
	metricScrapePeriod = 15 * time.Minute
	maxSeriesPerQuery  = 500
)

func (c *Collector) runMetricCollector(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("metric-collector")

	client, err := prometheusapi.NewClient(prometheusapi.Config{Address: c.Config.PrometheusURL})
	if err != nil {
		logger.Error(err, "failed to create prometheus client")
		return
	}
	api := prometheusv1.NewAPI(client)

	ticker := time.NewTicker(metricScrapePeriod)
	defer ticker.Stop()

	// Scrape once immediately, then on each tick.
	c.scrapeAll(ctx, api)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.scrapeAll(ctx, api)
		}
	}
}

func (c *Collector) scrapeAll(ctx context.Context, api prometheusv1.API) {
	logger := log.FromContext(ctx).WithName("metric-collector")
	now := time.Now()
	for _, q := range c.Config.MetricsQueries {
		result, warnings, err := api.Query(ctx, q, now)
		if err != nil {
			logger.Error(err, "prometheus query failed", "query", q)
			continue
		}
		if len(warnings) > 0 {
			logger.Info("prometheus warnings", "query", q, "warnings", warnings)
		}
		vector, ok := result.(model.Vector)
		if !ok {
			continue
		}
		if len(vector) > maxSeriesPerQuery {
			vector = vector[:maxSeriesPerQuery]
		}
		for _, sample := range vector {
			labelsJSON, _ := json.Marshal(sample.Metric)
			snap := &store.MetricSnapshot{
				Query:      q,
				LabelsJSON: string(labelsJSON),
				Value:      float64(sample.Value),
				Ts:         sample.Timestamp.Time(),
			}
			if err := c.Store.InsertMetricSnapshot(ctx, snap); err != nil {
				logger.Error(err, "insert metric snapshot failed")
			}
		}
	}
}