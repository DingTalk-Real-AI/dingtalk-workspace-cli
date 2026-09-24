// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package preview provides a deterministic reference renderer for fast local
// authoring checks. It is intentionally not a DingTalk client fidelity claim.
package preview

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/state"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type node struct {
	ID, Kind, Text, Variant                                    string
	Label, Placeholder, Href, Alt                              string
	BindingPath, Value, EventName, EventSurfaceID              string
	Description, MimeType, FileSize, FileType, FileMeta        string
	Theme, Color, Align, Justify, InputType, DisplayStyle      string
	Gap, Padding, Radius, Weight, BackgroundToken, BorderToken string
	MaxLine, MaxHeight, DownloadHref                           string
	PreviewEventName, PreviewEventSurfaceID                    string
	DefaultExpanded, Disabled, PreviewEnabled                  bool
	ImageSrc, IconSrc                                          template.URL
	NodeStyle, PanelStyle                                      template.CSS
	MarkdownHTML, IconSVG                                      template.HTML
	Children                                                   []node
	Options                                                    []option
}

type option struct {
	Label, Value string
	Selected     bool
}

const document = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>A2UI reference preview</title><script>if(new URLSearchParams(location.search).has("embed")){document.documentElement.classList.add("embed")}document.addEventListener("DOMContentLoaded",()=>{const result=document.getElementById("interaction-result");result.setAttribute("role","status");result.setAttribute("aria-live","polite");document.querySelectorAll('[data-component="ChoicePicker"]').forEach(group=>{group.setAttribute("role","radiogroup");group.setAttribute("aria-label",group.querySelector(".FieldLabel")?.textContent||"请选择")})})</script><style>
.embed{background:transparent}.embed body{padding:0;background:transparent}.embed .notice{display:none}
:root{--space-1:4px;--space-2:8px;--space-3:12px;--space-4:16px;color-scheme:light dark;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC",sans-serif;background:#f3f5f8;color:#171a1f}*{box-sizing:border-box}body{margin:0;padding:28px 18px 56px}.notice,.interaction-result{max-width:600px;margin:0 auto var(--space-3);color:#687078;font-size:12px;line-height:1.5}.surface{max-width:600px;margin:auto}.Card{background:#fff;border:1px solid #e3e7ec;border-radius:16px;padding:var(--space-3);box-shadow:0 10px 32px #18203312}.Column{display:flex;min-width:0;flex-direction:column;gap:var(--space-3)}.Card>.Column{gap:var(--space-2)}.Row{display:flex;min-width:0;align-items:center;gap:var(--space-2)}.Row>*{min-width:0}.Row[data-align="start"]{align-items:flex-start}.Row[data-align="end"]{align-items:flex-end}.Row[data-align="stretch"]{align-items:stretch}.Row[data-justify="center"]{justify-content:center}.Row[data-justify="end"]{justify-content:flex-end}.Column[data-align="center"]{align-items:center}.Column[data-align="end"]{align-items:flex-end}.Column[data-align="stretch"]{align-items:stretch}[data-gap="2"]{gap:2px}[data-gap="4"]{gap:var(--space-1)}[data-gap="8"]{gap:var(--space-2)}[data-gap="10"]{gap:10px}[data-gap="12"]{gap:var(--space-3)}[data-gap="16"]{gap:var(--space-4)}[data-weight="1"]{flex:1 1 0}[data-padding="4"]{padding:var(--space-1)}[data-padding="8"]{padding:var(--space-2)}[data-padding="10"]{padding:10px}[data-padding="12"]{padding:var(--space-3)}[data-padding="16"]{padding:var(--space-4)}[data-padding="20"]{padding:20px}[data-radius="8"]{border-radius:8px}.token-common_fg_z1_color{background:#f5f7fa}.Card.token-common_fg_z1_color{box-shadow:none}.border-token-common_line_hard_color{border-color:#d5dce4;border-style:solid}.Text{line-height:1.45;overflow-wrap:anywhere}.Text[data-max-line]{display:-webkit-box;overflow:hidden;-webkit-box-orient:vertical}.Text.title{font-size:20px;font-weight:680;letter-spacing:-.01em}.Text.bold{font-weight:650}.Text.caption{font-size:13px}.Text.gray{color:#687078}.Tag{display:inline-flex;width:max-content;max-width:100%;align-items:center;border-radius:999px;padding:3px 9px;font-size:12px;font-weight:550;white-space:nowrap}.Tag.blue{background:#e8f3ff;color:#075fc1}.Tag.green{background:#e8f8ef;color:#0b6b38}.Tag.orange{background:#fff3e5;color:#934900}.Tag.red{background:#ffebeb;color:#b22222}.Tag.gray,.Tag.black{background:#eef1f4;color:#4c5561}.Tag.hollow{border:1px solid currentColor;background:transparent}.Divider{height:1px;margin:1px 0;background:#e4e7eb}.ImagePreview{display:block;border-radius:10px;outline-offset:2px}.ImagePreview:focus-visible{outline:3px solid #1677ff55}.ImageFrame{position:relative;width:100%;height:200px;margin:0;overflow:hidden;border-radius:10px;background:#eef2f6}.Image{display:block;width:100%;height:100%;object-fit:cover}.ImageFrame.contain .Image{object-fit:contain}.ImageFrame[data-load="error"]{height:112px}.ImageFrame[data-load="error"] .Image{display:none}.ImageFallback{height:100%;padding:20px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:var(--space-1);color:#687078;text-align:center}.ImageFallback strong{color:#343b44;font-size:14px}.ImageFallback[hidden]{display:none}.Markdown{font-size:15px;line-height:1.65;color:#282e36}.Markdown>:first-child{margin-top:0}.Markdown>:last-child{margin-bottom:0}.Markdown h1,.Markdown h2,.Markdown h3,.Markdown h4,.Markdown h5,.Markdown h6{line-height:1.35;margin:0 0 10px;color:#171a1f;font-weight:660}.Markdown h1{font-size:21px}.Markdown h2{font-size:18px}.Markdown h3{font-size:16px}.Markdown h4,.Markdown h5,.Markdown h6{font-size:15px}.Markdown p{margin:0 0 10px}.Markdown ul,.Markdown ol{margin:0 0 10px;padding-left:22px}.Markdown li+li{margin-top:var(--space-1)}.Markdown a,.Link a{color:#126ee3;text-decoration:none}.Markdown a:hover,.Link a:hover{text-decoration:underline}.Markdown code{padding:2px 5px;border-radius:4px;background:#f0f2f5;font-size:.9em}.Markdown blockquote{margin:0 0 10px;padding-left:var(--space-3);border-left:3px solid #d9dde3;color:#687078}[data-component-id="metrics"]{flex-wrap:wrap}[data-component-id="metrics"]>.Column{min-width:88px;min-height:68px;justify-content:center}[data-component-id^="metric_value_"]{font-size:18px;color:#171a1f}.FileControl{display:flex;align-items:center;gap:var(--space-2);padding:var(--space-2);border:1px solid #e2e6eb;border-radius:10px;background:#fafbfc}.FileMain{display:flex;min-width:0;flex:1;align-items:center;gap:var(--space-3);padding:var(--space-1);color:inherit;text-decoration:none}.FileControl:hover{border-color:#b8c8df;background:#f7f9fc}.FileIcon{display:flex;width:40px;height:40px;flex:none;align-items:center;justify-content:center;border-radius:8px;background:#e8f3ff;color:#075fc1;font-size:11px;font-weight:700;letter-spacing:.03em}.FileCopy{min-width:0;flex:1}.FileName{display:block;overflow:hidden;color:#20252b;font-weight:600;text-overflow:ellipsis;white-space:nowrap}.FileMeta{display:block;margin-top:3px;color:#5f6872;font-size:12px}.FileAction{display:flex;min-width:44px;min-height:44px;align-items:center;justify-content:center;padding:var(--space-2);color:#075fc1;font-size:13px;font-weight:600;text-decoration:none}.CollapsiblePanel{border-radius:10px;background:#f7f8fa}.CollapsiblePanel details{padding:0 var(--space-3)}.CollapsiblePanel summary{display:flex;min-height:44px;align-items:center;gap:var(--space-2);padding:var(--space-3) 0;cursor:pointer;color:#343b44;font-weight:600;list-style:none}.CollapsiblePanel summary::-webkit-details-marker{display:none}.CollapsiblePanel summary::before{width:8px;height:8px;flex:none;border-right:2px solid currentColor;border-bottom:2px solid currentColor;content:"";transform:rotate(-45deg);transform-origin:center}.CollapsiblePanel details[open]>summary::before{transform:rotate(45deg)}.PanelContent{position:relative;margin:0;padding:var(--space-3) 0;overflow:auto}.CollapsiblePanel.indented .PanelContent{padding-left:calc(var(--space-2) + 1px)}.CollapsiblePanel.indented .PanelContent::before{position:absolute;top:var(--space-3);bottom:var(--space-3);left:0;width:1px;border-radius:1px;background:#d9dde3;content:""}.Button{flex:1}.ButtonControl{box-sizing:border-box;width:100%;min-height:44px;padding:10px 14px;border:0;border-radius:9px;background:#1677ff;color:#fff;font:inherit;font-weight:600;cursor:pointer}.ButtonControl.default{background:#eef1f5;color:#24292f}.ButtonControl.borderless{background:transparent;color:#075fc1}.ButtonControl:hover{filter:brightness(.97)}.ButtonControl:focus-visible{outline:3px solid #1677ff33;outline-offset:2px}.ButtonControl:active{transform:translateY(1px)}.ButtonControl:disabled{cursor:not-allowed;opacity:.45}.FieldLabel{display:block;margin-bottom:6px;color:#4c5561;font-size:13px;font-weight:600}.TextFieldControl{box-sizing:border-box;width:100%;padding:11px;border:1px solid #d4d9df;border-radius:8px;background:#fff;color:#171a1f;font:inherit;resize:vertical}.TextFieldControl:focus{outline:2px solid #1677ff33;border-color:#1677ff}.ChoicePickerControl{padding:var(--space-1) 0}.option{display:inline-flex;align-items:center;gap:6px;margin:5px 14px 5px 0;cursor:pointer}.interaction-result{box-sizing:border-box;margin-top:14px;padding:var(--space-3);border:1px solid #d9dde3;border-radius:9px;background:#fff;color:#171a1f}.interaction-result pre{margin:var(--space-2) 0 0;white-space:pre-wrap;word-break:break-word}.interaction-result[hidden]{display:none}@media(max-width:480px){body{padding:var(--space-4) 10px 40px}.Card{border-radius:14px}.Text.title{font-size:19px}[data-component-id="metrics"]{gap:6px}[data-component-id="metrics"]>.Column{min-width:88px}}@media(prefers-color-scheme:dark){:root{background:#121416;color:#eef1f5}.Card,.interaction-result{background:#1f2328;border-color:#353b42;color:#eef1f5}.Divider{background:#353b42}.token-common_fg_z1_color,.CollapsiblePanel{background:#292e35}.Markdown,.Markdown h1,.Markdown h2,.Markdown h3,.Markdown h4,.Markdown h5,.Markdown h6,.Text.title,[data-component-id^="metric_value_"],.FileName,.CollapsiblePanel summary{color:#eef1f5}.Markdown code{background:#292e35}.FieldLabel,.Text.gray,.FileMeta,.ImageFallback{color:#c4cad1}.TextFieldControl{background:#171a1f;border-color:#4c5561;color:#eef1f5}.ButtonControl.default{background:#353b42;color:#eef1f5}.FileControl{border-color:#3b424a;background:#252a30}.border-token-common_line_hard_color{border-color:#4d5661}.FileControl:hover{border-color:#586675;background:#292f36}.CollapsiblePanel.indented .PanelContent::before{background:#4d5661}.ImageFrame{background:#292e35}.ImageFallback strong{color:#eef1f5}.FileAction,.Link a{color:#79b8ff}}
</style></head><body><div class="notice">DWS A2UI reference_preview · 可本地编辑表单并模拟 Action，不发送真实请求；用于结构、层级与字段映射快速验证，不代表钉钉客户端像素级效果</div><main class="surface">{{template "node" .}}</main><aside id="interaction-result" class="interaction-result" hidden><strong>本地 Action 模拟结果</strong><pre id="interaction-json"></pre></aside>{{define "node"}}<section class="{{.Kind}} {{.Variant}} {{.Theme}} {{.Color}} {{.BackgroundToken}} {{.BorderToken}}{{if .DefaultExpanded}} is-expanded{{end}}" data-component-id="{{.ID}}" data-component="{{.Kind}}" data-align="{{.Align}}" data-justify="{{.Justify}}" data-gap="{{.Gap}}" data-padding="{{.Padding}}" data-radius="{{.Radius}}" data-weight="{{.Weight}}" data-max-line="{{.MaxLine}}"{{if .LineStyle}} style="{{.LineStyle}}"{{end}}>{{if eq .Kind "Image"}}{{if .PreviewEnabled}}<a class="ImagePreview" href="{{.ImageSrc}}" target="_blank" rel="noopener noreferrer" aria-label="预览：{{.Alt}}">{{end}}<figure class="ImageFrame {{.Variant}} {{.Align}}" data-load="pending"><img class="Image" src="{{.ImageSrc}}" alt="{{.Alt}}"><figcaption class="ImageFallback" hidden><strong>图片暂不可用</strong><span>{{.Alt}}</span></figcaption></figure>{{if .PreviewEnabled}}</a>{{end}}{{else if eq .Kind "File"}}<div class="FileControl"><a class="FileMain" href="{{.Href}}" target="_blank" rel="noopener noreferrer" data-preview-event="{{.PreviewEventName}}" data-surface-id="{{.PreviewEventSurfaceID}}"><span class="FileIcon">{{if .IconSrc}}<img src="{{.IconSrc}}" alt="">{{else}}{{.FileType}}{{end}}</span><span class="FileCopy"><span class="FileName">{{.Text}}</span><span class="FileMeta">{{.FileMeta}}</span></span></a><a class="FileAction" href="{{.DownloadHref}}" target="_blank" rel="noopener noreferrer" aria-label="打开 {{.Text}}"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2.5 12s3.5-5.5 9.5-5.5 9.5 5.5 9.5 5.5-3.5 5.5-9.5 5.5S2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="2.7"/></svg></a></div>{{else if eq .Kind "TextField"}}<label>{{if .Label}}<span class="FieldLabel">{{.Label}}</span>{{end}}{{if eq .Variant "longText"}}<textarea class="TextFieldControl" placeholder="{{.Placeholder}}" data-binding-path="{{.BindingPath}}"{{if .Disabled}} disabled{{end}}>{{.Value}}</textarea>{{else}}<input class="TextFieldControl" placeholder="{{.Placeholder}}" value="{{.Value}}" data-binding-path="{{.BindingPath}}"{{if .Disabled}} disabled{{end}}>{{end}}</label>{{else if eq .Kind "ChoicePicker"}}<div class="ChoicePickerControl">{{if .Label}}<div class="FieldLabel">{{.Label}}</div>{{end}}{{range .Options}}<label class="option"><input type="{{$.InputType}}" name="{{$.ID}}" value="{{.Value}}" data-binding-path="{{$.BindingPath}}" {{if .Selected}}checked{{end}}{{if $.Disabled}} disabled{{end}}> {{.Label}}</label>{{end}}</div>{{else if eq .Kind "Markdown"}}{{.MarkdownHTML}}{{else if eq .Kind "Link"}}{{if .Href}}<a href="{{.Href}}" target="_blank" rel="noopener noreferrer">{{.Text}}</a>{{else}}{{.Text}}{{end}}{{else if eq .Kind "CollapsiblePanel"}}<details{{if .DefaultExpanded}} open{{end}}><summary>{{.Text}}</summary><div class="PanelContent" data-max-height="{{.MaxHeight}}"{{if .PanelStyle}} style="{{.PanelStyle}}"{{end}}>{{range .Children}}{{template "node" .}}{{end}}</div></details>{{else if eq .Kind "Button"}}<button type="button" class="ButtonControl {{.Variant}}" data-event-name="{{.EventName}}" data-surface-id="{{.EventSurfaceID}}"{{if .Disabled}} disabled{{end}}>{{if .Text}}{{.Text}}{{end}}{{range .Children}}{{template "node" .}}{{end}}</button>{{else}}{{if .Text}}{{.Text}}{{end}}{{range .Children}}{{template "node" .}}{{end}}{{end}}</section>{{end}}<script>
(()=>{const emit=(name,surfaceId)=>{if(!name)return;const model=collect();const payload={event:{name,context:{surfaceId,form:model.form??{}}},localPreview:true};document.getElementById("interaction-json").textContent=JSON.stringify(payload,null,2);document.getElementById("interaction-result").hidden=false;document.dispatchEvent(new CustomEvent("dws-a2ui-preview-action",{detail:payload}))};const markImage=(image)=>{const frame=image.closest(".ImageFrame");const fallback=frame.querySelector(".ImageFallback");const failed=!image.naturalWidth||image.naturalWidth<=1||image.naturalHeight<=1;frame.dataset.load=failed?"error":"ready";fallback.hidden=!failed};document.querySelectorAll(".Image").forEach(image=>{image.addEventListener("load",()=>markImage(image));image.addEventListener("error",()=>markImage(image));if(image.complete)markImage(image)});const assign=(target,path,value)=>{const parts=path.replace(/^\//,"").split("/").filter(Boolean);let current=target;parts.forEach((part,index)=>{if(index===parts.length-1){current[part]=value;return}current[part]??={};current=current[part]})};const collect=()=>{const model={};document.querySelectorAll("[data-binding-path]").forEach(control=>{if((control.type==="radio"||control.type==="checkbox")&&!control.checked)return;assign(model,control.dataset.bindingPath,control.value)});return model};document.querySelectorAll("button[data-event-name]").forEach(button=>button.addEventListener("click",()=>emit(button.dataset.eventName,button.dataset.surfaceId)));document.querySelectorAll("[data-preview-event]").forEach(link=>link.addEventListener("click",()=>emit(link.dataset.previewEvent,link.dataset.surfaceId)))})();
</script></body></html>`

// These rules mirror public Image.variant geometry. They are injected rather
// than tied to fixture IDs so every valid A2UI message gets the same behavior.
const previewEnhancements = `.ImagePreview{width:max-content;max-width:100%}.Image.header>.ImagePreview,.Image.largeFeature>.ImagePreview{width:100%}.ImageFrame.icon{width:24px;height:24px}.ImageFrame.avatar{width:40px;height:40px;border-radius:50%}.ImageFrame.smallFeature{width:96px;height:96px}.ImageFrame.mediumFeature{width:160px;height:120px}.ImageFrame.largeFeature{width:min(320px,100%);height:200px}.ImageFrame.header{width:100%;height:200px}.ImageFrame.header.contain{background:#fff}.Text.black,.Icon.black{color:#171a1f}.Text.darkGray,.Icon.darkGray{color:#4c5561}.Text.blue,.Icon.blue{color:#126ee3}.Text.green,.Icon.green{color:#168455}.Text.orange,.Icon.orange{color:#a65300}.Text.red,.Icon.red{color:#c93636}.Text.white,.Icon.white{color:#fff}.InlineIcon{display:inline-flex;width:16px;height:16px;flex:none;align-items:center;justify-content:center;vertical-align:-2px}.Text.caption>.InlineIcon{width:14px;height:14px}.Text.title>.InlineIcon{width:18px;height:18px}.InlineIcon svg,.InlineIcon img{display:block;width:100%;height:100%;object-fit:contain}.Icon>.InlineIcon{width:20px;height:20px;vertical-align:middle}.Text:has(>.InlineIcon){display:inline-flex;align-items:center;gap:6px}.ButtonControl{border-radius:999px;transition:background-color .16s ease,color .16s ease,filter .16s ease,transform .08s ease}.ButtonControl .Text{color:inherit}.ButtonControl .Text:has(>.InlineIcon){justify-content:center}.ButtonControl:hover{background:#0f6fe8}.ButtonControl.default:hover{background:#e5e9ef}.ButtonControl.borderless:hover{background:#e8f3ff}.ButtonControl:disabled:hover{filter:none}.token-common_level1_base_color{color:#171a1f}.token-common_level2_base_color{color:#4c5561}.token-common_level3_base_color{color:#687078}.token-common_level4_base_color{color:#9aa2ac}.token-common_link_color,.token-theme_primary1_color{color:#126ee3}.token-common_red1_color{color:#c93636}.token-common_orange1_color{color:#a65300}.token-common_green1_color{color:#168455}@media(prefers-color-scheme:dark){.Text.black,.Text.darkGray,.Icon.black,.Icon.darkGray,.token-common_level1_base_color{color:#eef1f5}.Text.blue,.Icon.blue,.token-common_link_color,.token-theme_primary1_color{color:#79b8ff}.Text.green,.Icon.green,.token-common_green1_color{color:#58c995}.Text.orange,.Icon.orange,.token-common_orange1_color{color:#ffb866}.Text.red,.Icon.red,.token-common_red1_color{color:#ff8585}.token-common_level2_base_color{color:#c4cad1}.token-common_level3_base_color{color:#9da6b1}.token-common_level4_base_color{color:#727b86}.Text.custom-color,.Icon.custom-color{color:var(--text-dark)!important}}`

const layoutEnhancements = `.Row>.Image{flex:none}.Row>.Image.icon{width:24px}.Row>.Image.avatar{width:40px}.Row>.Image.smallFeature{width:96px}.Row>.Image.mediumFeature{width:160px}.Row>.Image>.ImagePreview{width:100%}.Row>[data-weight="1"]{width:0}.CardHeader{display:flex;min-height:48px;align-items:center;justify-content:space-between;gap:var(--space-2);padding:var(--space-3);background:#e8f3ff;color:#20252b;font-size:17px;font-weight:650}.CardHeader>.Tag{flex:none}.CardHeader.green{background:#e8f8ef}.CardHeader.orange{background:#fff3e5}.CardHeader.red{background:#ffebeb}`

const markdownTableEnhancements = `.Markdown table{width:100%;border-collapse:collapse;table-layout:fixed;line-height:1.45}.Markdown th,.Markdown td{padding:8px;border:1px solid #e3e7ec;text-align:left;overflow-wrap:anywhere}.Markdown th{background:#f5f7fa;color:#171a1f;font-weight:650}@media(prefers-color-scheme:dark){.Markdown th,.Markdown td{border-color:#3b424a}.Markdown th{background:#292e35;color:#eef1f5}}`

const reasoningEnhancements = `.CollapsiblePanel.reasoning{background:transparent}`

// Match the public File mapper's measured attachment row geometry in Nest.
const fileEnhancements = `.FileControl{height:64px;min-height:64px;gap:0;padding:12px;border:.5px solid rgba(24,28,31,.2);border-radius:12px;background:#fff;box-shadow:none}.FileControl:hover{border-color:rgba(24,28,31,.2);background:#fff}.FileMain{gap:12px;padding:0}.FileIcon{width:40px;height:40px;padding:0;border-radius:0;background:transparent}.FileIcon img{display:block;width:40px;height:40px;object-fit:contain}.FileCopy{min-width:0}.FileName{font-size:14px;line-height:20px;font-weight:500;color:#141414}.FileMeta{margin-top:2px;color:rgba(20,20,20,.35);font-size:12px;line-height:16px}.FileAction{width:16px;min-width:16px;height:16px;min-height:16px;margin-left:12px;padding:0;color:rgba(20,20,20,.35)}.FileAction svg{display:block;width:16px;height:16px;fill:none;stroke:currentColor;stroke-width:1.7;stroke-linecap:round;stroke-linejoin:round}@media(prefers-color-scheme:dark){.FileControl,.FileControl:hover{background:#18191a;border-color:rgba(255,255,255,.09)}.FileName{color:#eef1f5}.FileMeta,.FileAction{color:#8f93a3}}`

const choicePickerEnhancements = `.ChoicePickerControl select{display:block;width:100%;min-height:44px;margin-top:8px;padding:0 12px;border:1px solid #d4dce6;border-radius:8px;background:#fff;color:#20252b;font:inherit}.ChoicePickerControl select:focus-visible{outline:2px solid #1677ff;outline-offset:2px}@media(prefers-color-scheme:dark){.ChoicePickerControl select{background:#1f2328;border-color:#454d57;color:#eef1f5}}`

const choicePickerMarkup = `{{else if eq .Kind "ChoicePicker"}}<div class="ChoicePickerControl">{{if .Label}}<div class="FieldLabel">{{.Label}}</div>{{end}}{{if eq .DisplayStyle "dropdown"}}<select data-binding-path="{{.BindingPath}}"{{if .Disabled}} disabled{{end}}><option value="">请选择</option>{{range .Options}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}</select>{{else}}{{range .Options}}<label class="option"><input type="{{$.InputType}}" name="{{$.ID}}" value="{{.Value}}" data-binding-path="{{$.BindingPath}}" {{if .Selected}}checked{{end}}{{if $.Disabled}} disabled{{end}}> {{.Label}}</label>{{end}}{{end}}</div>{{else if eq .Kind "Markdown"}}`

func Render(surface state.Surface) ([]byte, error) {
	root, ok := surface.Components["root"]
	if !ok {
		return nil, fmt.Errorf("preview requires component id root")
	}
	view, err := buildNode("root", root, surface, map[string]bool{})
	if err != nil {
		return nil, err
	}
	renderDocument := strings.Replace(document, "</style>", previewEnhancements+layoutEnhancements+markdownTableEnhancements+reasoningEnhancements+fileEnhancements+choicePickerEnhancements+"</style>", 1)
	renderDocument = strings.Replace(renderDocument, `{{else if eq .Kind "ChoicePicker"}}<div class="ChoicePickerControl">{{if .Label}}<div class="FieldLabel">{{.Label}}</div>{{end}}{{range .Options}}<label class="option"><input type="{{$.InputType}}" name="{{$.ID}}" value="{{.Value}}" data-binding-path="{{$.BindingPath}}" {{if .Selected}}checked{{end}}{{if $.Disabled}} disabled{{end}}> {{.Label}}</label>{{end}}</div>{{else if eq .Kind "Markdown"}}`, choicePickerMarkup, 1)
	renderDocument = strings.Replace(renderDocument, `const collect=()=>{const model={};document.querySelectorAll("[data-binding-path]").forEach(control=>{if((control.type==="radio"||control.type==="checkbox")&&!control.checked)return;assign(model,control.dataset.bindingPath,control.value)});return model};`, `const collect=()=>{const model={};const selected=new Map();document.querySelectorAll("[data-binding-path]").forEach(control=>{const path=control.dataset.bindingPath;if(control.type==="radio"||control.type==="checkbox"){if(!selected.has(path))selected.set(path,[]);if(control.checked)selected.get(path).push(control.value)}else if(control.tagName==="SELECT"){assign(model,path,control.value?[control.value]:[])}else{assign(model,path,control.value)}});selected.forEach((values,path)=>assign(model,path,values));return model};`, 1)
	renderDocument = strings.Replace(renderDocument, `group.setAttribute("role","radiogroup");`, `if(group.querySelector("select"))return;group.setAttribute("role",group.classList.contains("multipleSelection")?"group":"radiogroup");`, 1)
	// CollapsiblePanel does not declare content padding in the public protocol.
	// Keep the preview honest: a message must add an explicit padded Row/Column
	// when it needs an inset instead of receiving preview-only whitespace.
	renderDocument = strings.Replace(renderDocument, `.PanelContent{position:relative;margin:0;padding:var(--space-3) 0;overflow:auto}`, `.PanelContent{position:relative;margin:0;padding:0;overflow:auto}`, 1)
	// Component wrappers and their native <img> previously shared the .Image
	// class. Scope replaced image rules to the native element so a wrapper in a
	// Row cannot inherit height:100% and push the remaining card content away.
	renderDocument = strings.Replace(renderDocument, `.Image{display:block;width:100%;height:100%;object-fit:cover}`, `.ImageFrame>.Image{display:block;width:100%;height:100%;object-fit:cover}`, 1)
	renderDocument = strings.Replace(renderDocument, `.ImageFrame.contain .Image{object-fit:contain}`, `.ImageFrame.contain>.Image{object-fit:contain}`, 1)
	renderDocument = strings.Replace(renderDocument, `.ImageFrame[data-load="error"] .Image{display:none}`, `.ImageFrame[data-load="error"]>.Image{display:none}`, 1)
	renderDocument = strings.Replace(renderDocument, `{{if .LineStyle}} style="{{.LineStyle}}"{{end}}`, `{{if .NodeStyle}} style="{{.NodeStyle}}"{{end}}`, 1)
	renderDocument = strings.Replace(renderDocument, `{{else}}{{if .Text}}{{.Text}}{{end}}{{range .Children}}`, `{{else}}{{if or (eq .Kind "Text") (eq .Kind "Icon")}}{{if .IconSVG}}<span class="InlineIcon" aria-hidden="true">{{.IconSVG}}</span>{{else if .IconSrc}}<span class="InlineIcon" aria-hidden="true"><img src="{{.IconSrc}}" alt=""></span>{{end}}{{end}}{{if .Text}}{{.Text}}{{end}}{{range .Children}}`, 1)
	tmpl, err := template.New("preview").Parse(renderDocument)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, view); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func buildNode(id string, component map[string]any, surface state.Surface, stack map[string]bool) (node, error) {
	if stack[id] {
		return node{}, fmt.Errorf("component cycle at %s", id)
	}
	stack[id] = true
	defer delete(stack, id)
	kind, _ := component["component"].(string)
	view := node{ID: id, Kind: kind, InputType: "radio"}
	if variant, ok := component["variant"].(string); ok {
		view.Variant = variant
	}
	view.Theme, _ = component["theme"].(string)
	view.Color, _ = component["color"].(string)
	view.Align, _ = component["align"].(string)
	view.Justify, _ = component["justify"].(string)
	view.BackgroundToken, _ = component["backgroundColorToken"].(string)
	if borderToken, ok := component["borderColorToken"].(string); ok && borderToken != "" {
		view.BorderToken = "border-token-" + borderToken
	}
	if colorToken, _ := component["colorToken"].(string); colorToken != "" {
		view.BackgroundToken = strings.TrimSpace(view.BackgroundToken + " token-" + colorToken)
	}
	view.Gap = numberAttribute(component["gap"])
	view.Padding = numberAttribute(component["padding"])
	view.Radius = numberAttribute(component["cornerRadius"])
	view.Weight = numberAttribute(component["weight"])
	view.MaxLine = numberAttribute(component["maxLine"])
	view.MaxHeight = numberAttribute(component["maxHeight"])
	view.DefaultExpanded, _ = component["defaultExpanded"].(bool)
	view.Disabled, _ = component["disabled"].(bool)
	if kind == "Text" {
		if view.MaxLine == "" {
			view.MaxLine = "3"
		}
	}
	if cssColor(component["customLightColor"]) != "" || cssColor(component["customDarkColor"]) != "" {
		view.Variant = strings.TrimSpace(view.Variant + " custom-color")
	}
	if kind == "Icon" {
		view.IconSVG = namedIconSVG(resolve(component["name"], surface.Data))
	}
	if kind == "Text" {
		if icon, ok := component["icon"].(map[string]any); ok {
			if name := resolve(icon["name"], surface.Data); name != "" {
				view.IconSVG = namedIconSVG(name)
			} else if url := resolve(icon["url"], surface.Data); url != "" {
				view.IconSrc = safeImageURL(url)
			}
		}
	}
	view.NodeStyle = componentStyle(component, kind == "Text" || kind == "Icon", view.MaxLine)
	if kind == "CollapsiblePanel" {
		if view.MaxHeight == "" {
			view.MaxHeight = "300"
		}
		if view.MaxHeight != "0" {
			view.PanelStyle = template.CSS("max-height:" + view.MaxHeight + "px") // #nosec G203 -- MaxHeight is a validated finite number.
		}
	}
	if bold, _ := component["bold"].(bool); bold {
		view.Variant = strings.TrimSpace(view.Variant + " bold")
	}
	for _, field := range []string{"text", "content", "title", "fileName", "label", "placeholder", "url"} {
		if value, ok := component[field]; ok {
			if rendered := resolve(value, surface.Data); rendered != "" {
				view.Text = rendered
				break
			}
		}
	}
	if kind == "Markdown" {
		rendered, err := renderMarkdown(view.Text)
		if err != nil {
			return node{}, fmt.Errorf("render Markdown component %s: %w", id, err)
		}
		view.MarkdownHTML = rendered
		view.Text = ""
	}
	if kind == "TextField" {
		view.Label = resolve(component["label"], surface.Data)
		view.Placeholder = resolve(component["placeholder"], surface.Data)
		view.BindingPath = dataBindingPath(component["value"])
		view.Value = resolve(component["value"], surface.Data)
		view.Text = ""
	}
	if kind == "ChoicePicker" {
		view.Label = resolve(component["label"], surface.Data)
		view.DisplayStyle, _ = component["displayStyle"].(string)
		view.BindingPath = dataBindingPath(component["value"])
		view.Value = resolve(component["value"], surface.Data)
		if view.Variant == "multipleSelection" {
			view.InputType = "checkbox"
		}
	}
	if kind == "Link" {
		view.Href = actionURL(component)
	}
	if kind == "Image" {
		view.Align, _ = component["fit"].(string)
		view.ImageSrc = safeImageURL(view.Text)
		view.PreviewEnabled = true
		if enabled, ok := component["previewEnabled"].(bool); ok {
			view.PreviewEnabled = enabled
		}
		if accessibility, ok := component["accessibility"].(map[string]any); ok {
			view.Alt = resolve(accessibility["label"], surface.Data)
		}
	}
	if kind == "File" {
		view.Description = resolve(component["description"], surface.Data)
		view.MimeType = resolve(component["mimeType"], surface.Data)
		view.FileSize = formatFileSize(component["size"])
		view.FileType = fileType(view.Text)
		view.FileMeta = view.Description
		if view.FileMeta == "" {
			view.FileMeta = view.FileSize
			if view.MimeType != "" {
				if view.FileMeta != "" {
					view.FileMeta += " · "
				}
				view.FileMeta += view.MimeType
			}
		}
		if view.FileType == "PDF" {
			view.IconSrc = safeImageURL("https://g.alicdn.com/dingding/card_a2ui/0.1.0/img/ding-file-icons/pdf_light.Dsh5wUh.png")
		}
		view.DownloadHref = resolve(component["url"], surface.Data)
		view.Href = resolve(component["previewUrl"], surface.Data)
		if view.Href == "" {
			view.Href = resolve(component["url"], surface.Data)
		}
		view.PreviewEventName, view.PreviewEventSurfaceID = eventInfoFrom(component["onPreview"])
	}
	if id == "title" {
		view.Variant = strings.TrimSpace(view.Variant + " title")
	}
	if kind == "Button" && view.Variant == "" {
		view.Variant = "default"
	}
	if kind == "Button" {
		view.EventName, view.EventSurfaceID = eventInfo(component)
	}
	if options, ok := component["options"].([]any); ok {
		for _, item := range options {
			if item, ok := item.(map[string]any); ok {
				value := resolve(item["value"], surface.Data)
				view.Options = append(view.Options, option{
					Label:    resolve(item["label"], surface.Data),
					Value:    value,
					Selected: value == view.Value,
				})
			}
		}
	}
	for _, childID := range refs(component) {
		child, ok := surface.Components[childID]
		if !ok {
			return node{}, fmt.Errorf("component %s references missing child %s", id, childID)
		}
		childView, err := buildNode(childID, child, surface, stack)
		if err != nil {
			return node{}, err
		}
		view.Children = append(view.Children, childView)
	}
	return view, nil
}

func safeImageURL(value string) template.URL {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "data:image/png;base64,") || strings.HasPrefix(lower, "data:image/jpeg;base64,") ||
		strings.HasPrefix(lower, "data:image/webp;base64,") || strings.HasPrefix(lower, "data:image/svg+xml;base64,") {
		return template.URL(value) // #nosec G203 -- only explicit image URL schemes are admitted above.
	}
	return ""
}

func dataBindingPath(value any) string {
	binding, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	path, _ := binding["path"].(string)
	return path
}

func eventInfo(component map[string]any) (string, string) {
	return eventInfoFrom(component["action"])
}

func eventInfoFrom(raw any) (string, string) {
	action, ok := raw.(map[string]any)
	if !ok {
		return "", ""
	}
	event, ok := action["event"].(map[string]any)
	if !ok {
		return "", ""
	}
	name, _ := event["name"].(string)
	context, _ := event["context"].(map[string]any)
	surfaceID, _ := context["surfaceId"].(string)
	return name, surfaceID
}

func actionURL(component map[string]any) string {
	action, ok := component["action"].(map[string]any)
	if !ok {
		return ""
	}
	call, ok := action["functionCall"].(map[string]any)
	if !ok {
		return ""
	}
	args, ok := call["args"].(map[string]any)
	if !ok {
		return ""
	}
	url, _ := args["url"].(string)
	return url
}

func componentStyle(component map[string]any, clampText bool, maxLine string) template.CSS {
	styles := make([]string, 0, 7)
	for _, mapping := range [][2]string{
		{"padding", "padding"},
		{"gap", "gap"},
		{"cornerRadius", "border-radius"},
		{"borderWidth", "border-width"},
	} {
		if value := numberAttribute(component[mapping[0]]); value != "" {
			styles = append(styles, mapping[1]+":"+value+"px")
		}
	}
	if color := cssColor(component["backgroundColor"]); color != "" {
		styles = append(styles, "background-color:"+color)
	}
	if color := cssColor(component["borderColor"]); color != "" {
		styles = append(styles, "border-color:"+color, "border-style:solid")
	}
	if clampText {
		light := cssColor(component["customLightColor"])
		dark := cssColor(component["customDarkColor"])
		if light != "" || dark != "" {
			if light == "" {
				light = "currentColor"
			}
			if dark == "" {
				dark = light
			}
			styles = append(styles, "--text-light:"+light, "--text-dark:"+dark, "color:var(--text-light)")
		}
		if size := textSize(component["sizeToken"]); size != "" {
			styles = append(styles, "font-size:"+size)
		}
	}
	if clampText && maxLine != "" {
		styles = append(styles, "-webkit-line-clamp:"+maxLine)
	}
	if len(styles) == 0 {
		return ""
	}
	return template.CSS(strings.Join(styles, ";")) // #nosec G203 -- every value is numeric or a validated hex color.
}

func textSize(value any) string {
	token, _ := value.(string)
	sizes := map[string]string{
		"common_supertitle_text_style__font_size":  "22px",
		"common_largetitle_text_style__font_size":  "20px",
		"common_h1_text_style__font_size":          "18px",
		"common_h2_text_style__font_size":          "16px",
		"common_subhead_text_style__font_size":     "15px",
		"common_body_text_style__font_size":        "15px",
		"common_action_text_style__font_size":      "15px",
		"common_description_text_style__font_size": "13px",
		"common_footnote_text_style__font_size":    "12px",
		"common_tiny_text_style__font_size":        "11px",
	}
	return sizes[token]
}

func namedIconSVG(name string) template.HTML {
	lower := strings.ToLower(name)
	body := `<circle cx="12" cy="12" r="8"/><path d="M12 8v4l3 2"/>`
	switch {
	case strings.Contains(lower, "search"):
		body = `<circle cx="10.5" cy="10.5" r="6.5"/><path d="m15.5 15.5 4 4"/>`
	case strings.Contains(lower, "check"), strings.Contains(lower, "approve"), strings.Contains(lower, "success"):
		body = `<path d="m5 12.5 4.2 4.2L19 7"/>`
	case strings.Contains(lower, "delete"), strings.Contains(lower, "reject"), strings.Contains(lower, "dislike"):
		body = `<path d="M5 7h14M9 7V4h6v3m2 0-1 13H8L7 7m3 4v5m4-5v5"/>`
	case strings.Contains(lower, "edit"):
		body = `<path d="m14.5 5.5 4 4L9 19H5v-4zM13 7l4 4"/>`
	case strings.Contains(lower, "upload"):
		body = `<path d="M12 16V4m-5 5 5-5 5 5M5 20h14"/>`
	case strings.Contains(lower, "download"):
		body = `<path d="M12 4v12m-5-5 5 5 5-5M5 20h14"/>`
	case strings.Contains(lower, "folder"):
		body = `<path d="M3.5 7.5h6l2-2h9v13h-17z"/>`
	case strings.Contains(lower, "picture"), strings.Contains(lower, "image"):
		body = `<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><circle cx="9" cy="10" r="1.5"/><path d="m5 17 4.5-4 3 2.5 2.5-2 4 3.5"/>`
	case strings.Contains(lower, "location"):
		body = `<path d="M12 21s6-5.2 6-11a6 6 0 1 0-12 0c0 5.8 6 11 6 11z"/><circle cx="12" cy="10" r="2"/>`
	case strings.Contains(lower, "people"), strings.Contains(lower, "profile"):
		body = `<circle cx="9" cy="8" r="3"/><path d="M3.5 19c.5-4 2.5-6 5.5-6s5 2 5.5 6M16 8.5a2.5 2.5 0 0 1 0 5m1 1c2 .5 3 2 3.5 4.5"/>`
	case strings.Contains(lower, "document"), strings.Contains(lower, "list"):
		body = `<path d="M6 3.5h8l4 4v13H6zM14 3.5v4h4M9 12h6m-6 4h6"/>`
	case strings.Contains(lower, "comment"):
		body = `<path d="M4 5h16v11H9l-5 4z"/><path d="M8 9h8m-8 3h5"/>`
	case strings.Contains(lower, "phone"):
		body = `<path d="M7 3 4.5 5.5c1.5 7 7 12.5 14 14L21 17l-4-3-2 2c-3-1.5-5.5-4-7-7l2-2z"/>`
	case strings.Contains(lower, "copy"):
		body = `<rect x="8" y="8" width="11" height="12" rx="2"/><path d="M16 8V4H5v12h3"/>`
	case strings.Contains(lower, "like"):
		body = `<path d="M8 10v10H4V10zm0 9h9l3-8-1.5-2H14l.5-4L12 3l-4 7"/>`
	case strings.Contains(lower, "external"):
		body = `<path d="M13 5h6v6m0-6-8 8M19 14v5H5V5h5"/>`
	case strings.Contains(lower, "tool"), strings.Contains(lower, "management"):
		body = `<path d="M14.5 6.5a4 4 0 0 0-5 5L4 17l3 3 5.5-5.5a4 4 0 0 0 5-5l-3 3-3-3z"/>`
	}
	return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" focusable="false">` + body + `</svg>`) // #nosec G203 -- body is selected only from fixed renderer-owned SVG fragments.
}

func cssColor(value any) string {
	raw, ok := value.(string)
	if !ok || len(raw) != 7 && len(raw) != 9 || raw[0] != '#' {
		return ""
	}
	for _, char := range raw[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return ""
		}
	}
	if len(raw) == 9 {
		// A2UI uses #AARRGGBB; CSS uses #RRGGBBAA.
		return "#" + raw[3:] + raw[1:3]
	}
	return raw
}

func numberAttribute(value any) string {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return ""
	}
	return strconv.FormatFloat(number, 'f', -1, 64)
}

func formatFileSize(value any) string {
	size, ok := value.(float64)
	if !ok || size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		return ""
	}
	units := []string{"B", "KB", "MB", "GB"}
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	precision := 0
	if unit > 0 && size < 10 {
		precision = 1
	}
	return strconv.FormatFloat(size, 'f', precision, 64) + " " + units[unit]
}

func fileType(name string) string {
	index := strings.LastIndex(name, ".")
	if index < 0 || index == len(name)-1 {
		return "FILE"
	}
	extension := strings.ToUpper(name[index+1:])
	if len(extension) > 4 {
		return "FILE"
	}
	return extension
}

func renderMarkdown(source string) (template.HTML, error) {
	var out bytes.Buffer
	// Goldmark's default HTML renderer escapes raw HTML and rejects dangerous
	// link schemes. Marking that renderer output as template.HTML prevents a
	// second escaping pass without trusting the original A2UI Markdown string.
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM)).Convert([]byte(source), &out); err != nil {
		return "", err
	}
	return template.HTML(out.String()), nil // #nosec G203 -- output comes from the safe Goldmark renderer.
}

func refs(component map[string]any) []string {
	var out []string
	if child, ok := component["child"].(string); ok {
		out = append(out, child)
	}
	if children, ok := component["children"].([]any); ok {
		for _, child := range children {
			if id, ok := child.(string); ok {
				out = append(out, id)
			}
		}
	}
	if trailing, ok := component["trailing"].(string); ok {
		out = append(out, trailing)
	}
	return out
}

func resolve(value, data any) string {
	if text, ok := value.(string); ok {
		return text
	}
	binding, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	path, ok := binding["path"].(string)
	if !ok {
		return ""
	}
	current := data
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[part]
	}
	return fmt.Sprint(current)
}

func ComponentInventory(surface state.Surface) []string {
	set := map[string]bool{}
	for _, component := range surface.Components {
		if kind, ok := component["component"].(string); ok {
			set[kind] = true
		}
	}
	out := make([]string, 0, len(set))
	for kind := range set {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}
