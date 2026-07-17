// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newDriveTaskCommand() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "异步任务管理",
		RunE:  groupRunE,
	}

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "查询导出或导入任务状态",
		Example: `  dws drive task get --type export --id <jobId>
  dws drive task get --type import --id <taskId>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlags(cmd, "type", "id"); err != nil {
				return err
			}
			taskType := strings.ToLower(strings.TrimSpace(mustGetFlag(cmd, "type")))
			taskID := mustGetFlag(cmd, "id")
			if taskType != "export" && taskType != "import" {
				return fmt.Errorf("unsupported --type %q; expected export or import", taskType)
			}
			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "查询异步任务")
				deps.Out.PrintKeyValue("类型", taskType)
				deps.Out.PrintKeyValue("ID", taskID)
				return nil
			}
			result, err := queryAsyncTask(cmd.Context(), taskType, taskID)
			if err != nil {
				return err
			}
			return deps.Out.PrintJSON(result)
		},
	}
	getCmd.Flags().String("type", "", "任务类型: export 或 import (必填)")
	getCmd.Flags().String("id", "", "异步任务 ID (必填)")
	taskCmd.AddCommand(getCmd)

	return taskCmd
}
