// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

const (
	appFixtureCurrentDOpenID  = "DAAAAAAAAAAAiE"
	appFixtureCurrentDOpenID2 = "DAQEBAQEBAQEiE"
)

// paramAliasCompleteCommands is deliberately keyed by the exact reviewed
// fixture command path. Every argv is a complete, business-valid invocation:
// required companion flags are present, time and enum values are valid, and
// write commands use the capture caller rather than a real transport. The
// target canonical flag must occur exactly once so the test can replace only
// its spelling while holding every other input constant.
var paramAliasCompleteCommands = map[string][]string{
	"aitable +base-search":                     {"aitable", "+base-search", "--query", "fixture"},
	"aitable +field-get":                       {"aitable", "+field-get", "--base-id", "base-1", "--table-id", "table-1"},
	"aitable +list-tables":                     {"aitable", "+list-tables", "--base", "base-1"},
	"aitable +record-query":                    {"aitable", "+record-query", "--base-id", "base-1", "--table-id", "table-1", "--query", "fixture"},
	"aitable +record-share-url":                {"aitable", "+record-share-url", "--base-id", "base-1", "--table-id", "table-1", "--record-ids", "record-1"},
	"aitable +table-get":                       {"aitable", "+table-get", "--base-id", "base-1"},
	"aitable record query":                     {"aitable", "record", "query", "--base-id", "base-1", "--table-id", "table-1", "--limit", "7"},
	"attendance check result":                  {"attendance", "check", "result", "--users", "user-1,user-2", "--start", "2026-03-01", "--end", "2026-03-02"},
	"attendance +check-result":                 {"attendance", "+check-result", "--users", "user-1,user-2", "--start", "2026-03-01", "--end", "2026-03-02"},
	"calendar event list":                      {"calendar", "event", "list", "--start", "2026-03-10T14:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--calendar-id", "primary", "--cursor", "cursor-1", "--limit", "7"},
	"chat +chat-messages":                      {"chat", "+chat-messages", "--group", "fixture-conversation"},
	"chat +chat-add-bot":                       {"chat", "+chat-add-bot", "--id", "fixture-conversation", "--robot-code", "robot-1", "--yes"},
	"chat +chat-audit-join":                    {"chat", "+chat-audit-join", "--group", "fixture-conversation", "--record-id", "7", "--applicant", "user-1", "--inviter", "user-2", "--status", "AuditApprove", "--yes"},
	"chat +chat-members-get":                   {"chat", "+chat-members-get", "--id", "fixture-conversation", "--users", appFixtureCurrentDOpenID + "," + appFixtureCurrentDOpenID2},
	"chat +chat-members-list":                  {"chat", "+chat-members-list", "--conversation-id", "fixture-conversation", "--member-types", "user,bot"},
	"chat +chat-mute-member":                   {"chat", "+chat-mute-member", "--group", "fixture-conversation", "--users", appFixtureCurrentDOpenID + "," + appFixtureCurrentDOpenID2, "--mute-time", "3600000", "--yes"},
	"chat +chat-remove-bot":                    {"chat", "+chat-remove-bot", "--id", "fixture-conversation", "--bot-id", "bot-1", "--yes"},
	"chat +chat-role-remove-user":              {"chat", "+chat-role-remove-user", "--group", "fixture-conversation", "--user", appFixtureCurrentDOpenID, "--role-ids", "role-1", "--yes"},
	"chat +chat-transfer-owner":                {"chat", "+chat-transfer-owner", "--group", "fixture-conversation", "--new-owner", appFixtureCurrentDOpenID, "--yes"},
	"chat +chat-update":                        {"chat", "+chat-update", "--group", "fixture-conversation", "--name", "Fixture Renamed Group", "--yes"},
	"chat +bot-find":                           {"chat", "+bot-find", "--query", "fixture", "--limit", "7"},
	"chat +bot-search":                         {"chat", "+bot-search", "--name", "Fixture Bot", "--page", "2", "--size", "7"},
	"chat +category-create":                    {"chat", "+category-create", "--title", "Fixture Cat", "--yes"},
	"chat +category-rename":                    {"chat", "+category-rename", "--category-id", "7", "--title", "Renamed Cat", "--yes"},
	"chat +group-members":                      {"chat", "+group-members", "--group", "Fixture Group"},
	"chat +conversation-set-top":               {"chat", "+conversation-set-top", "--conversation-id", "fixture-conversation", "--yes"},
	"chat +feed-group-query-item":              {"chat", "+feed-group-query-item", "--category-id", "7", "--conversation-ids", "fixture-conversation"},
	"chat +flag-cancel":                        {"chat", "+flag-cancel", "--conversation-id", "fixture-conversation", "--message-id", "message-1", "--yes"},
	"chat +flag-create":                        {"chat", "+flag-create", "--conversation-id", "fixture-conversation", "--message-id", "message-1", "--yes"},
	"chat +flag-list":                          {"chat", "+flag-list", "--cursor", "0", "--page-size", "7"},
	"chat +messages-combine-forward":           {"chat", "+messages-combine-forward", "--src-conversation-id", "fixture-source", "--msg-ids", "message-1,message-2", "--dest-conversation-id", "fixture-destination", "--yes"},
	"chat +messages-forward":                   {"chat", "+messages-forward", "--src-conversation-id", "fixture-source", "--msg-id", "message-1", "--dest-conversation-id", "fixture-destination", "--yes"},
	"chat +messages-forward-topic":             {"chat", "+messages-forward-topic", "--src-msg-id", "message-1", "--src-conversation-id", "fixture-source", "--src-thread-id", "convThread-fixture", "--dest-conversation-id", "fixture-destination", "--yes"},
	"chat +messages-list":                      {"chat", "+messages-list", "--group", "fixture-conversation", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	"chat +messages-list-direct":               {"chat", "+messages-list-direct", "--user", "user-1", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	"chat +messages-list-unread-conversations": {"chat", "+messages-list-unread-conversations", "--count", "7", "--exclude-muted"},
	"chat +messages-reply":                     {"chat", "+messages-reply", "--conversation-id", "fixture-conversation", "--ref-msg-id", "message-1", "--ref-sender", appFixtureCurrentDOpenID, "--text", "hello fixture", "--yes"},
	"chat +messages-resource-download":         {"chat", "+messages-resource-download", "--resource-id", "resource-1", "--message-id", "message-1", "--open-conversation-id", "fixture-conversation", "--output", "downloads/fixture.bin"},
	"chat +messages-set-pin":                   {"chat", "+messages-set-pin", "--open-conversation-id", "fixture-conversation", "--msg-id", "message-1", "--yes"},
	"chat +messages-send-by-webhook":           {"chat", "+messages-send-by-webhook", "--token", "fixture-token", "--title", "Fixture Alert", "--text", "fixture", "--at-users", "user-1,user-2", "--yes"},
	"chat +search-msg":                         {"chat", "+search-msg", "--group", "fixture-conversation", "--query", "fixture", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-11T00:00:00+08:00", "--no-enrich"},
	"chat +send-to-group":                      {"chat", "+send-to-group", "--group", "Fixture Group", "--text", "hello fixture", "--yes"},
	"chat +unread-chats":                       {"chat", "+unread-chats", "--count", "7", "--exclude-muted"},
	"chat bot find":                            {"chat", "bot", "find", "--query", "fixture", "--limit", "7"},
	"chat bot search":                          {"chat", "bot", "search", "--name", "Fixture Bot", "--page", "2", "--size", "7"},
	"chat category create":                     {"chat", "category", "create", "--title", "Fixture Cat", "--yes"},
	"chat category create-smart":               {"chat", "category", "create-smart", "--name", "Fixture Smart Category", "--keywords", "fixture,priority", "--yes"},
	"chat category rename":                     {"chat", "category", "rename", "--category-id", "7", "--title", "Renamed Cat", "--yes"},
	"chat group members":                       {"chat", "group", "members", "--id", "fixture-conversation"},
	"chat group members add":                   {"chat", "group", "members", "add", "--id", "fixture-conversation", "--users", appFixtureCurrentDOpenID},
	"chat group members add-bot":               {"chat", "group", "members", "add-bot", "--id", "fixture-conversation", "--robot-code", "robot-1", "--yes"},
	"chat group members list-by-ids":           {"chat", "group", "members", "list-by-ids", "--id", "fixture-conversation", "--users", appFixtureCurrentDOpenID + "," + appFixtureCurrentDOpenID2},
	"chat group members remove":                {"chat", "group", "members", "remove", "--id", "fixture-conversation", "--users", appFixtureCurrentDOpenID, "--yes"},
	"chat group members remove-bot":            {"chat", "group", "members", "remove-bot", "--id", "fixture-conversation", "--bot-id", "bot-1", "--yes"},
	"chat group rename":                        {"chat", "group", "rename", "--id", "fixture-conversation", "--name", "Fixture Renamed Group", "--yes"},
	"chat group set-admin":                     {"chat", "group", "set-admin", "--group", "fixture-conversation", "--user", "user-1", "--yes"},
	"chat message add-emoji":                   {"chat", "message", "add-emoji", "--conversation-id", "fixture-conversation", "--msg-id", "message-1", "--emoji", "赞", "--yes"},
	"chat message add-favorite":                {"chat", "message", "add-favorite", "--open-message-id", "message-1", "--open-conversation-id", "fixture-conversation", "--yes"},
	"chat message combine-forward":             {"chat", "message", "combine-forward", "--src-conversation-id", "fixture-source", "--msg-ids", "message-1,message-2", "--dest-conversation-id", "fixture-destination", "--yes"},
	"chat message forward-topic":               {"chat", "message", "forward-topic", "--src-msg-id", "message-1", "--src-conversation-id", "fixture-source", "--src-thread-id", "convThread-fixture", "--dest-conversation-id", "fixture-destination", "--yes"},
	"chat message list":                        {"chat", "message", "list", "--group", "fixture-conversation", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	"chat message list-all":                    {"chat", "message", "list-all", "--start", "2026-03-10 00:00:00", "--end", "2026-03-11 00:00:00"},
	"chat message list-by-sender":              {"chat", "message", "list-by-sender", "--sender-user-id", "user-1", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-11T00:00:00+08:00", "--limit", "7", "--cursor", "0"},
	"chat message list-favorites":              {"chat", "message", "list-favorites", "--cursor", "2", "--size", "7"},
	"chat message list-by-ids":                 {"chat", "message", "list-by-ids", "--msg-ids", "message-1,message-2"},
	"chat message list-unread-conversations":   {"chat", "message", "list-unread-conversations", "--count", "7", "--exclude-muted"},
	"chat message recall":                      {"chat", "message", "recall", "--conversation-id", "fixture-conversation", "--msg-id", "message-1", "--yes"},
	"chat message reply":                       {"chat", "message", "reply", "--conversation-id", "fixture-conversation", "--ref-msg-id", "message-1", "--ref-sender", appFixtureCurrentDOpenID, "--text", "hello fixture", "--yes"},
	"chat message search-advanced":             {"chat", "message", "search-advanced", "--conversation-ids", "fixture-conversation", "--query", "fixture"},
	"chat message send":                        {"chat", "message", "send", "--user", appFixtureCurrentDOpenID, "--text", "hello fixture", "--idempotency-key", "param-alias-equivalence", "--yes"},
	"chat message send-by-bot":                 {"chat", "message", "send-by-bot", "--robot-code", "robot-1", "--group", "fixture-conversation", "--title", "Fixture Alert", "--text", "@user-1 @user-2 fixture", "--at-user-ids", "user-1,user-2", "--yes"},
	"chat message send-by-webhook":             {"chat", "message", "send-by-webhook", "--token", "fixture-token", "--title", "Fixture Alert", "--text", "fixture", "--at-users", "user-1,user-2", "--yes"},
	"contact +dept-members":                    {"contact", "+dept-members", "--dept", "Fixture Dept"},
	"contact +list-sub-depts":                  {"contact", "+list-sub-depts", "--dept", "1"},
	"contact +resolve-dept":                    {"contact", "+resolve-dept", "--name", "Fixture Dept"},
	"contact +search-user":                     {"contact", "+search-user", "--query", "Fixture User"},
	"contact dept list-children":               {"contact", "dept", "list-children", "--dept", "1"},
	"contact user profile get":                 {"contact", "user", "profile", "get", "--staff-id", "user-1"},
	"dev app get":                              {"dev", "app", "get", "--unified-app-id", "app-1"},
	"devdoc article search":                    {"devdoc", "article", "search", "--query", "fixture", "--page", "2", "--size", "7"},
	"ding +receiver-status":                    {"ding", "+receiver-status", "--ding-id", "ding-1"},
	"ding message receiver-status":             {"ding", "message", "receiver-status", "--ding-id", "ding-1"},
	"ding message send":                        {"ding", "message", "send", "--robot-code", "robot-1", "--content", "fixture", "--users", "user-1", "--yes"},
	"doc +comment-create":                      {"doc", "+comment-create", "--node", "node-1", "--content", "fixture comment", "--yes"},
	"doc +comment-list":                        {"doc", "+comment-list", "--node", "node-1", "--limit", "7", "--cursor", "cursor-1"},
	"doc +comment-reply":                       {"doc", "+comment-reply", "--node", "node-1", "--comment-key", "comment-1", "--content", "fixture reply", "--yes"},
	"doc +access-grant":                        {"doc", "+access-grant", "--node", "node-1", "--to", "Fixture User", "--role", "READER", "--workspace", "workspace-1", "--yes"},
	"doc +copy":                                {"doc", "+copy", "--node", "node-1", "--workspace", "workspace-1", "--yes"},
	"doc +create":                              {"doc", "+create", "--name", "Fixture Document", "--content", "fixture body", "--doc-format", "markdown"},
	"doc +create-from-template":                {"doc", "+create-from-template", "--query", "fixture template", "--name", "Fixture From Template", "--folder", "folder-1", "--workspace", "workspace-1"},
	"doc +doc-append":                          {"doc", "+doc-append", "--doc", "node-1", "--text", "fixture appendix", "--yes"},
	"doc +export-submit":                       {"doc", "+export-submit", "--node", "node-1", "--export-format", "docx"},
	"doc +fetch":                               {"doc", "+fetch", "--node", "node-1", "--scope", "section", "--start-block-id", "block-1"},
	"doc +find-doc":                            {"doc", "+find-doc", "--query", "fixture", "--limit", "7"},
	"doc +history-revert":                      {"doc", "+history-revert", "--node", "node-1", "--version", "3", "--yes"},
	"doc +inspect":                             {"doc", "+inspect", "--node", "node-1", "--include-history"},
	"doc +list":                                {"doc", "+list", "--folder", "folder-1", "--cursor", "cursor-1"},
	"doc +move":                                {"doc", "+move", "--node", "node-1", "--folder", "folder-1", "--yes"},
	"doc +search":                              {"doc", "+search", "--query", "fixture", "--limit", "7", "--cursor", "cursor-1"},
	"doc +template-list":                       {"doc", "+template-list", "--source", "MY", "--limit", "7", "--cursor", "cursor-1"},
	"doc +template-search":                     {"doc", "+template-search", "--query", "fixture", "--source", "MY", "--limit", "7"},
	"doc +version-list":                        {"doc", "+version-list", "--node", "node-1", "--limit", "7", "--cursor", "cursor-1"},
	"doc +version-revert":                      {"doc", "+version-revert", "--node", "node-1", "--version", "3", "--yes"},
	"doc +version-save":                        {"doc", "+version-save", "--node", "node-1", "--yes"},
	"doc +update":                              {"doc", "+update", "--node", "node-1", "--command", "overwrite", "--content", `["root",{}]`, "--doc-format", "jsonml", "--expected-revision", "1", "--yes"},
	"doc +export":                              {"doc", "+export", "--node", "node-1", "--export-format", "docx", "--output", "exports/fixture.docx"},
	"doc block insert":                         {"doc", "block", "insert", "--node", "node-1", "--text", "fixture paragraph", "--yes"},
	"doc block update":                         {"doc", "block", "update", "--node", "node-1", "--block-id", "block-1", "--text", "fixture paragraph", "--yes"},
	"doc comment create":                       {"doc", "comment", "create", "--node", "node-1", "--content", "fixture comment", "--yes"},
	"doc comment create-inline":                {"doc", "comment", "create-inline", "--node", "node-1", "--block-id", "block-1", "--start", "0", "--end", "7", "--content", "fixture comment", "--yes"},
	"doc comment delete":                       {"doc", "comment", "delete", "--node", "node-1", "--comment-key", "comment-1", "--yes"},
	"doc comment reply":                        {"doc", "comment", "reply", "--node", "node-1", "--comment-key", "comment-1", "--content", "fixture reply", "--mentioned-open-conversation-id", "cid-1,cid-2", "--yes"},
	"doc comment update":                       {"doc", "comment", "update", "--node", "node-1", "--comment-key", "comment-1", "--content", "fixture update", "--yes"},
	"doc create":                               {"doc", "create", "--name", "Fixture Document", "--workspace", "workspace-1"},
	"doc version revert":                       {"doc", "version", "revert", "--node", "node-1", "--version", "3", "--yes"},
	"drive +cover":                             {"drive", "+cover", "--node", "node-1"},
	"drive +create-folder":                     {"drive", "+create-folder", "--name", "Fixture Folder", "--space-id", "space-1", "--folder", "folder-1"},
	"drive +create-shortcut":                   {"drive", "+create-shortcut", "--node", "node-1", "--folder", "folder-1", "--workspace", "workspace-1"},
	"drive +delete":                            {"drive", "+delete", "--node", "node-1", "--yes"},
	"drive +download":                          {"drive", "+download", "--node", "node-1", "--space-id", "space-1", "--output", "downloads/fixture.bin"},
	"drive +info":                              {"drive", "+info", "--node", "node-1", "--space-id", "space-1"},
	"drive +inspect":                           {"drive", "+inspect", "--node", "node-1", "--space-id", "space-1", "--include-stats"},
	"drive +list":                              {"drive", "+list", "--space-id", "space-1", "--folder", "folder-1", "--limit", "7", "--cursor", "cursor-1", "--order-by", "name", "--order", "asc"},
	"drive +publish-get":                       {"drive", "+publish-get", "--node", "node-1"},
	"drive +publish-unset":                     {"drive", "+publish-unset", "--node", "node-1", "--yes"},
	"drive +recycle-list":                      {"drive", "+recycle-list", "--space-id", "space-1", "--limit", "7", "--cursor", "cursor-1"},
	"drive +recycle-restore":                   {"drive", "+recycle-restore", "--id", "recycle-1", "--yes"},
	"drive +rename":                            {"drive", "+rename", "--node", "node-1", "--name", "Fixture Renamed", "--yes"},
	"drive +search":                            {"drive", "+search", "--query", "fixture"},
	"drive +star-add":                          {"drive", "+star-add", "--node", "node-1"},
	"drive +star-list":                         {"drive", "+star-list", "--limit", "7", "--cursor", "cursor-1"},
	"drive +star-remove":                       {"drive", "+star-remove", "--node", "node-1"},
	"drive +stats":                             {"drive", "+stats", "--node", "node-1"},
	"drive +upload":                            {"drive", "+upload", "--file", "param_alias_payload_equivalence_test.go", "--file-name", "fixture.txt", "--mime-type", "text/plain", "--space-id", "space-1", "--node", "node-1", "--yes"},
	"drive +version-download":                  {"drive", "+version-download", "--node", "node-1", "--version", "3", "--output", "downloads/fixture-v3.bin"},
	"drive +version-get":                       {"drive", "+version-get", "--node", "node-1", "--version", "3"},
	"drive +version-history":                   {"drive", "+version-history", "--node", "node-1", "--limit", "7", "--cursor", "cursor-1"},
	"drive +version-revert":                    {"drive", "+version-revert", "--node", "node-1", "--version", "3", "--yes"},
	"drive commit":                             {"drive", "commit", "--file-name", "fixture.txt", "--file-size", "7", "--upload-id", "upload-1", "--space-id", "space-1"},
	"drive copy":                               {"drive", "copy", "--node", "node-1", "--folder", "folder-1"},
	"drive download":                           {"drive", "download", "--node", "node-1", "--output", "downloads/fixture.bin", "--version", "3", "--space-id", "space-1"},
	"drive info":                               {"drive", "info", "--node", "node-1", "--space-id", "space-1"},
	"drive list":                               {"drive", "list", "--folder", "folder-1", "--limit", "7"},
	"drive mkdir":                              {"drive", "mkdir", "--name", "Fixture Folder", "--space-id", "space-1"},
	"drive permission add":                     {"drive", "permission", "add", "--node", "node-1", "--users", "user-1,user-2", "--role", "READER"},
	"drive recycle list":                       {"drive", "recycle", "list", "--space-id", "space-1", "--limit", "7"},
	"drive recycle restore":                    {"drive", "recycle", "restore", "--id", "recycle-1"},
	"drive search":                             {"drive", "search", "--query", "fixture", "--created-from", "1", "--created-to", "2", "--modified-from", "3", "--modified-to", "4", "--creator-uids", "user-1,user-2"},
	"drive upload":                             {"drive", "upload", "--file", "../../go.mod", "--space-id", "space-1"},
	"drive upload-info":                        {"drive", "upload-info", "--file-name", "fixture.txt", "--file-size", "7", "--space-id", "space-1"},
	"mail +find-mail-user":                     {"mail", "+find-mail-user", "--query", "fixture", "--limit", "7"},
	"mail folder update":                       {"mail", "folder", "update", "--email", "fixture@example.com", "--id", "folder-1", "--name", "Fixture Folder", "--yes"},
	"mail message search":                      {"mail", "message", "search", "--email", "fixture@example.com", "--query", "subject:fixture"},
	"mail thread list":                         {"mail", "thread", "list", "--email", "fixture@example.com", "--folder", "folder-1", "--limit", "7"},
	"mail user search":                         {"mail", "user", "search", "--keyword", "fixture"},
	"oa +list-executed":                        {"oa", "+list-executed", "--limit", "7", "--page", "1"},
	"oa +search-forms":                         {"oa", "+search-forms", "--query", "fixture"},
	"oa approval search-forms":                 {"oa", "approval", "search-forms", "--query", "fixture"},
	"report list":                              {"report", "list", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-10T23:59:59+08:00"},
}

// A command can expose more than one mutually exclusive canonical route. In
// that case the shared command template above cannot contain every canonical
// flag at once, so select a fixture-specific complete invocation here.
var paramAliasCompleteCommandVariants = map[string]map[string][]string{
	"doc +copy": {
		"folder":    {"doc", "+copy", "--node", "node-1", "--folder", "folder-1", "--yes"},
		"workspace": {"doc", "+copy", "--node", "node-1", "--workspace", "workspace-1", "--yes"},
	},
	"doc block insert": {
		"parent-block": {"doc", "block", "insert", "--node", "node-1", "--parent-block", "parent-block-1", "--index", "0", "--text", "fixture paragraph", "--yes"},
	},
	"doc +inspect": {
		"include-permissions": {"doc", "+inspect", "--node", "node-1", "--include-permissions"},
	},
	"doc +search": {
		"created-from": {"doc", "+search", "--query", "fixture", "--created-from", "1"},
		"created-to":   {"doc", "+search", "--query", "fixture", "--created-to", "2"},
		"creator-uids": {"doc", "+search", "--query", "fixture", "--creator-uids", "user-1,user-2"},
	},
	"drive list": {
		"workspace": {"drive", "list", "--workspace", "workspace-1", "--limit", "7"},
		"order-by":  {"drive", "list", "--folder", "folder-1", "--order-by", "name", "--limit", "7"},
		"space-id":  {"drive", "list", "--space-id", "space-1", "--limit", "7"},
		"order":     {"drive", "list", "--folder", "folder-1", "--order", "asc", "--limit", "7"},
	},
	"chat message list": {
		"user": {"chat", "message", "list", "--user", "user-1", "--time", "2026-03-10 00:00:00", "--limit", "7"},
	},
	"chat message list-by-sender": {
		"sender-open-dingtalk-id": {"chat", "message", "list-by-sender", "--sender-open-dingtalk-id", appFixtureCurrentDOpenID, "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-11T00:00:00+08:00", "--limit", "7", "--cursor", "0"},
	},
	"chat message send": {
		"group":     {"chat", "message", "send", "--group", "fixture-conversation", "--text", "hello fixture", "--idempotency-key", "param-alias-equivalence-group", "--yes"},
		"file-path": {"chat", "message", "send", "--group", "fixture-conversation", "--msg-type", "file", "--file-path", "../../go.mod", "--dentry-id", "1", "--space-id", "2", "--idempotency-key", "param-alias-equivalence-file", "--yes"},
	},
	"chat +conversation-set-top": {
		"conversation-ids": {"chat", "+conversation-set-top", "--conversation-ids", "fixture-conversation-1,fixture-conversation-2", "--yes"},
	},
}

// paramAliasNewIMCases is the exact set of aliases added by the reviewed IM
// optimization. The dedicated gate below requires every one to remain active
// in the embedded generated table and equivalent at the final transport.
var paramAliasNewIMCases = []struct {
	command   string
	emitted   string
	canonical string
}{
	{command: "chat +chat-messages", emitted: "chat", canonical: "group"},
	{command: "chat +bot-find", emitted: "name", canonical: "query"},
	{command: "chat bot find", emitted: "name", canonical: "query"},
	{command: "chat +bot-search", emitted: "query", canonical: "name"},
	{command: "chat +bot-search", emitted: "current-page", canonical: "page"},
	{command: "chat +category-create", emitted: "name", canonical: "title"},
	{command: "chat +category-rename", emitted: "name", canonical: "title"},
	{command: "chat +messages-list-direct", emitted: "start", canonical: "time"},
	{command: "chat +messages-list-unread-conversations", emitted: "limit", canonical: "count"},
	{command: "chat +messages-list-unread-conversations", emitted: "size", canonical: "count"},
	{command: "chat +messages-send-by-webhook", emitted: "at-user-ids", canonical: "at-users"},
	{command: "chat +search-msg", emitted: "chat", canonical: "group"},
	{command: "chat +unread-chats", emitted: "limit", canonical: "count"},
	{command: "chat +unread-chats", emitted: "size", canonical: "count"},
	{command: "chat bot search", emitted: "query", canonical: "name"},
	{command: "chat bot search", emitted: "current-page", canonical: "page"},
	{command: "chat category create", emitted: "name", canonical: "title"},
	{command: "chat category create-smart", emitted: "title", canonical: "name"},
	{command: "chat category rename", emitted: "name", canonical: "title"},
	{command: "chat message list", emitted: "start", canonical: "time"},
	{command: "chat message list-by-sender", emitted: "user-id", canonical: "sender-user-id"},
	{command: "chat message list-by-sender", emitted: "open-dingtalk-id", canonical: "sender-open-dingtalk-id"},
	{command: "chat message list-favorites", emitted: "limit", canonical: "size"},
	{command: "chat message list-unread-conversations", emitted: "limit", canonical: "count"},
	{command: "chat message list-unread-conversations", emitted: "size", canonical: "count"},
	{command: "chat message send", emitted: "file", canonical: "file-path"},
	{command: "chat message send-by-bot", emitted: "at-users", canonical: "at-user-ids"},
	{command: "chat message send-by-webhook", emitted: "at-user-ids", canonical: "at-users"},
	{command: "chat +chat-update", emitted: "chat-id", canonical: "group"},
	{command: "chat +chat-update", emitted: "conversation-id", canonical: "group"},
	{command: "chat +chat-update", emitted: "open-conversation-id", canonical: "group"},
	{command: "chat +chat-update", emitted: "title", canonical: "name"},
	{command: "chat +chat-update", emitted: "new-title", canonical: "name"},
	{command: "chat +flag-list", emitted: "limit", canonical: "page-size"},
	{command: "chat +chat-members-list", emitted: "chat-id", canonical: "conversation-id"},
	{command: "chat +chat-members-list", emitted: "id", canonical: "conversation-id"},
	{command: "chat +conversation-set-top", emitted: "open-conversation-id", canonical: "conversation-id"},
	{command: "chat +conversation-set-top", emitted: "chat-ids", canonical: "conversation-ids"},
	{command: "chat +chat-members-get", emitted: "conversation-id", canonical: "id"},
	{command: "chat +chat-members-get", emitted: "open-dingtalk-ids", canonical: "users"},
	{command: "chat +chat-members-get", emitted: "chat", canonical: "id"},
	{command: "chat +messages-list", emitted: "start", canonical: "time"},
	{command: "chat +messages-reply", emitted: "msg-id", canonical: "ref-msg-id"},
	{command: "chat +messages-reply", emitted: "chat", canonical: "conversation-id"},
	{command: "chat +flag-cancel", emitted: "group", canonical: "conversation-id"},
	{command: "chat +flag-cancel", emitted: "chat", canonical: "conversation-id"},
	{command: "chat +flag-create", emitted: "group", canonical: "conversation-id"},
	{command: "chat +chat-add-bot", emitted: "conversation-id", canonical: "id"},
	{command: "chat +chat-add-bot", emitted: "robot", canonical: "robot-code"},
	{command: "chat +chat-audit-join", emitted: "applicant-user-id", canonical: "applicant"},
	{command: "chat +chat-mute-member", emitted: "user-ids", canonical: "users"},
	{command: "chat +chat-remove-bot", emitted: "open-bot-id", canonical: "bot-id"},
	{command: "chat +chat-role-remove-user", emitted: "open-dingtalk-id", canonical: "user"},
	{command: "chat +chat-transfer-owner", emitted: "user-id", canonical: "new-owner"},
	{command: "chat +feed-group-query-item", emitted: "chat-ids", canonical: "conversation-ids"},
	{command: "chat +messages-combine-forward", emitted: "src-open-cid", canonical: "src-conversation-id"},
	{command: "chat +messages-forward", emitted: "source-message-id", canonical: "msg-id"},
	{command: "chat +messages-forward-topic", emitted: "src-open-message-id", canonical: "src-msg-id"},
	{command: "chat +messages-resource-download", emitted: "conversation-id", canonical: "open-conversation-id"},
	{command: "chat +messages-set-pin", emitted: "conversation-id", canonical: "open-conversation-id"},
}

// paramAliasNewDriveCases is the exact executable-alias set introduced by the
// reviewed Drive expansion. Guard fixtures are covered separately by the
// exhaustive runtime-contract tests; every entry here must preserve the final
// transport payload of its canonical spelling.
var paramAliasNewDriveCases = []struct {
	command   string
	emitted   string
	canonical string
}{
	{command: "drive +cover", emitted: "dentry-uuid", canonical: "node"},
	{command: "drive +create-folder", emitted: "folder-name", canonical: "name"},
	{command: "drive +create-folder", emitted: "storage-space-id", canonical: "space-id"},
	{command: "drive +create-shortcut", emitted: "source-file-id", canonical: "node"},
	{command: "drive +create-shortcut", emitted: "target-folder-id", canonical: "folder"},
	{command: "drive +create-shortcut", emitted: "target-workspace-id", canonical: "workspace"},
	{command: "drive +delete", emitted: "file-id", canonical: "node"},
	{command: "drive +download", emitted: "dentry-uuid", canonical: "node"},
	{command: "drive +download", emitted: "destination-path", canonical: "output"},
	{command: "drive +inspect", emitted: "include-statistics", canonical: "include-stats"},
	{command: "drive +list", emitted: "folder-id", canonical: "folder"},
	{command: "drive +list", emitted: "page-size", canonical: "limit"},
	{command: "drive +list", emitted: "next-token", canonical: "cursor"},
	{command: "drive +list", emitted: "sort-direction", canonical: "order"},
	{command: "drive +list", emitted: "sort-by", canonical: "order-by"},
	{command: "drive +publish-get", emitted: "dentry-uuid", canonical: "node"},
	{command: "drive +publish-unset", emitted: "file-id", canonical: "node"},
	{command: "drive +recycle-list", emitted: "storage-space-id", canonical: "space-id"},
	{command: "drive +recycle-list", emitted: "page-token", canonical: "cursor"},
	{command: "drive +recycle-restore", emitted: "recycle-item-id", canonical: "id"},
	{command: "drive +rename", emitted: "file-id", canonical: "node"},
	{command: "drive +rename", emitted: "new-name", canonical: "name"},
	{command: "drive +star-add", emitted: "dentry-uuid", canonical: "node"},
	{command: "drive +star-list", emitted: "max-results", canonical: "limit"},
	{command: "drive +star-remove", emitted: "file-id", canonical: "node"},
	{command: "drive +stats", emitted: "dentry-uuid", canonical: "node"},
	{command: "drive +upload", emitted: "source-file", canonical: "file"},
	{command: "drive +upload", emitted: "name", canonical: "file-name"},
	{command: "drive +upload", emitted: "overwrite-node-id", canonical: "node"},
	{command: "drive +version-download", emitted: "version-number", canonical: "version"},
	{command: "drive +version-download", emitted: "save-path", canonical: "output"},
	{command: "drive +version-get", emitted: "version-no", canonical: "version"},
	{command: "drive +version-history", emitted: "next-cursor", canonical: "cursor"},
	{command: "drive +version-history", emitted: "page-size", canonical: "limit"},
	{command: "drive +version-revert", emitted: "version-number", canonical: "version"},
	{command: "drive +cover", emitted: "node-id", canonical: "node"},
	{command: "drive +create-shortcut", emitted: "node-id", canonical: "node"},
	{command: "drive +delete", emitted: "node-id", canonical: "node"},
	{command: "drive +download", emitted: "node-id", canonical: "node"},
	{command: "drive +inspect", emitted: "node-id", canonical: "node"},
	{command: "drive +publish-get", emitted: "node-id", canonical: "node"},
	{command: "drive +publish-unset", emitted: "node-id", canonical: "node"},
	{command: "drive +rename", emitted: "node-id", canonical: "node"},
	{command: "drive +star-add", emitted: "node-id", canonical: "node"},
	{command: "drive +star-remove", emitted: "node-id", canonical: "node"},
	{command: "drive +stats", emitted: "node-id", canonical: "node"},
	{command: "drive +upload", emitted: "node-id", canonical: "node"},
	{command: "drive +version-download", emitted: "node-id", canonical: "node"},
	{command: "drive +version-get", emitted: "node-id", canonical: "node"},
	{command: "drive +version-history", emitted: "node-id", canonical: "node"},
	{command: "drive +version-revert", emitted: "node-id", canonical: "node"},
	{command: "drive +cover", emitted: "url", canonical: "node"},
	{command: "drive +create-shortcut", emitted: "document-id", canonical: "node"},
	{command: "drive +delete", emitted: "folder-id", canonical: "node"},
	{command: "drive +inspect", emitted: "document-id", canonical: "node"},
	{command: "drive +inspect", emitted: "folder-id", canonical: "node"},
	{command: "drive +publish-get", emitted: "url", canonical: "node"},
	{command: "drive +publish-unset", emitted: "document-url", canonical: "node"},
	{command: "drive +rename", emitted: "document-id", canonical: "node"},
	{command: "drive +rename", emitted: "folder-id", canonical: "node"},
	{command: "drive +star-add", emitted: "url", canonical: "node"},
	{command: "drive +star-remove", emitted: "doc-id", canonical: "node"},
	{command: "drive +stats", emitted: "document-url", canonical: "node"},
	{command: "drive +upload", emitted: "file-id", canonical: "node"},
}

// paramAliasNewDriveConfirmationCases selects one newly reviewed alias for
// every Drive command in the expansion whose declared runtime safety requires
// confirmation. The full matrix below proves all spellings preserve the
// confirmed payload; this smaller matrix proves aliases cannot cross the
// confirmation boundary before any transport call is made.
var paramAliasNewDriveConfirmationCases = []struct {
	command   string
	emitted   string
	canonical string
}{
	{command: "drive +delete", emitted: "file-id", canonical: "node"},
	{command: "drive +publish-unset", emitted: "document-url", canonical: "node"},
	{command: "drive +recycle-restore", emitted: "recycle-item-id", canonical: "id"},
	{command: "drive +rename", emitted: "new-name", canonical: "name"},
	{command: "drive +upload", emitted: "source-file", canonical: "file"},
	{command: "drive +version-revert", emitted: "version-number", canonical: "version"},
}

// paramAliasRepresentativePayloadCases keeps final transport coverage across
// old concept aliases, command overrides, native compatibility flags, read and
// write commands, and different products. Every reviewed alias is still
// checked through the embedded PreParse delivery path and against a complete
// business-valid command template. The dedicated product gates below continue
// to execute every alias introduced by the reviewed IM and Drive expansions.
//
// Keeping the older 100+ aliases at the contract layer avoids rebuilding and
// executing the complete 800+ command Root twice per spelling under -race.
// That duplicated command construction was enough to push the pre-existing
// macOS app suite beyond its package-level 10-minute timeout.
var paramAliasRepresentativePayloadCases = map[string]bool{
	paramAliasPayloadCaseKey("aitable +record-query", "base"):                        true, // concept alias on a shortcut read
	paramAliasPayloadCaseKey("attendance check result", "user-ids"):                  true, // list-valued concept alias
	paramAliasPayloadCaseKey("calendar event list", "date"):                          true, // time concept alias
	paramAliasPayloadCaseKey("chat message add-favorite", "msg-id"):                  true, // scoped IM identifier alias
	paramAliasPayloadCaseKey("contact user profile get", "user-id"):                  true, // native compatibility flag
	paramAliasPayloadCaseKey("devdoc article search", "current-page"):                true, // command override
	paramAliasPayloadCaseKey("doc +comment-create", "body"):                          true, // write shortcut content alias
	paramAliasPayloadCaseKey("doc +copy", "parent-folder-id"):                        true, // Doc folder role on a write shortcut
	paramAliasPayloadCaseKey("doc create", "space-id"):                               true, // published Doc workspace compatibility remains payload-equivalent
	paramAliasPayloadCaseKey("doc +create", "content-format"):                        true, // shortcut format alias preserves markdown/jsonml enum
	paramAliasPayloadCaseKey("doc +create-from-template", "keyword"):                 true, // template search alias composes with a write workflow
	paramAliasPayloadCaseKey("doc +create-from-template", "workspace-id"):            true, // template target workspace identifier
	paramAliasPayloadCaseKey("doc +create-from-template", "parent-folder-id"):        true, // template target folder identifier
	paramAliasPayloadCaseKey("doc +export-submit", "doc-id"):                         true, // Doc node identifier alias
	paramAliasPayloadCaseKey("doc +fetch", "start-block"):                            true, // section boundary role remains exact
	paramAliasPayloadCaseKey("doc +history-revert", "version-number"):                true, // destructive history version alias keeps confirmation
	paramAliasPayloadCaseKey("doc +inspect", "include-versions"):                     true, // boolean section alias preserves value
	paramAliasPayloadCaseKey("doc +search", "create-time-start"):                     true, // observed lower-bound spelling preserves milliseconds
	paramAliasPayloadCaseKey("doc +search", "create-time-end"):                       true, // observed upper-bound spelling preserves milliseconds
	paramAliasPayloadCaseKey("doc +template-list", "next-token"):                     true, // Doc cursor alias on a read shortcut
	paramAliasPayloadCaseKey("doc +update", "mode"):                                  true, // write operation selector alias
	paramAliasPayloadCaseKey("doc +update", "revision"):                              true, // optimistic edit revision alias
	paramAliasPayloadCaseKey("doc +access-grant", "doc-id"):                          true, // permission write keeps document identity
	paramAliasPayloadCaseKey("doc +version-revert", "version-number"):                true, // high-write version role with canonical confirmation
	paramAliasPayloadCaseKey("doc block insert", "content"):                          true, // block write content alias
	paramAliasPayloadCaseKey("doc block insert", "parent-block-id"):                  true, // scoped block-role alias
	paramAliasPayloadCaseKey("doc comment delete", "comment-id"):                     true, // destructive comment-key alias
	paramAliasPayloadCaseKey("doc comment reply", "mentioned-open-conversation-ids"): true, // list-valued group mention role
	paramAliasPayloadCaseKey("drive info", "workspace"):                              true, // published numeric storage-space compatibility remains payload-equivalent
	paramAliasPayloadCaseKey("mail folder update", "folder-id"):                      true, // write-command identifier alias
	paramAliasPayloadCaseKey("report list", "from-date"):                             true, // date-range concept alias
}

func TestCrossPlatformCoverageReviewedParamAliasesHaveCompleteTemplatesAndRepresentativeFinalPayloads(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	activeCommands := make(map[string]bool)
	activeCases := 0
	executedRepresentatives := make(map[string]bool)
	for _, fixture := range concepts.Fixture {
		if strings.HasPrefix(fixture.Expect, "did-you-mean:") {
			continue
		}
		activeCommands[fixture.Command] = true
		activeCases++
		complete, ok := paramAliasCompleteCommand(fixture.Command, fixture.Expect)
		if !ok {
			t.Errorf("reviewed active fixture %q/%q has no complete-command E2E template", fixture.Command, fixture.Emitted)
			continue
		}
		canonicalArgs := append([]string(nil), complete...)
		aliasArgs, replacements := replaceLongFlag(canonicalArgs, fixture.Expect, fixture.Emitted)
		if replacements != 1 {
			t.Errorf("complete command for %q/%q must contain canonical --%s exactly once; replacements=%d args=%v", fixture.Command, fixture.Emitted, fixture.Expect, replacements, canonicalArgs)
			continue
		}

		caseKey := paramAliasPayloadCaseKey(fixture.Command, fixture.Emitted)
		if !paramAliasRepresentativePayloadCases[caseKey] {
			continue
		}
		executedRepresentatives[caseKey] = true
		t.Run(fixture.Command+"/"+fixture.Emitted, func(t *testing.T) {

			canonicalCaller := &paramAliasCaptureCaller{}
			_, canonicalErr := executeParamAliasPayloadE2E(t, canonicalCaller, canonicalArgs...)
			if canonicalErr != nil {
				t.Fatalf("complete canonical command failed: %v\nargs=%v\ncalls=%#v", canonicalErr, canonicalArgs, canonicalCaller.calls)
			}
			if len(canonicalCaller.calls) == 0 {
				t.Fatalf("complete canonical command reached no final transport payload: args=%v", canonicalArgs)
			}

			aliasCaller := &paramAliasCaptureCaller{}
			ctx, aliasErr := executeParamAliasPayloadE2E(t, aliasCaller, aliasArgs...)
			if aliasErr != nil {
				t.Fatalf("complete alias command failed: %v\nargs=%v\ncalls=%#v", aliasErr, aliasArgs, aliasCaller.calls)
			}
			if ctx == nil {
				t.Fatal("complete alias command skipped PreParse")
			}
			normalizeParamAliasVolatileDefaults(fixture.Command, canonicalCaller, aliasCaller)
			if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
				t.Fatalf("final transport calls differ\ncanonical args: %v\nalias args: %v\ncanonical calls: %#v\nalias calls: %#v", canonicalArgs, aliasArgs, canonicalCaller.calls, aliasCaller.calls)
			}
		})
	}

	if activeCases == 0 {
		t.Fatal("reviewed fixture contains no active alias cases")
	}
	for command := range paramAliasCompleteCommands {
		if !activeCommands[command] {
			t.Errorf("complete-command E2E template %q has no active reviewed fixture", command)
		}
	}
	for command := range activeCommands {
		if _, ok := paramAliasCompleteCommands[command]; !ok {
			t.Errorf("active reviewed command %q has no complete-command E2E template", command)
		}
	}
	if len(activeCommands) != len(paramAliasCompleteCommands) {
		t.Fatalf("complete-command coverage = %d templates for %d active commands (%d active cases)", len(paramAliasCompleteCommands), len(activeCommands), activeCases)
	}
	for caseKey := range paramAliasRepresentativePayloadCases {
		if !executedRepresentatives[caseKey] {
			t.Errorf("representative final-payload case %q has no active reviewed fixture", caseKey)
		}
	}
	if len(executedRepresentatives) != len(paramAliasRepresentativePayloadCases) {
		t.Fatalf("representative final-payload coverage = %d, want %d", len(executedRepresentatives), len(paramAliasRepresentativePayloadCases))
	}
}

func TestCrossPlatformCoverageNewIMParamAliasesReachCanonicalEquivalentFinalPayloads(t *testing.T) {
	activeAliases := 0
	for _, test := range paramAliasNewIMCases {
		test := test
		t.Run(test.command+"/"+test.emitted, func(t *testing.T) {
			complete, ok := paramAliasCompleteCommand(test.command, test.canonical)
			if !ok {
				t.Fatal("reviewed IM alias has no complete-command E2E template")
			}
			canonicalArgs := append([]string(nil), complete...)
			aliasArgs, replacements := replaceLongFlag(canonicalArgs, test.canonical, test.emitted)
			if replacements != 1 {
				t.Fatalf("complete command must contain canonical --%s exactly once; replacements=%d args=%v", test.canonical, replacements, canonicalArgs)
			}

			canonicalCaller := &paramAliasCaptureCaller{}
			_, canonicalErr := executeParamAliasPayloadE2E(t, canonicalCaller, canonicalArgs...)
			if canonicalErr != nil && !paramAliasExpectedCaptureBoundaryError(test.command, canonicalErr) {
				t.Fatalf("complete canonical command failed: %v\nargs=%v\ncalls=%#v", canonicalErr, canonicalArgs, canonicalCaller.calls)
			}
			if len(canonicalCaller.calls) == 0 {
				t.Fatalf("complete canonical command reached no final transport payload: args=%v", canonicalArgs)
			}

			entry, exists := cli.LookupParamAlias(test.command)
			target, active := entry.ResolveAlias(test.emitted)
			if !exists || !active {
				return
			}
			if target != test.canonical {
				t.Fatalf("active reviewed IM alias --%s resolves to --%s, want --%s", test.emitted, target, test.canonical)
			}
			activeAliases++
			aliasCaller := &paramAliasCaptureCaller{}
			ctx, aliasErr := executeParamAliasPayloadE2E(t, aliasCaller, aliasArgs...)
			if aliasErr != nil && !paramAliasExpectedCaptureBoundaryError(test.command, aliasErr) {
				t.Fatalf("complete alias command failed: %v\nargs=%v\ncalls=%#v", aliasErr, aliasArgs, aliasCaller.calls)
			}
			if ctx == nil {
				t.Fatal("complete alias command skipped PreParse")
			}
			if (canonicalErr == nil) != (aliasErr == nil) || (canonicalErr != nil && canonicalErr.Error() != aliasErr.Error()) {
				t.Fatalf("canonical and alias completion errors differ: canonical=%v alias=%v", canonicalErr, aliasErr)
			}
			normalizeParamAliasVolatileDefaults(test.command, canonicalCaller, aliasCaller)
			if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
				t.Fatalf("final transport calls differ\ncanonical args: %v\nalias args: %v\ncanonical calls: %#v\nalias calls: %#v", canonicalArgs, aliasArgs, canonicalCaller.calls, aliasCaller.calls)
			}
		})
	}
	if activeAliases != len(paramAliasNewIMCases) {
		t.Fatalf("new IM aliases active in embedded table = %d, want %d", activeAliases, len(paramAliasNewIMCases))
	}
}

func TestCrossPlatformCoverageNewDriveParamAliasesReachCanonicalEquivalentFinalPayloads(t *testing.T) {
	activeAliases := 0
	for _, test := range paramAliasNewDriveCases {
		test := test
		t.Run(test.command+"/"+test.emitted, func(t *testing.T) {
			complete, ok := paramAliasCompleteCommand(test.command, test.canonical)
			if !ok {
				t.Fatal("reviewed Drive alias has no complete-command E2E template")
			}
			canonicalArgs := append([]string(nil), complete...)
			aliasArgs, replacements := replaceLongFlag(canonicalArgs, test.canonical, test.emitted)
			if replacements != 1 {
				t.Fatalf("complete command must contain canonical --%s exactly once; replacements=%d args=%v", test.canonical, replacements, canonicalArgs)
			}

			canonicalCaller := &paramAliasCaptureCaller{}
			_, canonicalErr := executeParamAliasPayloadE2E(t, canonicalCaller, canonicalArgs...)
			if canonicalErr != nil && !paramAliasExpectedCaptureBoundaryError(test.command, canonicalErr) {
				t.Fatalf("complete canonical command failed: %v\nargs=%v\ncalls=%#v", canonicalErr, canonicalArgs, canonicalCaller.calls)
			}
			if len(canonicalCaller.calls) == 0 {
				t.Fatalf("complete canonical command reached no final transport payload: args=%v", canonicalArgs)
			}

			entry, exists := cli.LookupParamAlias(test.command)
			target, active := entry.ResolveAlias(test.emitted)
			if !exists || !active {
				return
			}
			if target != test.canonical {
				t.Fatalf("active reviewed Drive alias --%s resolves to --%s, want --%s", test.emitted, target, test.canonical)
			}
			activeAliases++

			aliasCaller := &paramAliasCaptureCaller{}
			ctx, aliasErr := executeParamAliasPayloadE2E(t, aliasCaller, aliasArgs...)
			if aliasErr != nil && !paramAliasExpectedCaptureBoundaryError(test.command, aliasErr) {
				t.Fatalf("complete alias command failed: %v\nargs=%v\ncalls=%#v", aliasErr, aliasArgs, aliasCaller.calls)
			}
			if ctx == nil {
				t.Fatal("complete alias command skipped PreParse")
			}
			if (canonicalErr == nil) != (aliasErr == nil) || (canonicalErr != nil && canonicalErr.Error() != aliasErr.Error()) {
				t.Fatalf("canonical and alias completion errors differ: canonical=%v alias=%v", canonicalErr, aliasErr)
			}
			if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
				t.Fatalf("final transport calls differ\ncanonical args: %v\nalias args: %v\ncanonical calls: %#v\nalias calls: %#v", canonicalArgs, aliasArgs, canonicalCaller.calls, aliasCaller.calls)
			}
		})
	}
	if activeAliases != len(paramAliasNewDriveCases) {
		t.Fatalf("new Drive aliases active in embedded table = %d, want %d", activeAliases, len(paramAliasNewDriveCases))
	}
}

func TestCrossPlatformCoverageNewDriveParamAliasesCannotBypassConfirmation(t *testing.T) {
	for _, test := range paramAliasNewDriveConfirmationCases {
		test := test
		t.Run(test.command+"/"+test.emitted, func(t *testing.T) {
			complete, ok := paramAliasCompleteCommand(test.command, test.canonical)
			if !ok {
				t.Fatal("reviewed Drive confirmation alias has no complete-command E2E template")
			}
			aliasArgs, replacements := replaceLongFlag(complete, test.canonical, test.emitted)
			if replacements != 1 {
				t.Fatalf("complete command must contain canonical --%s exactly once; replacements=%d args=%v", test.canonical, replacements, complete)
			}
			unconfirmedArgs, removals := removeExactArg(aliasArgs, "--yes")
			if removals != 1 {
				t.Fatalf("confirmation template must contain --yes exactly once; removals=%d args=%v", removals, aliasArgs)
			}

			entry, exists := cli.LookupParamAlias(test.command)
			target, active := entry.ResolveAlias(test.emitted)
			if !exists || !active || target != test.canonical {
				t.Fatalf("reviewed Drive alias --%s resolution = exists:%v active:%v target:%q, want --%s", test.emitted, exists, active, target, test.canonical)
			}

			caller := &paramAliasCaptureCaller{}
			ctx, err := executeParamAliasPayloadE2E(t, caller, unconfirmedArgs...)
			if ctx == nil {
				t.Fatal("unconfirmed alias command skipped PreParse")
			}
			var appErr *apperrors.Error
			if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
				t.Fatalf("unconfirmed alias command error = %#v, want confirmation_required\nargs=%v", err, unconfirmedArgs)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("unconfirmed alias command crossed the transport boundary: args=%v calls=%#v", unconfirmedArgs, caller.calls)
			}
		})
	}
}

// Some artifact commands deliberately continue past the final captured
// transport call into local download/upload handling. Deterministic invalid
// resource responses form their expected post-transport test boundary;
// canonical and alias calls must still produce the same request and error.
func paramAliasExpectedCaptureBoundaryError(command string, err error) bool {
	if err == nil {
		return false
	}
	switch command {
	case "chat +messages-resource-download":
		return strings.Contains(err.Error(), "资源下载接口未返回合法的 HTTPS 下载地址")
	case "drive +download", "drive +version-download":
		return strings.Contains(err.Error(), "下载地址必须是受信任域名上的 HTTPS URL")
	case "drive +upload":
		return strings.Contains(err.Error(), "incomplete drive upload credentials")
	default:
		return false
	}
}

// +chat-messages supplies the current wall-clock time when callers omit
// --time. Alias equivalence concerns the resolved target and transport shape;
// a suite crossing a second boundary must not make that default appear
// alias-dependent.
func normalizeParamAliasVolatileDefaults(command string, callers ...*paramAliasCaptureCaller) {
	if command != "chat +chat-messages" {
		return
	}
	for _, caller := range callers {
		for i := range caller.calls {
			if caller.calls[i].tool == "list_conversation_message_v2" || caller.calls[i].tool == "list_individual_chat_message" {
				delete(caller.calls[i].args, "time")
			}
		}
	}
}

func paramAliasCompleteCommand(command, canonical string) ([]string, bool) {
	complete, ok := paramAliasCompleteCommands[command]
	if variants := paramAliasCompleteCommandVariants[command]; variants != nil {
		if variant, exists := variants[canonical]; exists {
			return variant, true
		}
	}
	return complete, ok
}

func paramAliasPayloadCaseKey(command, emitted string) string {
	return command + "\x00" + emitted
}

func executeParamAliasPayloadE2E(t *testing.T, caller *paramAliasCaptureCaller, args ...string) (*pipeline.Context, error) {
	t.Helper()
	return executeParamAliasE2E(t, caller, args...)
}

func replaceLongFlag(args []string, canonical, emitted string) ([]string, int) {
	out := append([]string(nil), args...)
	replacements := 0
	for index, arg := range out {
		if arg == "--"+canonical {
			out[index] = "--" + emitted
			replacements++
			continue
		}
		if strings.HasPrefix(arg, "--"+canonical+"=") {
			out[index] = "--" + emitted + strings.TrimPrefix(arg, "--"+canonical)
			replacements++
		}
	}
	return out, replacements
}

func removeExactArg(args []string, target string) ([]string, int) {
	out := make([]string, 0, len(args))
	removals := 0
	for _, arg := range args {
		if arg == target {
			removals++
			continue
		}
		out = append(out, arg)
	}
	return out, removals
}
