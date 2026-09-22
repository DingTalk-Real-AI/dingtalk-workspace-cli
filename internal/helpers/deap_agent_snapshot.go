// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import "time"

var employeeRenameBusy = platformEmployeeRenameBusy

// Windows snapshot readers can briefly deny replacement. Retry only this
// contention, for at most 100ms, while preserving the same completed temp file.
// Never remove the destination: readers must see either the old or new JSON.
func renameEmployeeSnapshot(source, destination string) error {
	for attempt := 0; ; attempt++ {
		err := atomicRename(source, destination)
		if err == nil || !employeeRenameBusy(err) || attempt == 10 {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
