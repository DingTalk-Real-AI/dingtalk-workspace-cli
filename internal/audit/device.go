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

package audit

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// CollectDevice fills a Device record.
//
// OS is always cheap and non-identifying, so it is always set. DeviceID
// (machine UUID) and SerialNo (hardware serial) are personal information under
// PIPL — they are collected ONLY when fingerprint == true (the enterprise must
// explicitly opt in and disclose it to the user). Hostname is included with the
// fingerprint tier since it can identify a machine/user.
func CollectDevice(fingerprint bool) Device {
	d := Device{OS: runtime.GOOS}
	if !fingerprint {
		return d
	}
	if h, err := os.Hostname(); err == nil {
		d.Hostname = h
	}
	d.DeviceID = machineID()
	d.SerialNo = serialNo()
	return d
}

// ioregField extracts a quoted value for the given key from `ioreg` output,
// e.g. `"IOPlatformSerialNumber" = "C02XXXXXXXXX"`.
func ioregField(key string) string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*=\s*"([^"]+)"`)
	m := re.FindSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// machineID returns a stable per-machine identifier, best-effort per OS.
func machineID() string {
	switch runtime.GOOS {
	case "darwin":
		return ioregField("IOPlatformUUID")
	case "linux":
		// /etc/machine-id is the systemd-standard stable id; fall back to the
		// dbus copy. Neither requires root.
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(p); err == nil {
				if id := strings.TrimSpace(string(b)); id != "" {
					return id
				}
			}
		}
	case "windows":
		// MachineGuid lives in the registry; read via reg.exe to avoid a
		// Windows-only build dependency.
		out, err := exec.Command("reg", "query",
			`HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
		if err == nil {
			fields := strings.Fields(string(out))
			if len(fields) > 0 {
				return fields[len(fields)-1]
			}
		}
	}
	return ""
}

// serialNo returns the hardware serial number, best-effort per OS.
func serialNo() string {
	switch runtime.GOOS {
	case "darwin":
		return ioregField("IOPlatformSerialNumber")
	case "linux":
		// Usually root-only; return what we can read, empty otherwise.
		if b, err := os.ReadFile("/sys/class/dmi/id/product_serial"); err == nil {
			return strings.TrimSpace(string(b))
		}
	case "windows":
		out, err := exec.Command("wmic", "bios", "get", "serialnumber").Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 2 {
				return strings.TrimSpace(lines[1])
			}
		}
	}
	return ""
}
