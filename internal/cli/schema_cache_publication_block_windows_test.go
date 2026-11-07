// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build windows

package cli_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

// Windows FILE_ATTRIBUTE_READONLY on a directory does not prevent creating or
// renaming children. Production snapshot replace also succeeds while readers
// hold FILE_SHARE_WRITE|FILE_SHARE_DELETE handles. A protected DACL that
// denies FILE_ADD_FILE / FILE_DELETE_CHILD is the chmod 0500 equivalent.
func blockSchemaCachePublication(t *testing.T, cacheDirectory string) func() {
	t.Helper()
	if err := applySchemaCacheDirectoryACL(cacheDirectory, false); err != nil {
		t.Fatalf("deny cache directory writes: %v", err)
	}
	probe := filepath.Join(cacheDirectory, ".dws-unwritable-probe")
	if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600); err == nil {
		_ = f.Close()
		_ = os.Remove(probe)
		_ = applySchemaCacheDirectoryACL(cacheDirectory, true)
		t.Fatal("Windows cache directory stayed writable after denying create/rename rights")
	}
	var once sync.Once
	unblock := func() {
		once.Do(func() {
			if err := applySchemaCacheDirectoryACL(cacheDirectory, true); err != nil {
				t.Errorf("restore cache directory ACL: %v", err)
			}
		})
	}
	t.Cleanup(unblock)
	return unblock
}

func applySchemaCacheDirectoryACL(path string, writable bool) error {
	user, err := currentTestUserSID()
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	var access []windows.EXPLICIT_ACCESS
	if writable {
		access = []windows.EXPLICIT_ACCESS{
			explicitTestAccess(user, windows.TRUSTEE_IS_USER, windows.GENERIC_ALL, windows.GRANT_ACCESS),
			explicitTestAccess(system, windows.TRUSTEE_IS_USER, windows.GENERIC_ALL, windows.GRANT_ACCESS),
		}
	} else {
		admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
		if err != nil {
			return err
		}
		// FILE_ADD_FILE / FILE_ADD_SUBDIRECTORY / FILE_DELETE_CHILD are the
		// directory aliases of WRITE_DATA / APPEND_DATA / 0x40. Do not deny
		// SYNCHRONIZE or the probe/Open of existing artifacts also fails.
		const fileDeleteChild windows.ACCESS_MASK = 0x00000040
		denyWrite := windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA |
			windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA | fileDeleteChild
		access = []windows.EXPLICIT_ACCESS{
			explicitTestAccess(user, windows.TRUSTEE_IS_USER, denyWrite, windows.DENY_ACCESS),
			explicitTestAccess(admins, windows.TRUSTEE_IS_GROUP, denyWrite, windows.DENY_ACCESS),
			explicitTestAccess(user, windows.TRUSTEE_IS_USER, windows.GENERIC_READ|windows.GENERIC_EXECUTE, windows.GRANT_ACCESS),
			explicitTestAccess(system, windows.TRUSTEE_IS_USER, windows.GENERIC_ALL, windows.GRANT_ACCESS),
		}
	}
	acl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil,
	)
}

func explicitTestAccess(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE, permissions windows.ACCESS_MASK, mode windows.ACCESS_MODE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: permissions,
		AccessMode:        mode,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func currentTestUserSID() (*windows.SID, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return nil, err
	}
	defer token.Close()
	tu, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tu.User.Sid.Copy()
}
