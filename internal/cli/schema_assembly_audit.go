// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"errors"
	"fmt"
	"sync/atomic"
)

var ErrSchemaAssemblyConsumedDelivery = errors.New("Schema declaration assembly consumed runtime delivery")

type schemaAssemblyAudit struct {
	violation atomic.Pointer[schemaDeliveryAccess]
}

type schemaDeliveryAccess struct {
	audit     *schemaAssemblyAudit
	operation string
}

var activeSchemaAssemblyAudit atomic.Pointer[schemaAssemblyAudit]

// AuditSchemaAssembly is an exclusive build-time audit, not a runtime lock.
// The identity generator uses it around the entire declaration/projection path.
// Any delivery access, including an already cached lookup, aborts immediately
// before it can enter a loader Once. Do not run alongside runtime consumers.
// Normal production readers do not install an audit and retain concurrency.
func AuditSchemaAssembly(assemble func() error) (err error) {
	if assemble == nil {
		return errors.New("Schema assembly audit requires a callback")
	}
	audit := &schemaAssemblyAudit{}
	if !activeSchemaAssemblyAudit.CompareAndSwap(nil, audit) {
		return errors.New("a Schema assembly audit is already active")
	}
	defer activeSchemaAssemblyAudit.Store(nil)
	defer func() {
		if recovered := recover(); recovered != nil {
			violation, ok := recovered.(*schemaDeliveryAccess)
			if !ok || violation.audit != audit {
				panic(recovered)
			}
		}
		// A callback may recover a panic itself. The recorded violation still
		// invalidates the build instead of minting a supposedly isolated proof.
		if violation := audit.violation.Load(); violation != nil {
			err = fmt.Errorf("%w: %s", ErrSchemaAssemblyConsumedDelivery, violation.operation)
		}
	}()
	return assemble()
}

func auditSchemaDeliveryAccess(operation string) {
	if audit := activeSchemaAssemblyAudit.Load(); audit != nil {
		violation := &schemaDeliveryAccess{audit: audit, operation: operation}
		audit.violation.CompareAndSwap(nil, violation)
		panic(violation)
	}
}
