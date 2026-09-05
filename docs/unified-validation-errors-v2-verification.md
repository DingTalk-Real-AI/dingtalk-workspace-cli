# 统一校验框架 v2：验证与性能记录

本页保留首次实现的历史验证数据。后续架构缺口、修复与同一时段配对性能测量见[复审记录](unified-validation-errors-v2-review.md)；复审仍为 Draft，不能将本页的历史全量通过视为当前方案已经通过。

测量日期：2026-09-05 至 2026-09-06。对应 `fix/unified-validation-errors` 的统一校验框架 v2 实现，数据在提交前采集。方案状态为 Draft，本记录不代表性能验收已批准。

实现和功能检查已完成。性能已实测，但**尚不能签署“无性能退化”结论**：根命令构建的跨时段基准中位数增加约 20.5%，分配字节数增加约 1.6%；同一时段的新旧进程交替测量没有检出稳定的整进程耗时差异。后者不能排除被进程启动成本掩盖的构建退化。

## 功能与交付

- `DWS_PACKAGE_VERSION=0.0.0-test go test -p 2 ./... -timeout=20m`：通过，903.5 秒。app 470.482 秒，helpers 155.059 秒，脚本集成包 369.953 秒。
- `scripts/policy/check-typed-validation-errors.sh`：通过；检查指定用例确实 run 且 pass。
- `scripts/policy/check-generated-drift.sh`：通过，包括两次 Schema 组装的确定性。
- `scripts/policy/check-schema-catalog.sh`：通过，31 个产品、1,357 个工具；覆盖参数、帮助、声明绑定和确认契约。
- `make build`：通过，含 macOS runtime payload 附加和签名。
- 最终命令树扫描覆盖 1,809 个节点；edition fixture 验证位置参数、必填、参数组和解析错误，plugin fixture 验证 overlay 解析错误，共 5 个真实执行场景。
- 修改的 Go 文件已检查 gofmt，`git diff --check` 通过。

实际构建产物在隔离的 `DWS_CONFIG_DIR` 中验证，以下均为本地参数失败，无需调用远端服务：

| 场景 | 进程退出码 | JSON 契约 |
| --- | ---: | --- |
| `audit tail --lines 0 --format json` | 3 | stderr，legacy `error.category=validation` / `code=3` |
| `oa approval list-by-admin --request '{' --format json` | 3 | stderr，legacy `error.category=validation` / `code=3` |
| `skill search --query value --unknown-validation-gate-flag --format json` | 3 | stderr，legacy validation，保留可用参数提示 |
| `skill install --format json` | 3 | stderr，legacy `invalid_positionals` |
| `sheet revision-get --format json` | 3 | stdout，unified `ok=false` / `error.type=validation` / `exit_code=3`；stderr 为空 |

较早的验证曾出现一个审计异步等待用例失败，以及独立命令测试仍断言旧 Cobra 文案的问题。审计代码未修改，最终全量通过；文案断言已按统一契约修正，并保留失败时不调用业务的断言。

## 根命令构建

环境：Apple M3 Pro，darwin/arm64，Go 1.26.1，基准后缀 `-12`。前后使用相同命令和继承环境，每组 5 轮、每轮 `-benchtime=1s`。正式基准期间没有本任务启动的其他测试或构建，但桌面和后台进程仍在运行，不能视为整机隔离环境。基线在本次架构改动前采集，候选数据在功能检查和构建后采集，两组不在同一时段。

```sh
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/app \
  -run '^$' -bench '^BenchmarkNewRootCommand$' \
  -benchmem -benchtime=1s -count=5
```

| 指标（中位数） | 改造前 | 改造后 | 变化 |
| --- | ---: | ---: | ---: |
| 耗时（ms/op） | 15.612 | 18.811 | +20.49% |
| 分配字节（B/op） | 17,243,069 | 17,511,708 | +1.56% |
| 分配次数（allocs/op） | 160,297 | 159,717 | -0.36% |

耗时范围：改造前 13.793–17.404 ms；改造后 16.416–22.593 ms。分配字节增加是可观察的成本，未以分配次数下降抵消它。跨时段耗时差异无法全部归因于代码，也不能被忽略。

原始正式采样：

```text
goos: darwin
goarch: arm64
pkg: github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app
cpu: Apple M3 Pro
BenchmarkNewRootCommand-12    	      79	  15660012 ns/op	17254840 B/op	  160350 allocs/op
BenchmarkNewRootCommand-12    	      80	  15425018 ns/op	17236180 B/op	  160222 allocs/op
BenchmarkNewRootCommand-12    	      79	  13792983 ns/op	17230777 B/op	  160215 allocs/op
BenchmarkNewRootCommand-12    	      74	  15611731 ns/op	17243069 B/op	  160297 allocs/op
BenchmarkNewRootCommand-12    	      67	  17404286 ns/op	17251133 B/op	  160357 allocs/op
PASS
ok  	github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app	14.706s

goos: darwin
goarch: arm64
pkg: github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app
cpu: Apple M3 Pro
BenchmarkNewRootCommand-12    	      58	  17747968 ns/op	17544226 B/op	  159803 allocs/op
BenchmarkNewRootCommand-12    	      72	  19228407 ns/op	17511708 B/op	  159688 allocs/op
BenchmarkNewRootCommand-12    	      64	  16416389 ns/op	17505968 B/op	  159660 allocs/op
BenchmarkNewRootCommand-12    	      66	  18810896 ns/op	17511321 B/op	  159717 allocs/op
BenchmarkNewRootCommand-12    	      57	  22593164 ns/op	17522003 B/op	  159784 allocs/op
PASS
ok  	github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app	17.012s
```

## 已准备命令的执行成本

`BenchmarkPreparedCommandExecution` 排除建树和 transport；每个场景先构造并准备自己的小型命令树，预热 Cobra 自动 help/completion，然后反复执行相同 argv。成功路径包含共享 `Validate`，校验错误包含 parser、位置参数、required 和自定义 Validate。这里刻意保留 Cobra 的 flag 状态，不代表不同用户输入可以复用一棵树而自动清空参数。

```sh
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/corecmd \
  -run '^$' -bench '^BenchmarkPreparedCommandExecution$' \
  -benchmem -benchtime=1s -count=5
```

| 场景（5 轮中位数） | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| success | 2.365 | 4,299 | 32 |
| invalid_flag | 1.980 | 2,850 | 30 |
| missing_required | 1.248 | 2,529 | 22 |
| invalid_positionals | 1.375 | 2,586 | 25 |
| invalid_parameters | 1.916 | 3,274 | 25 |

这是新实现的绝对成本，没有对应的旧实现小型树基线，因此不用于宣称执行性能改善。

## 同时段整进程交替对照

保留改造前已有的构建产物，与本次 `make build` 产物比较。已核对二者均为 Go 1.26.1、darwin/arm64、PIE、trimpath、CGO_ENABLED=1。每个场景先各预热 3 次，再测量 30 对；每对交替 before/after 的启动顺序。用 Python `perf_counter_ns` 测量 `subprocess.run`，收集输出并验证真实退出码；两者使用同一隔离配置目录。每次均为新进程，预热意味着 OS 文件缓存可能已热，不是冷磁盘测试。

| 场景 | 旧进程中位数 ms | 新进程中位数 ms | 中位数之比变化 | 配对变化中位数的 bootstrap 95% 区间 |
| --- | ---: | ---: | ---: | --- |
| version | 352.776 | 347.684 | -1.44% | -5.28% 至 +2.11% |
| audit_validation | 373.131 | 370.332 | -0.75% | -1.73% 至 +9.13% |
| oa_validation | 338.358 | 340.751 | +0.71% | -3.51% 至 +2.26% |

区间由 30 个配对相对差值重采样 10,000 次估计，随机种子 42；它只描述这次采样，不能保证其他负载或命令没有退化。所有区间包含 0，因此没有从本组数据检出稳定的整进程差异；整进程约 340–373 ms 的开销可能掩盖数毫秒的建树差异。

二进制 SHA-256：

```text
before 7e80ba1e3bafdce762910875912bb25cb78a25fdbe7d529153fec0d3b6e6f9dc
after 61738de9c538ae29e480c099364b184bca4df246cdf95f6a318b879d6c4acf36
```

配对原始耗时（ms，保留三位小数）：

```csv
pair,version_before,version_after,audit_before,audit_after,oa_before,oa_after
1,364.332,334.206,362.243,318.215,519.477,453.148
2,323.537,359.387,338.564,361.206,478.639,431.197
3,357.768,331.530,361.845,343.268,345.821,401.766
4,335.410,338.817,408.942,371.149,456.795,446.413
5,340.483,356.182,385.473,337.770,327.209,347.365
6,420.205,354.057,397.462,346.648,360.079,332.325
7,352.437,332.819,361.994,394.301,334.210,351.478
8,353.114,326.414,362.136,359.707,330.368,337.050
9,327.531,345.170,535.447,384.081,346.623,312.429
10,331.880,320.002,353.839,380.245,359.876,339.922
11,364.228,353.236,399.908,394.387,332.885,344.921
12,347.663,357.399,428.575,618.842,393.737,347.283
13,362.049,322.773,411.006,340.094,332.462,351.479
14,368.633,350.198,340.854,357.087,334.228,341.580
15,331.353,364.853,345.811,378.902,379.955,345.081
16,338.288,337.576,385.188,384.799,342.489,371.286
17,332.891,338.725,328.959,367.384,323.995,322.591
18,326.240,315.501,377.936,421.100,323.177,310.670
19,333.862,360.943,428.818,396.626,351.691,324.393
20,365.322,329.658,464.048,530.129,348.922,337.903
21,404.046,385.175,390.882,504.893,308.949,311.308
22,377.399,420.562,417.673,355.865,377.318,387.198
23,342.114,350.549,346.189,355.512,349.462,314.501
24,356.654,336.058,368.327,360.658,321.531,325.965
25,327.510,333.242,382.356,432.858,309.917,342.924
26,373.110,371.677,322.085,346.402,322.979,369.214
27,321.961,334.841,336.938,336.056,322.034,329.485
28,548.362,411.022,323.398,369.515,344.544,338.583
29,642.611,538.771,336.387,444.951,324.461,315.082
30,355.296,387.693,416.625,455.544,332.788,334.015
```

性能验收结论：功能检查通过；整进程对照未检出稳定差异；根构建基准存在退化信号，仍保留为交付风险。如交付条件要求严格无退化，需要在受控负载下对相同版本的前后 benchmark 产物做同时段交替采样，或先减少构建快照和闭包的分配成本再复验。
