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

package corecmd

import (
	"context"

	"github.com/spf13/cobra"
)

// ExecuteForTest prepares standalone test commands as the app factory does,
// then executes them. Reusing a prepared command preserves Cobra flag state;
// tests of independent invocations must construct fresh commands.
// Production must prepare during assembly and call Cobra directly.
func ExecuteForTest(cmd *cobra.Command) error {
	_, err := ExecuteCForTest(cmd)
	return err
}

// ExecuteCForTest is ExecuteForTest with Cobra's executed-command result.
// Preparation-contract tests should call PrepareCommandTree directly instead.
func ExecuteCForTest(cmd *cobra.Command) (*cobra.Command, error) {
	if cmd == nil {
		return nil, PrepareCommandTree(nil)
	}
	root := cmd.Root()
	if root.Annotations[preparedCommandAnnotation] == "" {
		if err := PrepareCommandTree(root); err != nil {
			return nil, err
		}
	}
	return cmd.ExecuteC()
}

// ExecuteContextForTest prepares and executes with the supplied Cobra context.
func ExecuteContextForTest(cmd *cobra.Command, ctx context.Context) error {
	_, err := ExecuteContextCForTest(cmd, ctx)
	return err
}

// ExecuteContextCForTest preserves ExecuteContextC context assignment semantics.
func ExecuteContextCForTest(cmd *cobra.Command, ctx context.Context) (*cobra.Command, error) {
	cmd.SetContext(ctx)
	return ExecuteCForTest(cmd)
}
