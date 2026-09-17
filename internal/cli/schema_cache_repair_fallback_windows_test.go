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

//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// denySharedV1Writes denies create/rename/write rights on the shared cache
// directory for the current user (Windows ignores the read-only attribute on
// directories), so lock creation fails with a non-timeout error.
func denySharedV1Writes(t *testing.T, dir string) {
	t.Helper()
	if err := applyRepairFallbackACL(dir, false); err != nil {
		t.Fatalf("deny shared cache writes: %v", err)
	}
	probe := filepath.Join(dir, ".dws-unwritable-probe")
	if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600); err == nil {
		_ = f.Close()
		_ = os.Remove(probe)
		_ = applyRepairFallbackACL(dir, true)
		t.Fatal("shared cache directory stayed writable after denying create rights")
	}
	t.Cleanup(func() {
		if err := applyRepairFallbackACL(dir, true); err != nil {
			t.Errorf("restore shared cache ACL: %v", err)
		}
	})
}

func repairFallbackUserSID() (*windows.SID, error) {
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

func repairFallbackAccess(sid *windows.SID, trustee windows.TRUSTEE_TYPE, mask windows.ACCESS_MASK, mode windows.ACCESS_MODE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: mask,
		AccessMode:        mode,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trustee,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func applyRepairFallbackACL(path string, writable bool) error {
	user, err := repairFallbackUserSID()
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
			repairFallbackAccess(user, windows.TRUSTEE_IS_USER, windows.GENERIC_ALL, windows.GRANT_ACCESS),
			repairFallbackAccess(system, windows.TRUSTEE_IS_USER, windows.GENERIC_ALL, windows.GRANT_ACCESS),
		}
	} else {
		admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
		if err != nil {
			return err
		}
		const fileDeleteChild windows.ACCESS_MASK = 0x00000040
		denyWrite := windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA |
			windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA | fileDeleteChild
		access = []windows.EXPLICIT_ACCESS{
			repairFallbackAccess(user, windows.TRUSTEE_IS_USER, denyWrite, windows.DENY_ACCESS),
			repairFallbackAccess(admins, windows.TRUSTEE_IS_GROUP, denyWrite, windows.DENY_ACCESS),
			repairFallbackAccess(user, windows.TRUSTEE_IS_USER, windows.GENERIC_READ|windows.GENERIC_EXECUTE, windows.GRANT_ACCESS),
			repairFallbackAccess(system, windows.TRUSTEE_IS_USER, windows.GENERIC_ALL, windows.GRANT_ACCESS),
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
