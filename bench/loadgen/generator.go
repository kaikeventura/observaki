package loadgen

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ConfigSummary is the serializable part of a run configuration.
type ConfigSummary struct {
	Endpoint        string       `json:"endpoint"`
	Protocol        Protocol     `json:"protocol"`
	Compression     Compression  `json:"compression"`
	Rate            int          `json:"rate"`
	EventTimeRate   int          `json:"event_time_rate"`
	DurationSeconds float64      `json:"duration_seconds"`
	StartTime       time.Time    `json:"start_time"`
	Seed            int64        `json:"seed"`
	BatchSize       int          `json:"batch_size"`
	Workers         int          `json:"workers"`
	Mix             Mix          `json:"mix"`
	Payload         PayloadSizes `json:"payload"`
}

// Summary is the result of a generator run.
type Summary struct {
	Config          ConfigSummary   `json:"config"`
	PlannedEvents   int64           `json:"planned_events"`
	FinishedAt      time.Time       `json:"finished_at"`
	DurationSeconds float64         `json:"duration_seconds"`
	Metrics         MetricsSnapshot `json:"metrics"`
}

// Generator builds and sends the deterministic OTLP workload.
type Generator struct {
	cfg     Config
	builder *Builder
	sender  Sender
	metrics *RunMetrics

	pending [3][]int64
	batchCh chan *Batch
	wg      sync.WaitGroup
}

// NewGenerator validates the configuration and prepares a run.
func NewGenerator(cfg Config, sender Sender, metrics *RunMetrics) (*Generator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if sender == nil {
		return nil, errors.New("sender is required")
	}
	if metrics == nil {
		metrics = NewRunMetrics(cfg.Rate)
	}
	if cfg.StartTime.IsZero() {
		cfg.StartTime = time.Now().UTC()
	}
	builder, err := NewBuilder(cfg)
	if err != nil {
		return nil, err
	}
	return &Generator{
		cfg:     cfg,
		builder: builder,
		sender:  sender,
		metrics: metrics,
		batchCh: make(chan *Batch, cfg.Workers*2),
	}, nil
}

// Run generates the workload until the duration completes or the context is
// canceled. It always waits for in-flight sends before returning.
func (g *Generator) Run(ctx context.Context) (Summary, error) {
	total := g.cfg.TotalEvents()
	if total <= 0 {
		return Summary{}, errors.New("configuration produces zero events")
	}

	started := time.Now().UTC()
	g.metrics.MarkStart(started)
	for i := 0; i < g.cfg.Workers; i++ {
		g.wg.Add(1)
		go g.worker(ctx)
	}

	chunkSize := g.cfg.Rate / 1000
	if chunkSize < 1 {
		chunkSize = 1
	}
	if chunkSize > 100_000 {
		chunkSize = 100_000
	}

	generated := int64(0)
	next := time.Now()
	paceStart := started
	for generated < total {
		if ctx.Err() != nil {
			break
		}
		n := int64(chunkSize)
		if remaining := total - generated; remaining < n {
			n = remaining
		}
		if wait := time.Until(next); wait > 0 {
			time.Sleep(wait)
		}
		produced := g.generateChunk(ctx, generated, int(n))
		generated += int64(produced)
		if produced == 0 {
			break
		}
		next = next.Add(time.Duration(float64(produced) / float64(g.cfg.Rate) * float64(time.Second)))
		if lag := time.Since(next); lag > 100*time.Millisecond {
			due := int64(float64(g.cfg.Rate) * time.Since(paceStart).Seconds())
			if due > generated {
				g.metrics.AddPacingLag(due - generated)
			}
			paceStart = time.Now().Add(-time.Duration(float64(generated) / float64(g.cfg.Rate) * float64(time.Second)))
			next = time.Now()
		}
	}

	if ctx.Err() == nil {
		for _, signal := range AllSignals {
			g.submit(ctx, signal)
		}
	}
	close(g.batchCh)
	g.wg.Wait()

	finished := time.Now().UTC()
	summary := Summary{
		Config:          summarizeConfig(g.cfg),
		PlannedEvents:   total,
		FinishedAt:      finished,
		DurationSeconds: finished.Sub(started).Seconds(),
		Metrics:         g.metrics.Snapshot(),
	}
	return summary, ctx.Err()
}

func (g *Generator) generateChunk(ctx context.Context, start int64, count int) int {
	produced := 0
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			return produced
		}
		index := start + int64(i)
		signal := g.builder.SignalFor(index)
		g.pending[signal] = append(g.pending[signal], index)
		if len(g.pending[signal]) >= g.cfg.BatchSize {
			g.submit(ctx, signal)
		}
		produced++
	}
	return produced
}

func (g *Generator) submit(ctx context.Context, signal Signal) {
	indices := g.pending[signal]
	if len(indices) == 0 {
		return
	}
	batch, err := g.builder.BuildBatch(signal, indices)
	g.pending[signal] = indices[:0]
	if err != nil {
		g.metrics.Record(signal, len(indices), SendResult{}, fmt.Errorf("build %s batch: %w", signal, err), 0)
		return
	}
	g.metrics.AddGenerated(signal, int64(batch.Events))

	select {
	case g.batchCh <- batch:
		return
	case <-ctx.Done():
		return
	default:
	}

	waitStart := time.Now()
	select {
	case g.batchCh <- batch:
		g.metrics.AddBackpressure(int64(batch.Events), time.Since(waitStart))
	case <-ctx.Done():
	}
}

func (g *Generator) worker(ctx context.Context) {
	defer g.wg.Done()
	for batch := range g.batchCh {
		requestCtx, cancel := context.WithTimeout(ctx, g.cfg.RequestTimeout)
		start := time.Now()
		result, err := g.sender.Send(requestCtx, batch)
		cancel()
		g.metrics.Record(batch.Signal, batch.Events, result, err, time.Since(start))
	}
}

func summarizeConfig(cfg Config) ConfigSummary {
	return ConfigSummary{
		Endpoint:        cfg.Endpoint,
		Protocol:        cfg.Protocol,
		Compression:     cfg.Compression,
		Rate:            cfg.Rate,
		EventTimeRate:   cfg.EffectiveEventTimeRate(),
		DurationSeconds: cfg.Duration.Seconds(),
		StartTime:       cfg.StartTime.UTC(),
		Seed:            cfg.Seed,
		BatchSize:       cfg.BatchSize,
		Workers:         cfg.Workers,
		Mix:             cfg.Mix,
		Payload:         cfg.Payload,
	}
}
