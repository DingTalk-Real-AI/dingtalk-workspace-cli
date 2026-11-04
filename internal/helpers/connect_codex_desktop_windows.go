// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"net"
)

func dialCodexDesktop(context.Context) (net.Conn, error) {
	return nil, errors.New("desktop IPC sharing is not supported on Windows")
}
