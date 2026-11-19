// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
	"github.com/google/uuid"
)

const (
	maxImageBytes         = 20 * 1024 * 1024
	maxImagePixels        = 100 * 1000 * 1000
	maxImageSide          = 32768
	imageTargetConstraint = "part-id 选内嵌、page-id 选独立白板，二者互斥"
	imageFileConstraint   = "仅支持 PNG/JPEG，不支持 SVG；常规文件不超过 20MiB、1 亿像素、单边 32768px；显示尺寸保持原图比例"
)

var newImageNodeID = uuid.NewString

var imageReceiptDirectory = func() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "dws", "whiteboard-image-receipts"), nil
}

type imagePreflightKey struct{}

type imagePreflight struct {
	fileBytes       []byte
	fileDigest      string
	fileSize        int64
	mimeType        string
	intrinsicWidth  int
	intrinsicHeight int
	width           int
	height          int
	x               int
	y               int
	target          map[string]any
	snapshotDigest  string
	queryCall       whiteboardcore.Call
}

var InsertImage = shortcut.Shortcut{
	OutputRollout:    output.RolloutUnifiedActive,
	Service:          "whiteboard",
	Command:          "+insert-image",
	Product:          serverWhiteboard,
	Description:      "把本地 PNG/JPEG 插入已有文档内嵌或独立白板",
	Intent:           "编辑已有白板时直接插入本地图片；上传前只读预览和确认，上传后核对真实 diff，再 append 并读回",
	Risk:             shortcut.RiskHighWrite,
	SemanticReviewed: true,
	SemanticDelta:    "本地校验 PNG/JPEG 与画布布局后只读预览目标，用户确认后上传托管资源，以真实资源运行 diff；目标未变且无 blocker 时 append 并按原更新链路精确读回，dry-run 不上传。",
	Safety:           whiteboardWriteSafety(),
	Contract: func() corecmd.ContractDecl {
		declaration := whiteboardContract(
			"+insert-image", "shortcut_insert_image", "把本地 PNG/JPEG 插入已有白板并读回验证",
			"Composite local image validation, read-only preview, managed upload, final diff, append and exact readback.",
			&contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","description":"插图预览或已验证的终态结果","properties":{"nodeId":{"type":"string","description":"承载白板的节点身份"},"partId":{"type":"string","description":"内嵌白板 part 身份"},"pageId":{"type":"string","description":"独立白板页面身份"},"uploaded":{"type":"boolean","description":"资源上传是否完成"},"committed":{"type":"boolean","description":"节点写入是否有提交证据"},"verified":{"type":"boolean","description":"同目标读回是否完成"},"dryRun":{"type":"boolean","description":"是否仅为上传前预览"},"resourceId":{"type":"string","description":"托管资源身份"},"receiptFile":{"type":"string","description":"本地恢复回执路径"},"preview":{"type":"object","description":"上传前布局与目标预览","additionalProperties":true},"update":{"type":"object","description":"现有更新链路的写入及读回回执","additionalProperties":true}},"required":["nodeId","uploaded","committed","verified"],"additionalProperties":false}`), SensitivePaths: []string{"nodeId", "partId", "pageId", "resourceId", "receiptFile"}},
			&contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: true},
			[]contract.ParamDecl{{Name: "node", Property: "nodeId"}, {Name: "part-id", Property: "partId"}, {Name: "page-id", Property: "pageId", RequiredWhen: "独立白板时"}, {Name: "file", Property: "file"}, {Name: "x", Property: "x"}, {Name: "y", Property: "y"}, {Name: "width", Property: "width"}, {Name: "height", Property: "height"}},
			"已有白板插入本地 PNG/JPEG 图片", "SVG、GIF/WebP、远程图片 URL 或新建白板文件", "dws whiteboard +insert-image --node <ID> --page-id <PAGE> --file ./photo.png --x 100 --y 80 --width 640",
		)
		index := 0
		declaration.Selection.ExampleDispositions = []contract.ExampleDisposition{{
			Index: &index, Mode: contract.ExampleDispositionModeContractOnly,
			ReasonCode: contract.ExampleDispositionReasonStatefulPreflight,
			Reason:     "dry-run must query the live whiteboard to preview the actual target; the isolated Agent example runner has no remote whiteboard fixture",
			Reviewed:   true,
		}}
		return declaration
	}(),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Required: true, Desc: "承载文档或独立白板节点 ID"},
		{Name: "part-id", Type: shortcut.FlagString, Desc: "文档内白板 part ID；" + imageTargetConstraint},
		{Name: "page-id", Type: shortcut.FlagString, Desc: "独立白板目标页面 ID；" + imageTargetConstraint, RequiredWhen: "未提供 part-id 时"},
		{Name: "file", Type: shortcut.FlagString, Required: true, Desc: "本地 PNG/JPEG 文件路径；" + imageFileConstraint},
		{Name: "x", Type: shortcut.FlagInt, Default: "0", Desc: "画布横坐标，整数像素"},
		{Name: "y", Type: shortcut.FlagInt, Default: "0", Desc: "画布纵坐标，整数像素"},
		{Name: "width", Type: shortcut.FlagInt, Desc: "显示宽度，整数像素；不提供时最大 800px；" + imageFileConstraint},
		{Name: "height", Type: shortcut.FlagInt, Desc: "显示高度，整数像素；" + imageFileConstraint},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"part-id", "page-id"}, Description: imageTargetConstraint}, {Kind: shortcut.ConstraintCustom, Flags: []string{"file", "width", "height"}, Description: imageFileConstraint}},
	Tips:        []string{"dws whiteboard +insert-image --node <DOC_ID> --part-id <PART_ID> --file ./photo.jpg --x 100 --y 80 --width 640", "dws whiteboard +insert-image --node <WHITEBOARD_ID> --page-id <PAGE_ID> --file ./photo.png --width 640"},
	Validate:    preflightInsertImage,
	Execute:     executeInsertImage,
}

func preflightInsertImage(rt *shortcut.RuntimeContext) error {
	if rt.Changed("part-id") == rt.Changed("page-id") {
		return apperrors.NewValidation("必须且只能提供 --part-id 或 --page-id")
	}
	if rt.Changed("part-id") && rt.Str("part-id") == "" || rt.Changed("page-id") && rt.Str("page-id") == "" {
		return apperrors.NewValidation("目标 part-id/page-id 不能为空")
	}
	file, err := inspectLocalImage(rt.Str("file"))
	if err != nil {
		return err
	}
	file.x, file.y = rt.Int("x"), rt.Int("y")
	file.width, file.height, err = imageDisplaySize(file.intrinsicWidth, file.intrinsicHeight, rt.Int("width"), rt.Int("height"), rt.Changed("width"), rt.Changed("height"))
	if err != nil {
		return err
	}
	call, err := whiteboardDiffQueryCall(rt)
	if err != nil {
		return err
	}
	projected, err := readInsertImageTarget(rt, call)
	if err != nil {
		return err
	}
	source, ok := projected["source"].(map[string]any)
	if !ok {
		return apperrors.NewAPI("目标白板未返回完整 source", apperrors.WithReason("incomplete_snapshot"))
	}
	page, nodes, err := singleDiffPage(source)
	if err != nil {
		return err
	}
	// The embedded target cannot fail diffTarget; the standalone page match is
	// already enforced by projectStandaloneWhiteboardQuery(view=page).
	file.target, _ = diffTarget(projected, call, page)
	// The source was decoded from resultJson, so every value is JSON encodable.
	file.snapshotDigest, _ = whiteboardSnapshotDigest(source)
	file.queryCall = call
	rt.Command().SetContext(context.WithValue(rt.Command().Context(), imagePreflightKey{}, file))
	fmt.Fprintf(rt.Command().ErrOrStderr(), "插图预览：目标 %s，现有节点 %d；图片 %dx%d，位置 (%d,%d)，显示 %dx%d。确认后先上传资源，再核对真实 diff 并追加图片；上传后取消可能留下未引用资源。\n", call.Kind, len(nodes), file.intrinsicWidth, file.intrinsicHeight, file.x, file.y, file.width, file.height)
	return nil
}

func inspectLocalImage(path string) (*imagePreflight, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, apperrors.NewValidation("无法打开图片文件: " + err.Error())
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxImageBytes {
		return nil, apperrors.NewValidation("图片必须是 1 字节至 20MiB 的常规文件")
	}
	return inspectImageContents(path, f, info.Size())
}

func inspectImageContents(path string, reader io.Reader, fileSize int64) (*imagePreflight, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxImageBytes+1))
	if err != nil || int64(len(data)) != fileSize {
		return nil, apperrors.NewValidation("读取图片失败或文件在读取时发生变化")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "png" && format != "jpeg" {
		return nil, apperrors.NewValidation("图片内容必须是 PNG 或 JPEG，不支持 SVG 等其他格式；修改扩展名不能转换图片格式")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if format == "png" && ext != ".png" || format == "jpeg" && ext != ".jpg" && ext != ".jpeg" {
		return nil, apperrors.NewValidation("图片扩展名与实际 PNG/JPEG 内容不符")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageSide || config.Height > maxImageSide || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return nil, apperrors.NewValidation("图片像素尺寸超过首版限制")
	}
	if format == "jpeg" && hasNonDefaultEXIFOrientation(data) {
		return nil, apperrors.NewValidation("JPEG 的 EXIF 方向不是默认方向；请先旋转并保存后重试")
	}
	sum := sha256.Sum256(data)
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
	}
	return &imagePreflight{fileBytes: data, fileDigest: hex.EncodeToString(sum[:]), fileSize: fileSize, mimeType: mime, intrinsicWidth: config.Width, intrinsicHeight: config.Height}, nil
}

func imageDisplaySize(intrinsicWidth, intrinsicHeight, width, height int, hasWidth, hasHeight bool) (int, int, error) {
	if hasWidth && (width <= 0 || width > maxImageSide) || hasHeight && (height <= 0 || height > maxImageSide) {
		return 0, 0, apperrors.NewValidation("显示宽高必须在 1..32768 范围内")
	}
	if !hasWidth && !hasHeight {
		width = min(intrinsicWidth, 800)
		hasWidth = true
	}
	if hasWidth && !hasHeight {
		height = int(math.Round(float64(width) * float64(intrinsicHeight) / float64(intrinsicWidth)))
	}
	if !hasWidth && hasHeight {
		width = int(math.Round(float64(height) * float64(intrinsicWidth) / float64(intrinsicHeight)))
	}
	if width <= 0 || height <= 0 {
		return 0, 0, apperrors.NewValidation("按原图比例计算的显示尺寸退化为 0；请改指定另一边或调整显示尺寸")
	}
	if width > maxImageSide || height > maxImageSide {
		return 0, 0, apperrors.NewValidation("按原图比例计算的显示尺寸超出范围")
	}
	if hasWidth && hasHeight && math.Abs(float64(width)*float64(intrinsicHeight)-float64(height)*float64(intrinsicWidth)) > float64(max(intrinsicWidth, intrinsicHeight)) {
		return 0, 0, apperrors.NewValidation("显示宽高不能拉伸图片，请只指定其中一边或保持原图比例")
	}
	return width, height, nil
}

func hasNonDefaultEXIFOrientation(data []byte) bool {
	// DecodeConfig does not apply orientation. Reject EXIF-bearing JPEG unless the
	// orientation tag is absent or explicitly 1; malformed EXIF fails closed.
	for cursor := 2; cursor+4 < len(data) && data[cursor] == 0xff; {
		marker := data[cursor+1]
		if marker == 0xda || marker == 0xd9 {
			break
		}
		length := int(data[cursor+2])<<8 | int(data[cursor+3])
		if length < 2 || cursor+2+length > len(data) {
			return true
		}
		segment := data[cursor+4 : cursor+2+length]
		if marker == 0xe1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			return exifOrientationIsNotDefault(segment[6:])
		}
		cursor += 2 + length
	}
	return false
}

func exifOrientationIsNotDefault(tiff []byte) bool {
	if len(tiff) < 8 {
		return true
	}
	le := bytes.Equal(tiff[:2], []byte{'I', 'I'})
	if !le && !bytes.Equal(tiff[:2], []byte{'M', 'M'}) {
		return true
	}
	read16 := func(b []byte) int {
		if le {
			return int(b[0]) | int(b[1])<<8
		}
		return int(b[0])<<8 | int(b[1])
	}
	read32 := func(b []byte) int {
		if le {
			return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24
		}
		return int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	}
	if read16(tiff[2:4]) != 42 {
		return true
	}
	offset := read32(tiff[4:8])
	if offset < 0 || offset+2 > len(tiff) {
		return true
	}
	count := read16(tiff[offset : offset+2])
	if count > (len(tiff)-offset-2)/12 {
		return true
	}
	for i := 0; i < count; i++ {
		entry := tiff[offset+2+i*12 : offset+14+i*12]
		if read16(entry[:2]) == 0x112 {
			return read16(entry[2:4]) != 3 || read32(entry[4:8]) != 1 || read16(entry[8:10]) != 1
		}
	}
	return false
}

func readInsertImageTarget(rt *shortcut.RuntimeContext, call whiteboardcore.Call) (map[string]any, error) {
	var data map[string]any
	var err error
	if rt.DryRun() {
		// These two exact routes are read-only. The generic dry-run classifier does
		// not recognize the embedded route's historical read_ prefix.
		if call.Tool != toolQuery && call.Tool != toolQueryStandalone {
			return nil, apperrors.NewValidation("插图 dry-run 只允许白板查询")
		}
		data, err = rt.CallMCPReadData(serverWhiteboard, call.Tool, call.Args)
		if err == nil && data == nil {
			err = apperrors.NewAPI("白板查询返回了无效结果")
		}
	} else {
		data, err = rt.CallMCPData(serverWhiteboard, call.Tool, call.Args)
	}
	if err != nil {
		return nil, helpers.WithWhiteboardRecovery(err)
	}
	if call.Kind == whiteboardcore.KindEmbedded {
		return projectWhiteboardQuery(data, rt.Str("node"), rt.Str("part-id"))
	}
	return projectStandaloneWhiteboardQuery(data, call.Args)
}

func executeInsertImage(rt *shortcut.RuntimeContext) error {
	preflight, ok := rt.Command().Context().Value(imagePreflightKey{}).(*imagePreflight)
	if !ok {
		return apperrors.NewInternal("插图预检状态缺失")
	}
	preview := map[string]any{"target": preflight.target, "fileSize": preflight.fileSize, "intrinsicWidth": preflight.intrinsicWidth, "intrinsicHeight": preflight.intrinsicHeight, "x": preflight.x, "y": preflight.y, "width": preflight.width, "height": preflight.height, "approximate": true, "snapshotDigest": preflight.snapshotDigest}
	if rt.DryRun() {
		return rt.Output(map[string]any{"nodeId": rt.Str("node"), "uploaded": false, "committed": false, "verified": false, "dryRun": true, "preview": preview})
	}
	currentFile, err := inspectLocalImage(rt.Str("file"))
	if err != nil {
		return err
	}
	if currentFile.fileDigest != preflight.fileDigest || currentFile.fileSize != preflight.fileSize {
		return apperrors.NewValidation("图片在预览后发生变化；请重新执行插图命令")
	}
	privateDir, err := os.MkdirTemp("", "dws-whiteboard-image-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(privateDir)
	return uploadStagedImage(rt, preflight, privateDir, preview)
}

// uploadStagedImage owns the private snapshot and every action after upload.
// Keeping this boundary explicit lets local staging failures stop before any
// network request while preserving one cleanup owner in executeInsertImage.
func uploadStagedImage(rt *shortcut.RuntimeContext, preflight *imagePreflight, privateDir string, preview map[string]any) error {
	fileName := filepath.Base(rt.Str("file"))
	privateFile := filepath.Join(privateDir, fileName)
	if err := os.WriteFile(privateFile, preflight.fileBytes, 0600); err != nil {
		return err
	}
	var upload map[string]any
	if err := helpers.RunDocMediaUploadWithFile(rt.Command(), privateFile, fileName, func(data map[string]any) error { upload = data; return nil }); err != nil {
		return err
	}
	resourceID, _ := upload["resourceId"].(string)
	resourceURL, _ := upload["resourceUrl"].(string)
	// RunDocMediaUploadWithFile validates both fields before transferring bytes.
	receiptFile, err := persistImageReceipt(rt, preflight, upload)
	if err != nil {
		return apperrors.NewAPI("图片资源已上传，但无法保存本地回执（资源 ID "+resourceID+"）: "+err.Error(), apperrors.WithHint("请先核对白板与上传资源状态，勿盲目重复追加"))
	}
	postUploadFile, err := inspectLocalImage(rt.Str("file"))
	if err != nil || postUploadFile.fileDigest != preflight.fileDigest {
		return imageRecoveryError(apperrors.NewValidation("图片在上传期间发生变化；已停止写入白板"), receiptFile)
	}
	node := map[string]any{"id": "image-" + newImageNodeID(), "type": "image", "x": preflight.x, "y": preflight.y, "width": preflight.width, "height": preflight.height, "intrinsicWidth": preflight.intrinsicWidth, "intrinsicHeight": preflight.intrinsicHeight, "resource": map[string]any{"kind": "managed", "resourceId": resourceID, "url": resourceURL}}
	// The node is constructed from validated image dimensions and upload
	// metadata. Building the typed update directly avoids serializing a fixed
	// source only to parse and validate that same source again.
	nodes := []map[string]any{node}
	encoded, _ := json.Marshal(nodes) // Closed shape: maps contain only strings and ints.
	parsed := &parsedUpdate{NodesJSON: string(encoded), Nodes: nodes}
	projected, err := readInsertImageTarget(rt, preflight.queryCall)
	if err != nil {
		return imageRecoveryError(err, receiptFile)
	}
	finalDiff, err := buildWhiteboardDiff(projected, preflight.queryCall, parsed, nil, "semantic", defaultDiffDetailLimit)
	if err != nil {
		return imageRecoveryError(err, receiptFile)
	}
	return commitImageAfterDiff(rt, preflight, parsed, resourceID, receiptFile, preview, finalDiff)
}

// commitImageAfterDiff is the last policy boundary before the remote write.
// It keeps future diff risks and update-call validation fail closed even if
// the current append-only image intent cannot produce those risks itself.
func commitImageAfterDiff(rt *shortcut.RuntimeContext, preflight *imagePreflight, parsed *parsedUpdate, resourceID, receiptFile string, preview, finalDiff map[string]any) error {
	if finalDiff["snapshotDigest"] != preflight.snapshotDigest {
		return imageRecoveryError(apperrors.NewValidation("目标白板在预览后发生变化；必须重新确认差异"), receiptFile)
	}
	diffTarget, _ := finalDiff["target"].(map[string]any)
	for _, key := range []string{"kind", "nodeId", "partId", "pageId", "revision"} {
		if fmt.Sprint(diffTarget[key]) != fmt.Sprint(preflight.target[key]) {
			return imageRecoveryError(apperrors.NewValidation("白板目标或 revision 在预览后发生变化；必须重新确认差异"), receiptFile)
		}
	}
	summary, _ := finalDiff["summary"].(map[string]any)
	if blockers, _ := nonNegativeInt(summary["blockerCount"]); blockers != 0 {
		return imageRecoveryError(apperrors.NewValidation("真实 diff 存在 blocker，已停止提交"), receiptFile)
	}
	if warnings, _ := finalDiff["warningSummary"].([]any); len(warnings) != 0 {
		for _, raw := range warnings {
			warning, _ := raw.(map[string]any)
			if warning["reason"] != "embedded_preview_not_atomic" {
				return imageRecoveryError(apperrors.NewValidation("真实 diff 出现预览之外的风险；必须重新确认差异"), receiptFile)
			}
		}
	}
	options := whiteboardcore.UpdateOptions{Target: whiteboardcore.Target{NodeID: rt.Str("node"), PartID: rt.Str("part-id"), PartIDChanged: rt.Changed("part-id")}, Mode: "append", NodesJSON: parsed.NodesJSON}
	if preflight.queryCall.Kind == whiteboardcore.KindStandalone {
		// The standalone query projection requires a nonnegative revision;
		// diffTarget preserves it in the target map.
		revision, _ := nonNegativeInt(diffTarget["revision"])
		options.PageID, options.PageIDChanged = rt.Str("page-id"), true
		options.ExpectedRevision, options.ExpectedRevisionChanged = revision, true
		options.RequestID, options.RequestIDChanged = "wb-image-"+uuid.NewString(), true
	}
	call, err := whiteboardcore.BuildUpdateCall(options)
	if err != nil {
		return imageRecoveryError(err, receiptFile)
	}
	// buildWhiteboardDiff computed sourceDigest from this same immutable parsed
	// value. Comparing it against itself cannot detect remote changes; the
	// snapshot and target checks above are the optimistic concurrency guards.
	return commitInsertedImage(rt, preflight, parsed, call, resourceID, receiptFile, preview)
}

func persistImageReceipt(rt *shortcut.RuntimeContext, preflight *imagePreflight, upload map[string]any) (string, error) {
	dir, err := imageReceiptDirectory()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, uuid.NewString()+".json")
	data, err := json.MarshalIndent(map[string]any{"nodeId": rt.Str("node"), "partId": rt.Str("part-id"), "pageId": rt.Str("page-id"), "fileDigest": preflight.fileDigest, "upload": upload, "uploaded": true, "committed": false, "verified": false}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := helpers.AtomicWriteJSON(path, data); err != nil {
		return "", err
	}
	return path, nil
}

func updateImageReceipt(path string, committed, verified bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var receipt map[string]any
	if err := json.Unmarshal(data, &receipt); err != nil {
		return err
	}
	if receipt == nil {
		return apperrors.NewAPI("本地图片资源回执必须是 JSON 对象")
	}
	receipt["committed"], receipt["verified"] = committed, verified
	data, _ = json.MarshalIndent(receipt, "", "  ") // Values came from JSON plus booleans.
	return helpers.AtomicWriteJSON(path, data)
}

func imageRecoveryError(err error, receiptFile string) error {
	return apperrors.NewAPI("图片资源已上传，白板最终状态需核对: "+err.Error(), apperrors.WithHint("资源回执保存在 "+receiptFile+"；核对目标状态后再恢复，勿盲目重复追加"))
}

func commitInsertedImage(rt *shortcut.RuntimeContext, preflight *imagePreflight, parsed *parsedUpdate, call whiteboardcore.Call, resourceID, receiptFile string, preview map[string]any) error {
	receiptData, err := rt.CallMCPWriteDataStrict(serverWhiteboard, call.Tool, call.Args)
	if err != nil {
		return imageRecoveryError(err, receiptFile)
	}
	var result map[string]any
	if call.Kind == whiteboardcore.KindEmbedded {
		receipt, err := requireWhiteboardUpdateReceipt(receiptData, call.Args, "append", parsed)
		if err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		if err := updateImageReceipt(receiptFile, true, false); err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		readback, err := rt.CallMCPData(serverWhiteboard, toolQuery, map[string]any{"nodeId": call.Args["nodeId"], "partId": call.Args["partId"]})
		if err != nil {
			return imageRecoveryError(whiteboardCommittedVerificationError(err, call.Args, "append", receipt), receiptFile)
		}
		projected, err := projectWhiteboardQuery(readback, rt.Str("node"), rt.Str("part-id"))
		if err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		if err := verifyWhiteboardUpdate(parsed, projected, receipt.IDMap); err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		result = projectWhiteboardUpdateSuccess(call.Args, "append", parsed, projected, receipt)
	} else {
		receipt, err := requireStandaloneWhiteboardUpdateReceipt(receiptData, call.Args, parsed)
		if err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		if err := updateImageReceipt(receiptFile, true, false); err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		readArgs := map[string]any{"nodeId": call.Args["nodeId"], "view": "page", "pageId": receipt.PageID}
		readback, err := rt.CallMCPData(serverWhiteboard, toolQueryStandalone, readArgs)
		if err != nil {
			return imageRecoveryError(standaloneWhiteboardCommittedVerificationError(err, call.Args, receipt), receiptFile)
		}
		projected, err := projectStandaloneWhiteboardQuery(readback, readArgs)
		if err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		if revision, ok := nonNegativeInt(projected["revision"]); !ok || revision != receipt.CommittedRevision {
			return imageRecoveryError(apperrors.NewAPI("读回 revision 与提交回执不一致"), receiptFile)
		}
		if err := verifyWhiteboardUpdate(parsed, projected, receipt.IDMap); err != nil {
			return imageRecoveryError(err, receiptFile)
		}
		result = projectStandaloneWhiteboardUpdateSuccess(call.Args, parsed, projected, receipt)
	}
	if err := updateImageReceipt(receiptFile, true, true); err != nil {
		return imageRecoveryError(err, receiptFile)
	}
	return rt.Output(map[string]any{"nodeId": rt.Str("node"), "uploaded": true, "committed": true, "verified": true, "resourceId": resourceID, "receiptFile": receiptFile, "preview": preview, "update": result})
}
