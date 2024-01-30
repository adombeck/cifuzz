package api

import (
	"reflect"
	"testing"
	"time"

	"code-intelligence.com/cifuzz/pkg/report"
)

func Test_createMetricsForCampaignRun(t *testing.T) {
	now := time.Now()
	type args struct {
		firstMetrics *report.FuzzingMetric
		lastMetrics  *report.FuzzingMetric
	}
	fakeMetrics := &report.FuzzingMetric{
		Timestamp:               now,
		ExecutionsPerSecond:     100,
		Features:                100,
		CorpusSize:              100,
		SecondsSinceLastFeature: uint64(100),
		TotalExecutions:         uint64(0),
		Edges:                   100,
		SecondsSinceLastEdge:    uint64(100),
	}
	fakeMetricsInTheFuture := &report.FuzzingMetric{
		Timestamp:               now.Add(time.Second * 10),
		ExecutionsPerSecond:     100,
		Features:                100,
		CorpusSize:              100,
		SecondsSinceLastFeature: uint64(100),
		TotalExecutions:         uint64(100000),
		Edges:                   100,
		SecondsSinceLastEdge:    uint64(100),
	}
	tests := []struct {
		name string
		args args
		want []*Metrics
	}{
		{name: "nil metrics", args: args{firstMetrics: nil, lastMetrics: nil}, want: nil},
		{name: "nil first metrics", args: args{firstMetrics: nil, lastMetrics: fakeMetrics}, want: nil},
		{name: "nil first metrics", args: args{firstMetrics: fakeMetrics, lastMetrics: nil}, want: nil},
		{name: "happy path, should have non-zero performance", args: args{firstMetrics: fakeMetrics, lastMetrics: fakeMetricsInTheFuture}, want: []*Metrics{
			{
				Timestamp:                fakeMetricsInTheFuture.Timestamp.Format(time.RFC3339),
				ExecutionsPerSecond:      int32((time.Second * 10).Milliseconds()),
				Features:                 fakeMetricsInTheFuture.Features,
				CorpusSize:               fakeMetricsInTheFuture.CorpusSize,
				SecondsSinceLastCoverage: "100",
				TotalExecutions:          "100000",
				Edges:                    fakeMetricsInTheFuture.Edges,
				SecondsSinceLastEdge:     "100",
			},
			{
				Timestamp:                fakeMetrics.Timestamp.Format(time.RFC3339),
				ExecutionsPerSecond:      int32((time.Second * 10).Milliseconds()),
				Features:                 fakeMetrics.Features,
				CorpusSize:               fakeMetrics.CorpusSize,
				SecondsSinceLastCoverage: "100",
				TotalExecutions:          "0",
				Edges:                    fakeMetrics.Edges,
				SecondsSinceLastEdge:     "100",
			},
		}},
		// This case previously triggered a division by zero, which is an unspecified behavior, resulting in different results on different platforms
		{name: "0s execution time returns '0' executions per second", args: args{firstMetrics: fakeMetrics, lastMetrics: fakeMetrics}, want: []*Metrics{
			{
				Timestamp:                fakeMetrics.Timestamp.Format(time.RFC3339),
				ExecutionsPerSecond:      0,
				Features:                 fakeMetrics.Features,
				CorpusSize:               fakeMetrics.CorpusSize,
				SecondsSinceLastCoverage: "100",
				TotalExecutions:          "0",
				Edges:                    fakeMetrics.Edges,
				SecondsSinceLastEdge:     "100",
			},
			{
				Timestamp:                fakeMetrics.Timestamp.Format(time.RFC3339),
				ExecutionsPerSecond:      0,
				Features:                 fakeMetrics.Features,
				CorpusSize:               fakeMetrics.CorpusSize,
				SecondsSinceLastCoverage: "100",
				TotalExecutions:          "0",
				Edges:                    fakeMetrics.Edges,
				SecondsSinceLastEdge:     "100",
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := createMetricsForCampaignRun(tt.args.firstMetrics, tt.args.lastMetrics); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createMetricsForCampaignRun() = %+v, want %+v", got[0], tt.want[0])
				t.Errorf("createMetricsForCampaignRun() = %+v, want %+v", got[1], tt.want[1])
			}
		})
	}
}
