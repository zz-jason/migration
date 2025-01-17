// Copyright 2021 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tikv/migration/cdc/cdc/model"
	"github.com/tikv/migration/cdc/cdc/redo"
	"github.com/tikv/migration/cdc/cdc/sorter"
	"github.com/tikv/migration/cdc/cdc/sorter/memory"
	"github.com/tikv/migration/cdc/cdc/sorter/unified"
	"github.com/tikv/migration/cdc/pkg/config"
	cdcContext "github.com/tikv/migration/cdc/pkg/context"
	"github.com/tikv/migration/cdc/pkg/pipeline"
)

func TestUnifiedSorterFileLockConflict(t *testing.T) {
	dir := t.TempDir()
	captureAddr := "0.0.0.0:0"

	// GlobalServerConfig overrides dir parameter in NewUnifiedSorter.
	config.GetGlobalServerConfig().Sorter.SortDir = dir
	_, err := unified.NewUnifiedSorter(dir, "test-cf", "test", 0, captureAddr)
	require.Nil(t, err)

	unified.ResetGlobalPoolWithoutCleanup()
	ctx := cdcContext.NewBackendContext4Test(true)
	ctx.ChangefeedVars().Info.Engine = model.SortUnified
	ctx.ChangefeedVars().Info.SortDir = dir
	sorter := sorterNode{}
	err = sorter.Init(pipeline.MockNodeContext4Test(ctx, pipeline.Message{}, nil))
	require.True(t, strings.Contains(err.Error(), "file lock conflict"))
}

func TestSorterResolvedTs(t *testing.T) {
	t.Parallel()
	sn := newSorterNode("tableName", 1, 1, nil, nil, &config.ReplicaConfig{
		Consistent: &config.ConsistentConfig{},
	})
	sn.sorter = memory.NewEntrySorter()
	require.EqualValues(t, 1, sn.ResolvedTs())
	nctx := pipeline.NewNodeContext(
		cdcContext.NewContext(context.Background(), nil),
		pipeline.PolymorphicEventMessage(model.NewResolvedPolymorphicEvent(0, 2)),
		nil,
	)
	err := sn.Receive(nctx)
	require.Nil(t, err)
	require.EqualValues(t, 2, sn.ResolvedTs())
}

type checkSorter struct {
	ch chan *model.PolymorphicEvent
}

var _ sorter.EventSorter = (*checkSorter)(nil)

func (c *checkSorter) Run(ctx context.Context) error {
	return nil
}

func (c *checkSorter) AddEntry(ctx context.Context, entry *model.PolymorphicEvent) {
	c.ch <- entry
}

func (c *checkSorter) TryAddEntry(
	ctx context.Context, entry *model.PolymorphicEvent,
) (bool, error) {
	select {
	case c.ch <- entry:
		return true, nil
	default:
		return false, nil
	}
}

func (c *checkSorter) Output() <-chan *model.PolymorphicEvent {
	return c.ch
}

func TestSorterResolvedTsLessEqualBarrierTs(t *testing.T) {
	t.Parallel()
	sch := make(chan *model.PolymorphicEvent, 1)
	s := &checkSorter{ch: sch}
	sn := newSorterNode("tableName", 1, 1, nil, nil, &config.ReplicaConfig{
		Consistent: &config.ConsistentConfig{},
	})
	sn.sorter = s

	ch := make(chan pipeline.Message, 1)
	require.EqualValues(t, 1, sn.ResolvedTs())

	// Resolved ts must not regress even if there is no barrier ts message.
	resolvedTs1 := pipeline.PolymorphicEventMessage(model.NewResolvedPolymorphicEvent(0, 1))
	nctx := pipeline.NewNodeContext(
		cdcContext.NewContext(context.Background(), nil), resolvedTs1, ch)
	err := sn.Receive(nctx)
	require.Nil(t, err)
	require.EqualValues(t, model.NewResolvedPolymorphicEvent(0, 1), <-sch)

	// Advance barrier ts.
	nctx = pipeline.NewNodeContext(
		cdcContext.NewContext(context.Background(), nil),
		pipeline.BarrierMessage(2),
		ch,
	)
	err = sn.Receive(nctx)
	require.Nil(t, err)
	require.EqualValues(t, 2, sn.barrierTs)
	// Barrier message must be passed to the next node.
	require.EqualValues(t, pipeline.BarrierMessage(2), <-ch)

	resolvedTs2 := pipeline.PolymorphicEventMessage(model.NewResolvedPolymorphicEvent(0, 2))
	nctx = pipeline.NewNodeContext(
		cdcContext.NewContext(context.Background(), nil), resolvedTs2, nil)
	err = sn.Receive(nctx)
	require.Nil(t, err)
	require.EqualValues(t, resolvedTs2.PolymorphicEvent, <-s.Output())

	resolvedTs3 := pipeline.PolymorphicEventMessage(model.NewResolvedPolymorphicEvent(0, 3))
	nctx = pipeline.NewNodeContext(
		cdcContext.NewContext(context.Background(), nil), resolvedTs3, nil)
	err = sn.Receive(nctx)
	require.Nil(t, err)
	require.EqualValues(t, resolvedTs2.PolymorphicEvent, <-s.Output())

	resolvedTs4 := pipeline.PolymorphicEventMessage(model.NewResolvedPolymorphicEvent(0, 4))
	sn.replConfig.Consistent.Level = string(redo.ConsistentLevelEventual)
	nctx = pipeline.NewNodeContext(
		cdcContext.NewContext(context.Background(), nil), resolvedTs4, nil)
	err = sn.Receive(nctx)
	require.Nil(t, err)
	resolvedTs4 = pipeline.PolymorphicEventMessage(model.NewResolvedPolymorphicEvent(0, 4))
	require.EqualValues(t, resolvedTs4.PolymorphicEvent, <-s.Output())
}
