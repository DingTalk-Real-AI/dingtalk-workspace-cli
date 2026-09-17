package aem

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gitlab.alibaba-inc.com/aes/aem-go-sdk/internal/encoder"
)

// AES 协议固定常量（不可修改，与 JS @ali/aes-tracker v3.3.18 对齐）。
const (
	sdkVersion  = "3.3.18"
	platformGo  = "go"
	uuidCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXTZabcdefghiklmnopqrstuvwxyz" // 60 字符（缺 Y 和 j）
	uuidLength  = 20
)

const (
	configPID       = "pid"
	configAppName   = "app_name"
	configEnv       = "env"
	configEndpoint  = "endpoint"
	configVersion   = "version"
	configAsync     = "async"
	configQueueSize = "queue_size"
	// configDisableAutoDimensions is a local privacy control. It is never
	// serialized; callers that enable it receive only explicitly configured
	// dimensions plus app_version derived from version.
	configDisableAutoDimensions = "disable_auto_dimensions"
)

// Config 是 SDK 接入参数，key 与 AEM 协议字段保持一致。
//
// 仅 pid 必填，其余字段为空时由 applyDefaults 填充默认值。Go SDK 会上报
// 服务端适用的公共维度，例如 app_name、env、version、uid、username、
// user_type、dim1~dim10、sid、bucket_id、ext。async、queue_size、endpoint
// 是 SDK 本地控制项，不会作为公共维度上报。
type Config map[string]interface{}

var supportedSendConfigKeys = map[string]struct{}{
	configPID:     {},
	configAppName: {},
	configEnv:     {},
	configVersion: {},
	"user_type":   {},
	"uid":         {},
	"username":    {},
	"sid":         {},
	"bucket_id":   {},
	"ext":         {},
	"platform":    {},
}

func init() {
	for i := 1; i <= 10; i++ {
		supportedSendConfigKeys[fmt.Sprintf("dim%d", i)] = struct{}{}
	}
}

func cloneConfig(c Config) Config {
	cloned := make(Config, len(c))
	for k, v := range c {
		cloned[k] = v
	}
	return cloned
}

// applyDefaults 为可选字段填充默认值，并返回新的配置副本。
func applyDefaults(c Config) Config {
	cloned := cloneConfig(c)
	if stringValue(cloned, configAppName) == "" {
		cloned[configAppName] = "unknown"
	}
	if stringValue(cloned, configEnv) == "" {
		cloned[configEnv] = "prod"
	}
	if stringValue(cloned, configEndpoint) == "" {
		cloned[configEndpoint] = "gm.mmstat.com"
	}
	if stringValue(cloned, configVersion) == "" {
		cloned[configVersion] = "unknown"
	}
	if _, ok := cloned[configAsync]; !ok {
		cloned[configAsync] = true
	}
	if intValue(cloned, configQueueSize) <= 0 {
		cloned[configQueueSize] = 1000
	}
	return cloned
}

// buildSendConfig 构建 gokey 编码所需的全局维度 map。
//
// 默认包含 AES 协议的 sdk_version/platform/device_id/os/os_version/app_name/
// app_version/pv_id/timezone_offset 自动采集字段，加上用户配置的 pid/env/uid/
// username/version/user_type/dim1~dim10/sid/bucket_id/ext 等公共维度。启用
// disable_auto_dimensions 时仅保留显式配置字段和由 version 派生的 app_version。
// 空值字段不会被写入；page_id、utm_* 等页面/浏览器字段不会在 Go SDK 中上报。
func buildSendConfig(c Config) map[string]string {
	m := make(map[string]string, len(c)+8)
	for k, v := range c {
		if _, ok := supportedSendConfigKeys[k]; !ok {
			continue
		}
		if s, ok := encoder.ItemToString(v); ok {
			m[k] = s
		}
	}
	if boolValue(c, configDisableAutoDimensions) {
		if v, ok := m["version"]; ok && v != "" {
			m["app_version"] = v
		}
		return m
	}

	m["sdk_version"] = sdkVersion
	if _, ok := m["platform"]; !ok {
		m["platform"] = platformGo
	}
	m["device_id"] = getDeviceID()
	m["os"] = capitalizeOS()
	m["os_version"] = getOSVersion()
	if v, ok := m["version"]; ok && v != "" {
		m["app_version"] = v
	} else {
		m["app_version"] = runtime.Version()
	}
	m["pv_id"] = generateUUID()
	m["timezone_offset"] = getTimezoneOffset()

	return m
}

func stringValue(c Config, key string) string {
	if c == nil {
		return ""
	}
	v, ok := c[key]
	if !ok {
		return ""
	}
	s, ok := encoder.ItemToString(v)
	if !ok {
		return ""
	}
	return s
}

func boolValue(c Config, key string) bool {
	if c == nil {
		return false
	}
	v, ok := c[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, err := strconv.ParseBool(val)
		return err == nil && b
	default:
		return false
	}
}

func intValue(c Config, key string) int {
	s := stringValue(c, key)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// getMAC 返回第一个非零的网卡 MAC 地址；获取失败时返回 "00:00:00:00:00:00"。
func getMAC() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "00:00:00:00:00:00"
	}
	for _, iface := range ifaces {
		mac := iface.HardwareAddr
		if len(mac) == 0 {
			continue
		}
		macStr := mac.String()
		if macStr != "" && macStr != "00:00:00:00:00:00" {
			return macStr
		}
	}
	return "00:00:00:00:00:00"
}

// getDeviceID 用 MAC 地址的 MD5 hex 作为设备指纹。
func getDeviceID() string {
	hash := md5.Sum([]byte(getMAC()))
	return hex.EncodeToString(hash[:])
}

// getOSVersion 调用系统命令获取内核版本字符串。
func getOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("cmd", "/c", "ver").Output()
		if err != nil {
			return "Windows"
		}
		return strings.TrimSpace(string(out))
	default:
		out, err := exec.Command("uname", "-r").Output()
		if err != nil {
			return "unknown"
		}
		return strings.TrimSpace(string(out))
	}
}

// getTimezoneOffset 对齐 JS Date.getTimezoneOffset()。东八区返回 "-480"。
func getTimezoneOffset() string {
	_, offset := time.Now().Zone()
	return fmt.Sprintf("%d", -(offset / 60))
}

// capitalizeOS 把 runtime.GOOS 首字母大写（"darwin" → "Darwin"）。
func capitalizeOS() string {
	osName := runtime.GOOS
	if len(osName) == 0 {
		return osName
	}
	return strings.ToUpper(osName[:1]) + osName[1:]
}

// generateUUID 生成 20 字符的随机 ID，字符集与 JS 版严格一致。
//
// 使用 rejection sampling 保证字符分布均匀（拒绝阈值 240 = 60*4）。
func generateUUID() string {
	charsetLen := len(uuidCharset)        // 60
	maxByte := byte(256 - 256%charsetLen) // 240
	result := make([]byte, uuidLength)
	buf := make([]byte, 1)
	for i := 0; i < uuidLength; i++ {
		for {
			_, _ = rand.Read(buf)
			if buf[0] < maxByte {
				result[i] = uuidCharset[buf[0]%byte(charsetLen)]
				break
			}
		}
	}
	return string(result)
}
