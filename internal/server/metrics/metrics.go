package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SubtaskTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nscan_subtask_total",
		Help: "Total subtasks processed, by stage and final status.",
	}, []string{"stage", "status"})

	SubtaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nscan_subtask_duration_seconds",
		Help:    "Subtask execution duration in seconds.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 10),
	}, []string{"stage"})

	DedupHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nscan_dedup_hits_total",
		Help: "Number of targets deduplicated (already seen) before stage enqueue.",
	}, []string{"stage"})

	DedupNew = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nscan_dedup_new_total",
		Help: "Number of new (unseen) targets passed through to stage enqueue.",
	}, []string{"stage"})

	LeaseExpiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nscan_lease_expired_total",
		Help: "Total number of subtask leases reclaimed by watchdog or node disconnect.",
	})

	DeadLetterTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nscan_dead_letter_total",
		Help: "Total subtasks moved to dead-letter queue, by stage.",
	}, []string{"stage"})
)
