// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

//go:build !windows

package localio

import "os"

var replaceFileAtomically = os.Rename
