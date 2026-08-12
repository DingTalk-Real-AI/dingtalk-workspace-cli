package helpers

import "fmt"

// A fragment is the read-only result container returned by doc read
// --scope/--tags. It is not part of the writable JSONML schema, so write paths
// remove fragment wrappers and preserve their children before validation.

// jsonmlChildStart returns the index of the first child in a JSONML node array
// [tag, attrs?, ...children].
func jsonmlChildStart(arr []any) int {
	if len(arr) > 1 {
		if _, ok := arr[1].(map[string]any); ok {
			return 2
		}
	}
	return 1
}

// stripFragmentChildren recursively unwraps fragment nodes in a children
// slice, splicing each fragment's children in place.
func stripFragmentChildren(children []any) ([]any, int) {
	out := make([]any, 0, len(children))
	count := 0
	for _, child := range children {
		arr, ok := child.([]any)
		if !ok || len(arr) == 0 {
			out = append(out, child)
			continue
		}
		if tag, _ := arr[0].(string); tag == "fragment" {
			kids, nestedCount := stripFragmentChildren(arr[jsonmlChildStart(arr):])
			out = append(out, kids...)
			count += 1 + nestedCount
			continue
		}
		newNode, nestedCount := stripFragmentInNode(arr)
		out = append(out, newNode)
		count += nestedCount
	}
	return out, count
}

// stripFragmentInNode keeps a node's tag and attributes while recursively
// unwrapping fragments among its children.
func stripFragmentInNode(node []any) ([]any, int) {
	start := jsonmlChildStart(node)
	if start >= len(node) {
		return node, 0
	}
	kids, count := stripFragmentChildren(node[start:])
	if count == 0 {
		return node, 0
	}
	newNode := make([]any, 0, start+len(kids))
	newNode = append(newNode, node[:start]...)
	newNode = append(newNode, kids...)
	return newNode, count
}

// stripBodyFragments removes fragment wrappers from a document body. A
// top-level fragment is promoted to a root body so create/update can consume a
// scoped read result directly.
func stripBodyFragments(body []any) ([]any, int) {
	if len(body) == 0 {
		return body, 0
	}
	if tag, _ := body[0].(string); tag == "fragment" {
		kids, nestedCount := stripFragmentChildren(body[jsonmlChildStart(body):])
		root := make([]any, 0, 2+len(kids))
		root = append(root, "root", map[string]any{})
		root = append(root, kids...)
		return root, 1 + nestedCount
	}
	return stripFragmentInNode(body)
}

// stripNodeFragments removes fragment wrappers from a single block element.
// A top-level fragment must contain exactly one JSONML node because block
// insert/update accepts only one element.
func stripNodeFragments(node []any) ([]any, int, error) {
	if len(node) == 0 {
		return node, 0, nil
	}
	if tag, _ := node[0].(string); tag == "fragment" {
		kids, nestedCount := stripFragmentChildren(node[jsonmlChildStart(node):])
		switch len(kids) {
		case 0:
			return nil, 0, fmt.Errorf("fragment 只读容器为空，无可写回的节点；fragment 是 doc read --scope 的查询结果，不能写回")
		case 1:
			inner, ok := kids[0].([]any)
			if !ok {
				return nil, 0, fmt.Errorf("fragment 只读容器的子节点不是合法 JSONML 节点")
			}
			return inner, 1 + nestedCount, nil
		default:
			return nil, 0, fmt.Errorf("fragment 只读容器含 %d 个节点，block insert/update 一次只能写一个；请分多次调用，或用 doc update --content-format jsonml 整篇覆盖", len(kids))
		}
	}
	cleaned, count := stripFragmentInNode(node)
	return cleaned, count, nil
}
