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

package cli

import (
	"github.com/spf13/cobra"
	"github.com/tikv/migration/cdc/pkg/cmd/factory"
)

// newCmdCyclicChangefeed creates the `cli changefeed cyclic` command.
func newCmdCyclicChangefeed(f factory.Factory) *cobra.Command {
	cmds := &cobra.Command{
		Use:   "cyclic",
		Short: "(Experimental) Utility about cyclic replication",
	}

	cmds.AddCommand(newCmdCyclicCreateMarktables(f))

	return cmds
}
