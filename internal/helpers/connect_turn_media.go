// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var recoverChatRecordUnknownsForConnect = recoverChatRecordUnknowns

// assembleConnectTurnMedia resolves every inbound media locator and builds the
// prompt fragment handed to an agent backend. Keeping this transformation
// separate from the Stream callback makes its failure and fallback behavior
// deterministic and directly testable.
func assembleConnectTurnMedia(prompt, clientID, senderID string, mediaCli connectMediaClient, picCodes []string, fileInfos []fileInboundInfo, chatRecordLookups []chatRecordLookup) (string, []connectMediaAttachment) {
	for _, lookup := range chatRecordLookups {
		lookupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		enrichment, lookupErr := recoverChatRecordUnknownsForConnect(lookupCtx, lookup, callMCPToolReturnTextOnServer)
		cancel()
		if lookupErr != nil {
			fmt.Fprintf(os.Stderr, "[connect][media] unknownMsgType 补拉失败，保留原始 JSON (msgId=%s): %v\n", lookup.MsgID, lookupErr)
			continue
		}
		if strings.TrimSpace(enrichment.Prompt) != "" {
			if strings.TrimSpace(prompt) == "" {
				prompt = enrichment.Prompt
			} else {
				prompt += "\n\n" + enrichment.Prompt
			}
		}
		fileInfos = append(fileInfos, enrichment.Files...)
		fmt.Fprintf(os.Stderr, "[connect][media] unknownMsgType 补拉完成: 原始附件=%d 未定位=%d (msgId=%s)\n", len(enrichment.Files), enrichment.MissingCount, lookup.MsgID)
		if enrichment.MissingCount > 0 {
			prompt += fmt.Sprintf("\n（其中 %d 个转发附件仍未能定位原始文件，请明确告知用户未读取到这些附件。）", enrichment.MissingCount)
		}
	}
	var attachments []connectMediaAttachment
	for i, picCode := range picCodes {
		localPath, err := mediaCli.downloadMessageFile(context.Background(), clientID, picCode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[connect][media] 第 %d 张图片下载失败: %v\n", i+1, err)
			if prompt == "" {
				prompt = "（用户发来图片，但图片下载失败了。请告知用户图片没收到，建议补充文字描述。）"
			} else {
				prompt += "\n（用户同时附了一张图片，但图片下载失败，请基于现有文字回答并说明未能读取图片。）"
			}
			continue
		}
		if prompt == "" {
			prompt = "用户发来一张图片（本地路径 " + localPath + "），请查看图片内容并回答其中的问题。"
		} else {
			prompt += "\n（用户同时附了一张图片，本地路径 " + localPath + "，请结合图片内容回答。）"
		}
		attachments = append(attachments, connectMediaAttachment{LocalPath: localPath, FileName: filepath.Base(localPath), MediaType: "image"})
	}
	for i, fileInfo := range fileInfos {
		if !fileInfo.hasActionable() {
			continue
		}
		fileName := fileInfo.FileName
		mediaType := inboundMediaType(fileInfo.MediaType)
		mediaLabel := "文件"
		successPrompt := "请读取文件内容并回答"
		switch mediaType {
		case "image":
			mediaLabel = "图片"
			successPrompt = "请查看图片内容并回答"
		case "audio":
			mediaLabel = "语音"
			successPrompt = "请听取或转写语音内容并回答"
		case "video":
			mediaLabel = "视频"
			successPrompt = "请查看并分析视频内容后回答"
		}
		var localPath string
		var err error
		if fileInfo.DownloadCode != "" {
			localPath, err = mediaCli.downloadMessageFileNamed(context.Background(), clientID, fileInfo.DownloadCode, fileName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[connect][media] 第 %d 个%s下载失败: %v\n", i+1, mediaLabel, err)
			}
		} else if fileInfo.MediaID != "" || fileInfo.FileID != "" {
			downloadCtx, cancel := context.WithTimeout(context.Background(), mediaDownloadTimeout)
			localPath, err = mediaCli.downloadRecoveredChatRecordFile(downloadCtx, fileInfo)
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[connect][media] 第 %d 个转发%s原始内容下载失败: %v\n", i+1, mediaLabel, err)
			}
		}
		switch {
		case localPath != "":
			fmt.Fprintf(os.Stderr, "[connect][media] 第 %d 个%s已完整下载: %s\n", i+1, mediaLabel, localPath)
			attachments = append(attachments, connectMediaAttachment{LocalPath: localPath, FileName: fileName, MediaType: mediaType})
			if prompt == "" {
				prompt = "用户发来一个" + mediaLabel + "「" + fileName + "」（本地路径 " + localPath + "），" + successPrompt + "。"
			} else {
				prompt += "\n（用户同时附了一个" + mediaLabel + "「" + fileName + "」，本地路径 " + localPath + "，" + successPrompt + "。）"
			}
		case fileInfo.DentryID != 0 && fileInfo.SpaceID != 0:
			if unionID, unionErr := mediaCli.getUserUnionID(context.Background(), strings.TrimSpace(senderID)); unionErr != nil {
				fmt.Fprintf(os.Stderr, "[connect][media] userId→unionId 失败 (%s): %v\n", senderID, unionErr)
			} else if downloadedPath, downloadErr := mediaCli.downloadDentryFile(context.Background(), fileInfo.SpaceID, fileInfo.DentryID, unionID, fileName); downloadErr != nil {
				fmt.Fprintf(os.Stderr, "[connect][media] 钉盘文件下载失败 spaceId=%d dentryId=%d: %v\n", fileInfo.SpaceID, fileInfo.DentryID, downloadErr)
			} else {
				localPath = downloadedPath
			}
			if localPath != "" {
				fmt.Fprintf(os.Stderr, "[connect][media] 第 %d 个%s已完整下载: %s\n", i+1, mediaLabel, localPath)
				attachments = append(attachments, connectMediaAttachment{LocalPath: localPath, FileName: fileName, MediaType: mediaType})
				if prompt == "" {
					prompt = "用户发来一个" + mediaLabel + "「" + fileName + "」（本地路径 " + localPath + "），" + successPrompt + "。"
				} else {
					prompt += "\n（用户同时附了一个" + mediaLabel + "「" + fileName + "」，本地路径 " + localPath + "，" + successPrompt + "。）"
				}
			} else {
				meta := fmt.Sprintf("文件名「%s」，dentryId=%d，spaceId=%d", fileName, fileInfo.DentryID, fileInfo.SpaceID)
				if fileInfo.FileType != "" {
					meta += "，类型=" + fileInfo.FileType
				}
				if fileInfo.FileSize > 0 {
					meta += fmt.Sprintf("，大小=%d 字节", fileInfo.FileSize)
				}
				if prompt == "" {
					prompt = "用户发来一个文件（" + meta + "）。文件下载失败，请基于文件名与用户随附的文字信息回答，必要时请用户改用客户端上传或补充文字描述。"
				} else {
					prompt += "\n（用户同时附了一个文件：" + meta + "。文件下载失败，请结合文件名与随附文字回答。）"
				}
			}
		default:
			failure := "用户发来一个" + mediaLabel + "「" + fileName + "」，但原始内容下载失败。请明确告知用户该附件未能读取，建议重新发送或补充文字描述。"
			if prompt == "" {
				prompt = "（" + failure + "）"
			} else {
				prompt += "\n（" + failure + "）"
			}
		}
	}
	return prompt, attachments
}
