// Package aem 提供阿里巴巴 AEM (Application Experience Management) 平台的 Go 上报 SDK。
//
// 它适用于任何 Go 程序（web server、cron job、library 等），通过 Tracker.Track
// 将埋点事件加入后台队列并异步上报到 AES 后端。
//
// 简单用法：
//
//	tracker := aem.NewTracker(aem.Config{
//	    "pid": "your_project_id",
//	    "app_name": "my-service",
//	    "env": "prod",
//	    "version": "1.0.0",
//	})
//	defer tracker.Close()
//
//	tracker.Track(aem.Event{
//	    Type: "api",
//	    Fields: map[string]string{
//	        "url": "/api/user", "status": "200", "duration": "120",
//	    },
//	})
package aem

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.alibaba-inc.com/aes/aem-go-sdk/internal/encoder"
	"gitlab.alibaba-inc.com/aes/aem-go-sdk/internal/sender"
)

// ErrQueueFull 表示异步上报队列已满，本次事件没有入队。
var ErrQueueFull = errors.New("aem: async queue is full")

// ErrTrackerClosed 表示 Tracker 已关闭，不能再接收新的事件。
var ErrTrackerClosed = errors.New("aem: tracker is closed")

// Tracker 是 SDK 的核心入口，封装了配置、全局维度和上报通道。
//
// 一个进程通常只创建一个 Tracker，并发调用 Track 是安全的。生命周期内
// sendCfg 只构建一次以降低开销。
type Tracker struct {
	config  Config
	sendCfg map[string]string

	async      bool
	queue      chan string
	workerDone chan struct{}

	mu       sync.RWMutex
	closed   bool
	asyncErr error
}

// NewTracker 创建并初始化一个 Tracker。默认会立刻填充并采集设备维度
// （MAC、OS、PVID 等）；启用 disable_auto_dimensions 时跳过这些采集。
func NewTracker(c Config) *Tracker {
	cfg := applyDefaults(c)
	t := &Tracker{
		config:  cfg,
		sendCfg: buildSendConfig(cfg),
		async:   boolValue(cfg, configAsync),
	}
	if t.async {
		t.queue = make(chan string, intValue(cfg, configQueueSize))
		t.workerDone = make(chan struct{})
		go t.runWorker()
	}
	return t
}

// Track 上报一个事件，失败时返回 error。
//
// 如果 Event 没有自带 ts 字段，Track 会自动补 time.Now().UnixMilli()。
// 默认 async=true 时，Track 只负责把事件写入内存队列，不等待远端 HTTP 请求完成；
// 返回 error 仅表示参数校验、队列已满或 Tracker 已关闭。async=false 时，Track
// 会在当前 goroutine 内同步发送并返回远端发送结果。
func (t *Tracker) Track(event Event) error {
	if stringValue(t.config, configPID) == "" {
		return errors.New(`aem: config field "pid" is required`)
	}
	if event.Type == "" {
		return errors.New("aem: Event.Type is required")
	}

	fields := event.toMap()
	if _, ok := fields["ts"]; !ok {
		fields["ts"] = fmt.Sprintf("%d", time.Now().UnixMilli())
	}

	gokey := encoder.ProcessData([]map[string]string{fields}, t.sendCfg)

	if !t.async {
		t.mu.RLock()
		closed := t.closed
		t.mu.RUnlock()
		if closed {
			return ErrTrackerClosed
		}
		return t.send(gokey)
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.closed {
		return ErrTrackerClosed
	}
	select {
	case t.queue <- gokey:
		return nil
	default:
		return ErrQueueFull
	}
}

func (t *Tracker) runWorker() {
	defer close(t.workerDone)
	for gokey := range t.queue {
		if err := t.send(gokey); err != nil {
			t.asyncErr = err
		}
	}
}

// userAgent builds the User-Agent header from config: "app_name/version".
func (t *Tracker) userAgent() string {
	name := stringValue(t.config, configAppName)
	ver := stringValue(t.config, configVersion)
	if name == "" || name == "unknown" {
		return ""
	}
	if ver != "" && ver != "unknown" {
		return name + "/" + ver
	}
	return name
}

func (t *Tracker) send(gokey string) error {
	return sender.Send(stringValue(t.config, configEndpoint), gokey, t.userAgent())
}

// Config 返回 Tracker 当前使用的配置（副本）。
//
// 返回值是副本，修改它不会影响 Tracker 内部配置。
func (t *Tracker) Config() Config {
	return cloneConfig(t.config)
}

// Close 释放 Tracker 持有的资源。
//
// async=true 时，Close 会停止接收新事件，并等待队列中已入队的事件发送完成。
// 如果后台发送发生错误，Close 返回最后一次发送错误。
func (t *Tracker) Close() error {
	if !t.async {
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
		return nil
	}

	t.mu.Lock()
	if !t.closed {
		t.closed = true
		close(t.queue)
	}
	workerDone := t.workerDone
	t.mu.Unlock()

	<-workerDone
	return t.asyncErr
}
