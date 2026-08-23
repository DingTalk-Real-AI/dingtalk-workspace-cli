package errors

// 统一退出码表（B171/B172；轮 10 裁决⑬，权威 = 规划 v1.2 OQ-1 定案）：
// 类别与退出码一一对应，`confirmation_required` 是 `validation` 下的子类
// 共享 3，不新增独立退出码。
//
// 跨包同源锁定：internal/output.ExitCodeForEnvelope 对同一信封必须给出
// 本表完全一致的码（api=1/auth=2/validation=3/discovery=6/internal=5）。
// Typed output.Partial remains the only path to partial_failure exit 7.
// 修改本表 = 契约变更，必须双侧同步并更新两侧同源测试。
var exitCodeByCategory = map[Category]int{
	CategoryAPI:        ExitCodeAPI,
	CategoryAuth:       ExitCodeAuth,
	CategoryValidation: ExitCodeValidation,
	CategoryDiscovery:  ExitCodeDiscovery,
	CategoryInternal:   ExitCodeInternal,
	CategoryPartial:    ExitCodeInternal,
}
