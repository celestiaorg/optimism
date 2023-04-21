package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ethereum-optimism/optimism/op-service/metrics"
)

const Namespace = "op_celestia_indexer"

// Metricer defines the interface for metrics collection
type Metricer interface {
	metrics.RPCMetricer
	RecordInfo(version string)
	RecordUp()

	// Indexer-specific metrics
	RecordIndexedBlock(blockNum uint64)
	RecordLocationStored(l2Start, l2End uint64)
	RecordLocationRequested(l2Block uint64, found bool)
	RecordIndexingError(blockNum uint64)
	Document() []metrics.DocumentedMetric
}

// Metrics implements the Metricer interface
type Metrics struct {
	ns       string
	registry *prometheus.Registry
	factory  metrics.Factory

	info prometheus.GaugeVec
	up   prometheus.Gauge
	metrics.RPCMetrics

	// Indexer metrics
	indexedBlocks    prometheus.Counter
	locationsStored  prometheus.Counter
	l2BlocksIndexed  prometheus.Gauge
	locationRequests prometheus.CounterVec
	indexingErrors   prometheus.CounterVec
}

var _ Metricer = (*Metrics)(nil)

// NewMetrics creates a new Metrics instance
func NewMetrics(procName string) *Metrics {
	if procName == "" {
		procName = "default"
	}
	ns := Namespace + "_" + procName

	registry := prometheus.NewRegistry()
	factory := metrics.With(registry)

	return &Metrics{
		ns:         ns,
		registry:   registry,
		factory:    factory,
		RPCMetrics: metrics.MakeRPCMetrics(ns, factory),

		info: *factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "info",
			Help:      "Pseudo-metric tracking version and config info",
		}, []string{
			"version",
		}),
		up: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "up",
			Help:      "1 if the indexer has finished starting up",
		}),

		indexedBlocks: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "indexed_blocks_total",
			Help:      "Number of L1 blocks indexed",
		}),
		locationsStored: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "locations_stored_total",
			Help:      "Number of Celestia locations stored",
		}),
		l2BlocksIndexed: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "l2_blocks_indexed",
			Help:      "Total number of L2 blocks indexed",
		}),
		locationRequests: *factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "location_requests_total",
			Help:      "Number of location requests",
		}, []string{
			"found",
		}),
		indexingErrors: *factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "indexing_errors_total",
			Help:      "Number of indexing errors",
		}, []string{
			"block",
		}),
	}
}

// Registry returns the metrics registry
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// RecordInfo records build and version info
func (m *Metrics) RecordInfo(version string) {
	m.info.WithLabelValues(version).Set(1)
}

// RecordUp sets the up metric to 1
func (m *Metrics) RecordUp() {
	m.up.Set(1)
}

// RecordIndexedBlock records that a block was indexed
func (m *Metrics) RecordIndexedBlock(blockNum uint64) {
	m.indexedBlocks.Inc()
}

// RecordLocationStored records that a Celestia location was stored
func (m *Metrics) RecordLocationStored(l2Start, l2End uint64) {
	m.locationsStored.Inc()
	m.l2BlocksIndexed.Set(float64(l2End))
}

// RecordLocationRequested records a location request
func (m *Metrics) RecordLocationRequested(l2Block uint64, found bool) {
	foundStr := "false"
	if found {
		foundStr = "true"
	}
	m.locationRequests.WithLabelValues(foundStr).Inc()
}

// RecordIndexingError records an indexing error
func (m *Metrics) RecordIndexingError(blockNum uint64) {
	m.indexingErrors.WithLabelValues(string(rune(blockNum))).Inc()
}

// Document returns the metrics document
func (m *Metrics) Document() []metrics.DocumentedMetric {
	return m.factory.Document()
}

type noopMetrics struct {
	metrics.NoopRPCMetrics
}

var NoopMetrics Metricer = new(noopMetrics)

// RecordInfo records build and version info
func (*noopMetrics) RecordInfo(version string) {}

// RecordUp sets the up metric to 1
func (*noopMetrics) RecordUp() {}

// RecordIndexedBlock records that a block was indexed
func (*noopMetrics) RecordIndexedBlock(blockNum uint64) {}

// RecordLocationStored records that a Celestia location was stored
func (*noopMetrics) RecordLocationStored(l2Start, l2End uint64) {}

// RecordLocationRequested records a location request
func (*noopMetrics) RecordLocationRequested(l2Block uint64, found bool) {}

// RecordIndexingError records an indexing error
func (*noopMetrics) RecordIndexingError(blockNum uint64) {}

// Document returns the metrics document
func (*noopMetrics) Document() []metrics.DocumentedMetric { return nil }
