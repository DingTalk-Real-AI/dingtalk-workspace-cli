package schemacache

import "sync/atomic"

// IOSnapshot exposes cache-only operations without coupling callers to payload parsing.
type IOSnapshot struct {
	RootOpenOps          uint64
	MkdirOps             uint64
	FileOpenOps          uint64
	StatOps              uint64
	HeaderReadOps        uint64
	MetaPayloadReadOps   uint64
	MetaPayloadReadBytes uint64
	RegistryReadOps      uint64
	RegistryReadBytes    uint64
	WriteOps             uint64
	WriteBytes           uint64
	FileSyncOps          uint64
	CloseOps             uint64
	RenameOps            uint64
	RemoveOps            uint64
	DirectorySyncOps     uint64
	LockAttempts         uint64
}

type Counters struct {
	rootOpenOps          atomic.Uint64
	mkdirOps             atomic.Uint64
	fileOpenOps          atomic.Uint64
	statOps              atomic.Uint64
	headerReadOps        atomic.Uint64
	metaPayloadReadOps   atomic.Uint64
	metaPayloadReadBytes atomic.Uint64
	registryReadOps      atomic.Uint64
	registryReadBytes    atomic.Uint64
	writeOps             atomic.Uint64
	writeBytes           atomic.Uint64
	fileSyncOps          atomic.Uint64
	closeOps             atomic.Uint64
	renameOps            atomic.Uint64
	removeOps            atomic.Uint64
	directorySyncOps     atomic.Uint64
	lockAttempts         atomic.Uint64
}

func (c *Counters) Snapshot() IOSnapshot {
	if c == nil {
		return IOSnapshot{}
	}
	return IOSnapshot{
		RootOpenOps:          c.rootOpenOps.Load(),
		MkdirOps:             c.mkdirOps.Load(),
		FileOpenOps:          c.fileOpenOps.Load(),
		StatOps:              c.statOps.Load(),
		HeaderReadOps:        c.headerReadOps.Load(),
		MetaPayloadReadOps:   c.metaPayloadReadOps.Load(),
		MetaPayloadReadBytes: c.metaPayloadReadBytes.Load(),
		RegistryReadOps:      c.registryReadOps.Load(),
		RegistryReadBytes:    c.registryReadBytes.Load(),
		WriteOps:             c.writeOps.Load(),
		WriteBytes:           c.writeBytes.Load(),
		FileSyncOps:          c.fileSyncOps.Load(),
		CloseOps:             c.closeOps.Load(),
		RenameOps:            c.renameOps.Load(),
		RemoveOps:            c.removeOps.Load(),
		DirectorySyncOps:     c.directorySyncOps.Load(),
		LockAttempts:         c.lockAttempts.Load(),
	}
}
