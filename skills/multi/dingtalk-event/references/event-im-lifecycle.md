# IM consume 生命周期

## Ready 门禁

单事件只有出现以下 stderr 后才读取 stdout：

```text
[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>
```

多事件会先逐条输出 subscription，全部 IPC consumer 就绪后才输出：

```text
[event] subscription event_key=<key> subscribe_id=<id>
[event] subscription event_key=<key> subscribe_id=<id>
[event] ready event_count=2 bus_pid=<pid>
```

不能把 subscription 行当成整体 ready，也不要用 `sleep` 猜建联。任一订阅或 IPC 建联失败，
Runtime 会回滚本次已创建订阅。

## 有界与长期任务

- 有界消费使用 `--max-events N` 或 `--duration 10m`。
- 长期任务由宿主管理子进程并持续读取 stdout/stderr，避免管道背压。
- `--output-dir`/`--route` 是明确落盘任务，不要用文件 watcher 代替 stdout 事件循环。
- stdout 的 NDJSON 每行独立解析；单行失败不能吞掉后续事件。

## 干净退出

本次新建的订阅会在 SIGTERM、Ctrl+C、符合条件的 stdin EOF、duration 或 max-events 退出时
自动取消。复用 `--subscribe-id` 的订阅默认保留，除非显式 `--ephemeral`。

多事件中停止一个 subscribe_id 只移除对应 consumer；其余继续，最后一个移除后进程退出。
禁止 `kill -9`，它会跳过清理。外部停止流程见 [event-im-operations.md](event-im-operations.md)。
