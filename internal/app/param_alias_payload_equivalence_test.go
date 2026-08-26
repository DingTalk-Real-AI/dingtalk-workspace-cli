// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"errors"
	"os"
	"os/exec"
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

	paramAliasCalendarPayloadChildEnv = "DWS_TEST_CALENDAR_PARAM_ALIAS_PAYLOAD_CHILD"
)

// paramAliasCompleteCommands is deliberately keyed by the exact reviewed
// fixture command path. Every argv is a complete, business-valid invocation:
// required companion flags are present, time and enum values are valid, and
// write commands use the capture caller rather than a real transport. The
// target canonical flag must occur exactly once so the test can replace only
// its spelling while holding every other input constant.
var paramAliasCompleteCommands = map[string][]string{
	"aitable +base-search":                     {"aitable", "+base-search", "--query", "fixture"},
	"aitable +export-data":                     {"aitable", "+export-data", "--base-id", "base-1", "--scope", "all", "--format", "excel"},
	"aitable +field-get":                       {"aitable", "+field-get", "--base-id", "base-1", "--table-id", "table-1"},
	"aitable +find-record":                     {"aitable", "+find-record", "--base", "base-1", "--table", "table-1", "--query", "fixture"},
	"aitable +list-tables":                     {"aitable", "+list-tables", "--base", "base-1"},
	"aitable +record-query":                    {"aitable", "+record-query", "--base-id", "base-1", "--table-id", "table-1", "--query", "fixture"},
	"aitable +record-share-links":              {"aitable", "+record-share-links", "--base", "base-1", "--table", "table-1", "--record-ids", "record-1"},
	"aitable +record-share-url":                {"aitable", "+record-share-url", "--base-id", "base-1", "--table-id", "table-1", "--record-ids", "record-1"},
	"aitable +table-get":                       {"aitable", "+table-get", "--base-id", "base-1"},
	"aitable +workflow-list":                   {"aitable", "+workflow-list", "--base-id", "base-1", "--limit", "7"},
	"aitable attachment upload":                {"aitable", "attachment", "upload", "--base-id", "base-1", "--file-name", "fixture.txt", "--size", "7"},
	"aitable base list":                        {"aitable", "base", "list", "--cursor", "cursor-1", "--limit", "7"},
	"aitable base update":                      {"aitable", "base", "update", "--base-id", "base-1", "--name", "Fixture Base", "--desc", "fixture description"},
	"aitable field search-options":             {"aitable", "field", "search-options", "--base-id", "base-1", "--table-id", "table-1", "--field-id", "field-1", "--keyword", "fixture", "--limit", "7"},
	"aitable record query":                     {"aitable", "record", "query", "--base-id", "base-1", "--table-id", "table-1", "--limit", "7"},
	"aitable workflow get":                     {"aitable", "workflow", "get", "--base-id", "base-1", "--workflow-id", "workflow-1"},
	"aitable workflow history":                 {"aitable", "workflow", "history", "--base-id", "base-1", "--workflow-id", "workflow-1", "--after-time", "1000", "--before-time", "2000", "--page", "2", "--size", "25"},
	"aitable workflow run":                     {"aitable", "workflow", "run", "--base-id", "base-1", "--workflow-id", "workflow-1", "--table-id", "table-1", "--record-ids", "record-1", "--yes"},
	"attendance check result":                  {"attendance", "check", "result", "--users", "user-1,user-2", "--start", "2026-03-01", "--end", "2026-03-02"},
	"attendance +check-result":                 {"attendance", "+check-result", "--users", "user-1,user-2", "--start", "2026-03-01", "--end", "2026-03-02"},
	"calendar +agenda":                         {"calendar", "+agenda", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--calendar-id", "primary", "--cursor", "cursor-1", "--limit", "7"},
	"calendar +attendee-list":                  {"calendar", "+attendee-list", "--event", "event-1", "--calendar-id", "primary"},
	"calendar +book":                           {"calendar", "+book", "--title", "Fixture Meeting", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T10:00:00+08:00", "--with", "Fixture User", "--yes"},
	"calendar +book-search":                    {"calendar", "+book-search", "--query", "fixture"},
	"calendar +cancel-event":                   {"calendar", "+cancel-event", "--event", "event-1", "--yes"},
	"calendar +conflicts":                      {"calendar", "+conflicts", "--in-days", "1"},
	"calendar +create":                         {"calendar", "+create", "--title", "Fixture Meeting", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T10:00:00+08:00", "--desc", "fixture description", "--attendees", "user-1,user-2", "--rooms", "room-1,room-2", "--calendar-id", "primary", "--yes"},
	"calendar +free":                           {"calendar", "+free", "--who", "Fixture User", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00"},
	"calendar +free-slots":                     {"calendar", "+free-slots", "--from", "9", "--to", "18", "--in-days", "1"},
	"calendar +freebusy":                       {"calendar", "+freebusy", "--users", "user-1,user-2", "--rooms", "room-1,room-2", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00"},
	"calendar +get":                            {"calendar", "+get", "--event", "event-1", "--calendar-id", "primary"},
	"calendar +invite":                         {"calendar", "+invite", "--event", "event-1", "--with", "Fixture User", "--yes"},
	"calendar +my-free":                        {"calendar", "+my-free", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00"},
	"calendar +reschedule":                     {"calendar", "+reschedule", "--event", "event-1", "--start", "2026-03-10T10:00:00+08:00", "--end", "2026-03-10T11:00:00+08:00", "--yes"},
	"calendar +room-find":                      {"calendar", "+room-find", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T10:00:00+08:00", "--room-name", "Fixture Room", "--group-id", "group-1", "--page", "1", "--limit", "7"},
	"calendar +room-groups":                    {"calendar", "+room-groups", "--page", "1", "--limit", "7"},
	"calendar +room-search":                    {"calendar", "+room-search", "--room-name", "Fixture Room"},
	"calendar +rsvp":                           {"calendar", "+rsvp", "--event", "event-1", "--status", "accept", "--calendar-id", "primary", "--yes"},
	"calendar +search-event":                   {"calendar", "+search-event", "--query", "fixture", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--calendar-id", "primary", "--cursor", "cursor-1", "--limit", "7"},
	"calendar +suggest-time":                   {"calendar", "+suggest-time", "--with", "Fixture User", "--duration", "30", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00"},
	"calendar +suggestion":                     {"calendar", "+suggestion", "--users", "user-1,user-2", "--duration", "30", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--timezone", "Asia/Shanghai"},
	"calendar +update":                         {"calendar", "+update", "--event", "event-1", "--title", "Fixture Updated Meeting", "--desc", "fixture updated description", "--start", "2026-03-10T10:00:00+08:00", "--end", "2026-03-10T11:00:00+08:00", "--add-attendees", "user-2", "--remove-attendees", "user-1", "--yes"},
	"calendar busy search":                     {"calendar", "busy", "search", "--users", "user-1,user-2", "--rooms", "room-1,room-2", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00"},
	"calendar event create":                    {"calendar", "event", "create", "--title", "Fixture Meeting", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T10:00:00+08:00", "--remind-minutes", "15", "--timezone", "Asia/Shanghai", "--rooms", "room-1,room-2"},
	"calendar event list":                      {"calendar", "event", "list", "--start", "2026-03-10T14:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--calendar-id", "primary", "--cursor", "cursor-1", "--limit", "7"},
	"calendar event respond":                   {"calendar", "event", "respond", "--id", "event-1", "--status", "accepted"},
	"calendar event suggest":                   {"calendar", "event", "suggest", "--users", "user-1,user-2", "--duration", "30", "--start", "2026-03-10T09:00:00+08:00", "--end", "2026-03-10T18:00:00+08:00", "--timezone", "Asia/Shanghai"},
	"calendar event update":                    {"calendar", "event", "update", "--id", "event-1", "--timezone", "Asia/Shanghai"},
	"calendar room add":                        {"calendar", "room", "add", "--event", "event-1", "--rooms", "room-1,room-2"},
	"calendar room delete":                     {"calendar", "room", "delete", "--event", "event-1", "--rooms", "room-1,room-2"},
	"calendar room search":                     {"calendar", "room", "search", "--room-name", "Fixture Room", "--group-id", "group-1", "--start", "2027-03-10T09:00:00+08:00", "--end", "2027-03-10T10:00:00+08:00", "--page", "1", "--limit", "7"},
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
	"chat +messages-reply":                     {"chat", "+messages-reply", "--group", "fixture-conversation", "--ref-msg-id", "message-1", "--ref-sender", appFixtureCurrentDOpenID, "--content", "hello fixture", "--yes"},
	"chat +messages-resource-download":         {"chat", "+messages-resource-download", "--resource-id", "resource-1", "--message-id", "message-1", "--open-conversation-id", "fixture-conversation", "--output", "downloads/fixture.bin"},
	"chat +messages-set-pin":                   {"chat", "+messages-set-pin", "--open-conversation-id", "fixture-conversation", "--msg-id", "message-1", "--yes"},
	"chat +messages-send-by-webhook":           {"chat", "+messages-send-by-webhook", "--token", "fixture-token", "--title", "Fixture Alert", "--content", "fixture", "--at-users", "user-1,user-2", "--yes"},
	"chat +search-msg":                         {"chat", "+search-msg", "--group", "fixture-conversation", "--query", "fixture", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-11T00:00:00+08:00", "--no-enrich"},
	"chat +send-to-group":                      {"chat", "+send-to-group", "--group", "Fixture Group", "--content", "hello fixture", "--yes"},
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
	"chat message reply":                       {"chat", "message", "reply", "--group", "fixture-conversation", "--ref-msg-id", "message-1", "--ref-sender", appFixtureCurrentDOpenID, "--content", "hello fixture", "--yes"},
	"chat message search-advanced":             {"chat", "message", "search-advanced", "--conversation-ids", "fixture-conversation", "--query", "fixture"},
	"chat message send":                        {"chat", "message", "send", "--user", appFixtureCurrentDOpenID, "--content", "hello fixture", "--idempotency-key", "param-alias-equivalence", "--yes"},
	"chat message send-by-bot":                 {"chat", "message", "send-by-bot", "--robot-code", "robot-1", "--group", "fixture-conversation", "--title", "Fixture Alert", "--text", "@user-1 @user-2 fixture", "--at-user-ids", "user-1,user-2", "--yes"},
	"chat message send-by-webhook":             {"chat", "message", "send-by-webhook", "--token", "fixture-token", "--title", "Fixture Alert", "--content", "fixture", "--at-users", "user-1,user-2", "--yes"},
	"contact +dept-members":                    {"contact", "+dept-members", "--dept", "Fixture Dept"},
	"contact +list-sub-depts":                  {"contact", "+list-sub-depts", "--dept", "1"},
	"contact +resolve-dept":                    {"contact", "+resolve-dept", "--name", "Fixture Dept"},
	"contact +search-user":                     {"contact", "+search-user", "--query", "Fixture User"},
	"contact dept list-children":               {"contact", "dept", "list-children", "--dept", "1"},
	"contact user profile get":                 {"contact", "user", "profile", "get", "--staff-id", "user-1", "--fields", "name,userId"},
	"dev app get":                              {"dev", "app", "get", "--unified-app-id", "app-1"},
	"devdoc article search":                    {"devdoc", "article", "search", "--query", "fixture", "--page", "2", "--size", "7"},
	"ding +receiver-status":                    {"ding", "+receiver-status", "--ding-id", "ding-1"},
	"ding message receiver-status":             {"ding", "message", "receiver-status", "--ding-id", "ding-1"},
	"ding message send":                        {"ding", "message", "send", "--robot-code", "robot-1", "--content", "fixture", "--users", "user-1"},
	"doc +comment-create":                      {"doc", "+comment-create", "--node", "node-1", "--content", "fixture comment", "--yes"},
	"doc +comment-list":                        {"doc", "+comment-list", "--node", "node-1", "--limit", "7", "--cursor", "cursor-1"},
	"doc +comment-reply":                       {"doc", "+comment-reply", "--node", "node-1", "--comment-key", "comment-1", "--content", "fixture reply", "--yes"},
	"doc +access-grant":                        {"doc", "+access-grant", "--node", "node-1", "--to", "Fixture User", "--role", "READER", "--workspace", "workspace-1", "--yes"},
	"doc +copy":                                {"doc", "+copy", "--node", "node-1", "--workspace", "workspace-1", "--yes"},
	"doc +create":                              {"doc", "+create", "--name", "Fixture Document", "--content", "fixture body", "--doc-format", "markdown"},
	"doc +create-from-template":                {"doc", "+create-from-template", "--query", "fixture template", "--name", "Fixture From Template", "--folder", "folder-1", "--workspace", "workspace-1"},
	"doc +doc-append":                          {"doc", "+doc-append", "--doc", "node-1", "--content", "fixture appendix", "--yes"},
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
	"doc block insert":                         {"doc", "block", "insert", "--node", "node-1", "--content", "fixture paragraph", "--yes"},
	"doc block update":                         {"doc", "block", "update", "--node", "node-1", "--block-id", "block-1", "--content", "fixture paragraph", "--yes"},
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
	"mail +find-mail-user":                     {"mail", "+find-mail-user", "--query", "fixture", "--limit", "7", "--cursor", "cursor-1"},
	"mail folder update":                       {"mail", "folder", "update", "--email", "fixture@example.com", "--id", "folder-1", "--name", "Fixture Folder"},
	"mail message search":                      {"mail", "message", "search", "--email", "fixture@example.com", "--query", "subject:fixture"},
	"mail thread list":                         {"mail", "thread", "list", "--email", "fixture@example.com", "--folder", "folder-1", "--limit", "7"},
	"mail user search":                         {"mail", "user", "search", "--keyword", "fixture"},
	"oa +search-forms":                         {"oa", "+search-forms", "--query", "fixture"},
	"oa approval search-forms":                 {"oa", "approval", "search-forms", "--query", "fixture"},
	"report list":                              {"report", "list", "--start", "2026-03-10T00:00:00+08:00", "--end", "2026-03-10T23:59:59+08:00"},
}

// paramAliasCandidateCompleteCommands contains complete invocations for
// reviewed parameter-concept product drafts. Keeping candidate-only commands
// in a separate map lets a test change land before a draft replaces the formal
// param_concepts.json: inactive candidate templates are ignored, while every
// command becomes mandatory as soon as one of its reviewed aliases is active.
var paramAliasCandidateCompleteCommands = map[string][]string{
	"agoal contract detail":                {"agoal", "contract", "detail", "--contract-id", "contract-1"},
	"agoal contract update":                {"agoal", "contract", "update", "--contract-id", "contract-1", "--dimensions", `[{"id":"dimension-1","title":"Fixture Dimension","weight":100,"objectives":[]}]`},
	"agoal obj-template create-or-update":  {"agoal", "obj-template", "create-or-update", "--template-id", "template-1", "--dimensions", `[{"title":"Fixture Dimension","weight":100}]`},
	"agoal obj-template list":              {"agoal", "obj-template", "list", "--keyword", "fixture", "--page", "2", "--page-size", "7"},
	"agoal report list-statistics":         {"agoal", "report", "list-statistics", "--keyword", "Fixture Rule"},
	"agoal report submit-detail":           {"agoal", "report", "submit-detail", "--template-id", "template-1", "--submit-state", "ON_TIME", "--query-date", "2026-06-18T00:00:00+08:00"},
	"agoal scorecard detail":               {"agoal", "scorecard", "detail", "--dept-id", "dept-1", "--selected-time", "2026-01-01T00:00:00+08:00"},
	"agoal scorecard entity-detail":        {"agoal", "scorecard", "entity-detail", "--sc-id", "scorecard-1", "--entity-id", "entity-1"},
	"agoal strategy detail":                {"agoal", "strategy", "detail", "--profile-id", "profile-1"},
	"agoal user objectives":                {"agoal", "user", "objectives", "--user-id", "user-1", "--rule-id", "rule-1", "--period-ids", "period-1,period-2"},
	"agoal user rules":                     {"agoal", "user", "rules", "--user-id", "user-1"},
	"aisearch":                             {"aisearch", "--query", "Fixture User", "--dimension", "name"},
	"aisearch +search-person":              {"aisearch", "+search-person", "--query", "Fixture User", "--dimensions", "name"},
	"aisearch behavior":                    {"aisearch", "behavior", "--queries", "fixture", "--types", "im", "--behavior-type", "send", "--chat-scope", "Fixture Group", "--direction", "我->Fixture User", "--time-range", "本周"},
	"aisearch enterprise":                  {"aisearch", "enterprise", "--queries", "fixture", "--types", "document", "--time-range", "本周"},
	"aisearch person":                      {"aisearch", "person", "--query", "Fixture User", "--dimension", "name"},
	"audit export":                         {"audit", "export", "--since", "2026-03-01", "--until", "2026-03-10", "--format", "jsonl", "--output", "/tmp/dws-audit-export-fixture.jsonl"},
	"audit tail":                           {"audit", "tail", "--lines", "7", "--output", "/tmp/dws-audit-tail-fixture.jsonl"},
	"audit verify":                         {"audit", "verify", "--file", "../../go.mod", "--output", "/tmp/dws-audit-verify-fixture.json"},
	"contact +by-mobile":                   {"contact", "+by-mobile", "--mobile", "13800138000"},
	"contact +list-dept-members":           {"contact", "+list-dept-members", "--depts", "1,2"},
	"contact +list-followings":             {"contact", "+list-followings", "--open-id", "open-fixture-1"},
	"contact +list-role-members":           {"contact", "+list-role-members", "--id", "12345"},
	"contact +lookup":                      {"contact", "+lookup", "--name", "Fixture User"},
	"contact +org":                         {"contact", "+org", "--name", "Fixture User"},
	"contact +search-mobile":               {"contact", "+search-mobile", "--mobile", "13800138000"},
	"contact +team":                        {"contact", "+team", "--name", "Fixture User"},
	"contact account create":               {"contact", "account", "create", "--login-id", "fixture-login", "--org-user-name", "Fixture User", "--dept-ids", "1,2"},
	"contact account update":               {"contact", "account", "update", "--user-id", "user-1", "--org-user-name", "Fixture User", "--depts", `[{"deptId":1}]`, "--avatar-file-id", "file-1", "--yes"},
	"contact dept create":                  {"contact", "dept", "create", "--name", "Fixture Dept", "--parent", "1", "--create-dept-group", "--yes"},
	"contact dept get-info":                {"contact", "dept", "get-info", "--dept", "1"},
	"contact dept list-members":            {"contact", "dept", "list-members", "--depts", "1,2"},
	"contact dept search":                  {"contact", "dept", "search", "--query", "Fixture Dept"},
	"contact dept update":                  {"contact", "dept", "update", "--dept", "2", "--name", "Fixture Dept", "--parent", "1", "--yes"},
	"contact label get":                    {"contact", "label", "get", "--names", "Fixture Role"},
	"contact org create":                   {"contact", "org", "create", "--org-name", "Fixture Org", "--creator-username", "Fixture Creator"},
	"contact user dismission search":       {"contact", "user", "dismission", "search", "--depts", "1,2", "--start", "2026-03-01", "--end", "2026-03-31", "--page", "2", "--limit", "7"},
	"contact user get":                     {"contact", "user", "get", "--ids", "user-1,user-2"},
	"contact user invite":                  {"contact", "user", "invite", "--org-user-mobile", "13800138000", "--org-user-name", "Fixture User", "--depts", `[{"deptId":1}]`},
	"contact user search":                  {"contact", "user", "search", "--query", "Fixture User"},
	"contact user search-mobile":           {"contact", "user", "search-mobile", "--mobile", "13800138000"},
	"contact user update":                  {"contact", "user", "update", "--user-id", "user-1", "--org-user-name", "Fixture User", "--depts", `[{"deptId":1}]`, "--yes"},
	"contact user update-ownness":          {"contact", "user", "update-ownness", "--user-id", "user-1", "--ownness-text", "Fixture Status", "--yes"},
	"contact user update-self":             {"contact", "user", "update-self", "--avatar-file-id", "file-1", "--nick", "Fixture Nick", "--yes"},
	"dev app create":                       {"dev", "app", "create", "--name", "Fixture App", "--desc", "Fixture Description", "--yes"},
	"dev app credentials get":              {"dev", "app", "credentials", "get", "--unified-app-id", "app-1"},
	"dev app delete":                       {"dev", "app", "delete", "--unified-app-id", "app-1", "--confirm-name", "Fixture App", "--yes"},
	"dev app disable":                      {"dev", "app", "disable", "--unified-app-id", "app-1", "--yes"},
	"dev app enable":                       {"dev", "app", "enable", "--unified-app-id", "app-1", "--yes"},
	"dev app event list":                   {"dev", "app", "event", "list", "--unified-app-id", "app-1", "--cursor", "cursor-1"},
	"dev app event subscribe":              {"dev", "app", "event", "subscribe", "--unified-app-id", "app-1", "--event-codes", "chat_message_received", "--yes"},
	"dev app event unsubscribe":            {"dev", "app", "event", "unsubscribe", "--unified-app-id", "app-1", "--event-codes", "chat_message_received", "--yes"},
	"dev app list":                         {"dev", "app", "list", "--robot-name", "Fixture Robot"},
	"dev app member add":                   {"dev", "app", "member", "add", "--unified-app-id", "app-1", "--member-type", "DEVELOPER", "--user-ids", "user-1,user-2", "--yes"},
	"dev app member list":                  {"dev", "app", "member", "list", "--unified-app-id", "app-1"},
	"dev app member remove":                {"dev", "app", "member", "remove", "--unified-app-id", "app-1", "--member-type", "DEVELOPER", "--user-ids", "user-1,user-2", "--yes"},
	"dev app permission add":               {"dev", "app", "permission", "add", "--unified-app-id", "app-1", "--scope-values", "Contact.User.Read", "--yes"},
	"dev app permission remove":            {"dev", "app", "permission", "remove", "--unified-app-id", "app-1", "--scope-values", "Contact.User.Read", "--yes"},
	"dev app robot config":                 {"dev", "app", "robot", "config", "--unified-app-id", "app-1", "--i18n-description", `{"zh_CN":"Fixture Robot"}`, "--yes"},
	"dev app robot disable":                {"dev", "app", "robot", "disable", "--unified-app-id", "app-1", "--yes"},
	"dev app robot enable":                 {"dev", "app", "robot", "enable", "--unified-app-id", "app-1", "--yes"},
	"dev app robot get":                    {"dev", "app", "robot", "get", "--unified-app-id", "app-1"},
	"dev app robot result":                 {"dev", "app", "robot", "result", "--task-id", "task-1"},
	"dev app robot submit":                 {"dev", "app", "robot", "submit", "--name", "Fixture Agent", "--desc", "Fixture robot description", "--robot-name", "Fixture Robot", "--yes"},
	"dev app security config":              {"dev", "app", "security", "config", "--unified-app-id", "app-1", "--redirect-urls", "https://example.test/callback", "--yes"},
	"dev app update":                       {"dev", "app", "update", "--unified-app-id", "app-1", "--name", "Fixture App", "--desc", "Fixture Description", "--yes"},
	"dev app version check-approval":       {"dev", "app", "version", "check-approval", "--unified-app-id", "app-1", "--version-id", "version-1"},
	"dev app version create":               {"dev", "app", "version", "create", "--unified-app-id", "app-1", "--version", "1.0.1", "--desc", "Fixture Version", "--yes"},
	"dev app version get":                  {"dev", "app", "version", "get", "--unified-app-id", "app-1", "--version-id", "version-1"},
	"dev app version list":                 {"dev", "app", "version", "list", "--unified-app-id", "app-1", "--cursor", "cursor-1"},
	"dev app version publish":              {"dev", "app", "version", "publish", "--unified-app-id", "app-1", "--version-id", "version-1", "--yes"},
	"dev app version status":               {"dev", "app", "version", "status", "--unified-app-id", "app-1", "--version-id", "version-1"},
	"dev app webapp config":                {"dev", "app", "webapp", "config", "--unified-app-id", "app-1", "--pc-homepage-url", "https://example.test/app", "--yes"},
	"dev app webapp get":                   {"dev", "app", "webapp", "get", "--unified-app-id", "app-1"},
	"dev connect restart":                  {"dev", "connect", "restart", "--robot-client-id", "robot-client-1"},
	"dev connect status":                   {"dev", "connect", "status", "--robot-client-id", "robot-client-1"},
	"dev connect stop":                     {"dev", "connect", "stop", "--robot-client-id", "robot-client-1"},
	"dev doc search":                       {"dev", "doc", "search", "--query", "fixture", "--page", "2"},
	"devdoc +search-docs":                  {"devdoc", "+search-docs", "--query", "fixture", "--page", "2", "--size", "7"},
	"devapp +create":                       {"devapp", "+create", "--name", "Fixture App", "--desc", "Fixture Description", "--yes"},
	"devapp +delete":                       {"devapp", "+delete", "--unified-app-id", "app-1", "--yes"},
	"devapp +disable":                      {"devapp", "+disable", "--unified-app-id", "app-1", "--yes"},
	"devapp +enable":                       {"devapp", "+enable", "--unified-app-id", "app-1", "--yes"},
	"devapp +event-list":                   {"devapp", "+event-list", "--unified-app-id", "app-1", "--cursor", "cursor-1"},
	"devapp +get":                          {"devapp", "+get", "--unified-app-id", "app-1"},
	"devapp +list":                         {"devapp", "+list", "--app-key", "app-key-1"},
	"devapp +member-add":                   {"devapp", "+member-add", "--unified-app-id", "app-1", "--member-type", "DEVELOPER", "--user-ids", "user-1,user-2", "--yes"},
	"devapp +member-list":                  {"devapp", "+member-list", "--unified-app-id", "app-1", "--user-id", "user-1"},
	"devapp +member-remove":                {"devapp", "+member-remove", "--unified-app-id", "app-1", "--member-type", "DEVELOPER", "--user-ids", "user-1,user-2", "--yes"},
	"devapp +permission-list":              {"devapp", "+permission-list", "--unified-app-id", "app-1", "--api-status", "PUBLISHED", "--scope-type", "APP"},
	"devapp +robot-get":                    {"devapp", "+robot-get", "--unified-app-id", "app-1"},
	"devapp +update":                       {"devapp", "+update", "--unified-app-id", "app-1", "--name", "Fixture App", "--desc", "Fixture Description", "--yes"},
	"devapp +version-check-approval":       {"devapp", "+version-check-approval", "--unified-app-id", "app-1", "--version-id", "version-1"},
	"devapp +version-get":                  {"devapp", "+version-get", "--unified-app-id", "app-1", "--version-id", "version-1"},
	"devapp +version-list":                 {"devapp", "+version-list", "--unified-app-id", "app-1", "--cursor", "cursor-1"},
	"devapp +version-status":               {"devapp", "+version-status", "--unified-app-id", "app-1", "--version-id", "version-1"},
	"devapp +webapp-config":                {"devapp", "+webapp-config", "--unified-app-id", "app-1", "--pc-homepage-url", "https://example.test/app", "--yes"},
	"devapp +webapp-get":                   {"devapp", "+webapp-get", "--unified-app-id", "app-1"},
	"event +listen-im":                     {"event", "+listen-im", "--user", "user-1", "--events", "message,reaction", "--query", "fixture", "--duration", "1s", "--max-events", "1"},
	"event consume":                        {"event", "consume", "--subscribe-id", "subscription-1", "--user", "user-1", "--group", "fixture-conversation", "--query", "fixture", "--output-dir", "/tmp/dws-event-fixture", "--filter-json", `{"rules":[]}`},
	"event list":                           {"event", "list", "--category", "im", "--include-pending"},
	"event schema":                         {"event", "schema", "--flatten"},
	"event status":                         {"event", "status", "--event", "im_message_received", "--status", "active", "--subscribe-id", "subscription-1"},
	"event stop":                           {"event", "stop", "--all", "--yes"},
	"hrbrain +get-pool":                    {"hrbrain", "+get-pool", "--pool-code", "pool-1"},
	"hrbrain +list-pool-employees":         {"hrbrain", "+list-pool-employees", "--pool-code", "pool-1", "--page", "2", "--page-size", "7"},
	"hrbrain +list-pools":                  {"hrbrain", "+list-pools", "--keyword", "fixture", "--labels", "label-a,label-b", "--page", "2", "--page-size", "7"},
	"hrbrain +profile-career":              {"hrbrain", "+profile-career", "--work-no", "work-1"},
	"hrbrain +profile-labels":              {"hrbrain", "+profile-labels", "--staff-ids", "work-1,work-2", "--all-label"},
	"hrbrain +profile-metadata":            {"hrbrain", "+profile-metadata", "--work-no", "work-1"},
	"hrbrain +profile-performance":         {"hrbrain", "+profile-performance", "--work-no", "work-1"},
	"hrbrain +query-profile":               {"hrbrain", "+query-profile", "--work-no", "work-1", "--data-queries", `[{"modelCode":"basic","fields":["name"]}]`},
	"hrbrain +search-employees":            {"hrbrain", "+search-employees", "--keyword", "fixture", "--dept-name", "Fixture Dept", "--position-name", "Engineer", "--job-level", "P7", "--pool-code", "pool-1", "--page", "2", "--page-size", "7"},
	"hrbrain +search-employees-structured": {"hrbrain", "+search-employees-structured", "--origin-json", `{"rules":[],"combinator":"and"}`, "--fields", `[{"label":"name","value":"name"}]`, "--order-by", "name", "--page", "2", "--page-size", "7"},
	"hrbrain profile career":               {"hrbrain", "profile", "career", "--work-no", "work-1"},
	"hrbrain profile labels":               {"hrbrain", "profile", "labels", "--staff-ids", "work-1,work-2", "--all-label"},
	"hrbrain profile metadata":             {"hrbrain", "profile", "metadata", "--work-no", "work-1"},
	"hrbrain profile performance":          {"hrbrain", "profile", "performance", "--work-no", "work-1"},
	"hrbrain profile query":                {"hrbrain", "profile", "query", "--work-no", "work-1", "--data-queries", `[{"modelCode":"basic","fields":["name"]}]`},
	"hrbrain search employees":             {"hrbrain", "search", "employees", "--keyword", "fixture", "--dept-name", "Fixture Dept", "--position-name", "Engineer", "--job-level", "P7", "--pool-code", "pool-1", "--page", "2", "--page-size", "7"},
	"hrbrain search employees-structured":  {"hrbrain", "search", "employees-structured", "--origin-json", `{"rules":[],"combinator":"and"}`, "--fields", `[{"label":"name","value":"name"}]`, "--order-by", "name", "--page", "2", "--page-size", "7"},
	"hrbrain talent-pool detail":           {"hrbrain", "talent-pool", "detail", "--pool-code", "pool-1"},
	"hrbrain talent-pool employees":        {"hrbrain", "talent-pool", "employees", "--pool-code", "pool-1", "--page", "2", "--page-size", "7"},
	"hrbrain talent-pool list":             {"hrbrain", "talent-pool", "list", "--keyword", "fixture", "--labels", "label-a,label-b", "--page", "2", "--page-size", "7"},
	"pat +browser-policy":                  {"pat", "+browser-policy", "--enabled=false", "--agent-code", "fixture-agent", "--dry-run"},
	"pat browser-policy":                   {"pat", "browser-policy", "--enabled=false", "--agentCode", "fixture-agent"},
	"pat chmod":                            {"pat", "chmod", "--product", "calendar", "--products", "aitable", "--domain", "chat", "--domains", "mail", "--grant-type", "session", "--session-id", "session-1", "--recommend", "--agentCode", "fixture-agent", "--dry-run"},
	"attendance +check-record":             {"attendance", "+check-record", "--users", "user-1,user-2", "--start", "2026-03-10 00:00:00", "--end", "2026-03-10 23:59:59"},
	"attendance +get-adjustment-rule":      {"attendance", "+get-adjustment-rule", "--adjustment-id", "adjustment-1"},
	"attendance +get-approve-template":     {"attendance", "+get-approve-template", "--type", "leave"},
	"attendance +get-checkin-record":       {"attendance", "+get-checkin-record", "--operator-corp-id", "corp-1", "--operator-staff-id", "staff-operator", "--staff-ids", "staff-1,staff-2", "--start", "2026-03-10 00:00:00", "--end", "2026-03-10 23:59:59"},
	"attendance +get-leave-records":        {"attendance", "+get-leave-records", "--user", "user-1", "--start", "2026-03-01", "--end", "2026-03-31", "--leave-code", "annual_leave"},
	"attendance +get-overtime-rule":        {"attendance", "+get-overtime-rule", "--overtime-id", "overtime-1"},
	"attendance +get-schedule":             {"attendance", "+get-schedule", "--users", "user-1,user-2", "--start", "2026-03-10", "--end", "2026-03-11"},
	"attendance +get-self-setting":         {"attendance", "+get-self-setting", "--user", "user-1", "--setting-scene", "checkRemind"},
	"attendance +get-summary":              {"attendance", "+get-summary", "--user", "user-1", "--date", "2026-03-10", "--stats-type", "week"},
	"attendance +list-approve":             {"attendance", "+list-approve", "--users", "user-1,user-2", "--types", "leave", "--start", "2026-03-01", "--end", "2026-03-31"},
	"attendance +query-report-data":        {"attendance", "+query-report-data", "--users", "user-1,user-2", "--columns", "attendance_days,late_count", "--start", "2026-03-01", "--end", "2026-03-31"},
	"attendance +search-adjustment-rule":   {"attendance", "+search-adjustment-rule", "--query", "fixture", "--page", "2", "--limit", "7"},
	"attendance +search-class":             {"attendance", "+search-class", "--filter-type", "name", "--query", "fixture"},
	"attendance +search-group":             {"attendance", "+search-group", "--type", "FIXED"},
	"attendance +search-overtime-rule":     {"attendance", "+search-overtime-rule", "--query", "fixture", "--page", "2", "--limit", "7"},
	"ding +list":                           {"ding", "+list", "--cursor", "0", "--type", "ALL"},
	"ding +recall-personal":                {"ding", "+recall-personal", "--id", "ding-1", "--yes"},
	"ding +send-personal":                  {"ding", "+send-personal", "--users", appFixtureCurrentDOpenID, "--content", "fixture", "--yes"},
	"mail +contact-list":                   {"mail", "+contact-list", "--email", "fixture@example.com", "--limit", "7", "--cursor", "cursor-1"},
	"mail +folder-list":                    {"mail", "+folder-list", "--email", "fixture@example.com", "--folder", "folder-1"},
	"mail +message":                        {"mail", "+message", "--email", "fixture@example.com", "--id", "message-1"},
	"mail +messages":                       {"mail", "+messages", "--email", "fixture@example.com", "--ids", "message-1,message-2"},
	"mail +recent-mail":                    {"mail", "+recent-mail", "--limit", "7", "--cursor", "cursor-1"},
	"mail +search-mail":                    {"mail", "+search-mail", "--query", "fixture", "--size", "7", "--cursor", "cursor-1"},
	"mail +template-list":                  {"mail", "+template-list", "--email", "fixture@example.com", "--limit", "7", "--cursor", "cursor-1"},
	"mail +thread":                         {"mail", "+thread", "--email", "fixture@example.com", "--id", "thread-1"},
	"mail +thread-list":                    {"mail", "+thread-list", "--email", "fixture@example.com", "--folder", "folder-1", "--cursor", "cursor-1"},
	"mail +triage":                         {"mail", "+triage", "--query", "fixture", "--limit", "7", "--cursor", "cursor-1"},
	"mail +unread-mail":                    {"mail", "+unread-mail", "--size", "7", "--cursor", "cursor-1"},
	"mail +user-search":                    {"mail", "+user-search", "--keyword", "fixture", "--cursor", "cursor-1"},
	"markdown create":                      {"markdown", "create", "--content", "# Fixture", "--name", "fixture.md", "--space-id", "space-1"},
	"markdown diff":                        {"markdown", "diff", "--node", "node-1", "--version", "1", "--version2", "2", "--context", "3"},
	"markdown fetch":                       {"markdown", "fetch", "--node", "node-1", "--space-id", "space-1", "--output", "/tmp/dws-markdown-fixture.md"},
	"markdown overwrite":                   {"markdown", "overwrite", "--node", "node-1", "--content", "# Fixture", "--name", "fixture.md", "--space-id", "space-1", "--yes"},
	"markdown patch":                       {"markdown", "patch", "--node", "node-1", "--pattern", "old", "--content", "new", "--regex", "--space-id", "space-1", "--yes"},
	"oa +list-cc":                          {"oa", "+list-cc", "--page", "2"},
	"oa +list-executed":                    {"oa", "+list-executed", "--limit", "7", "--page", "2"},
	"oa +list-forms":                       {"oa", "+list-forms", "--cursor", "2"},
	"oa +list-pending":                     {"oa", "+list-pending", "--start", "1773072000000", "--end", "1773158399000", "--page", "2"},
	"oa +list-submitted":                   {"oa", "+list-submitted", "--page", "2"},
	"oa +my-initiated":                     {"oa", "+my-initiated", "--page", "2"},
	"report +outbox-list":                  {"report", "+outbox-list", "--size", "7"},
	"report +report-latest":                {"report", "+report-latest", "--keyword", "Fixture", "--start", "2026-03-01T00:00:00+08:00", "--end", "2026-03-10T00:00:00+08:00"},
	"report +template-search":              {"report", "+template-search", "--query", "fixture"},
	"recruit job create":                   {"recruit", "job", "create", "--from", "testdata/recruit_job.json", "--yes"},
	"recruit job get":                      {"recruit", "job", "get", "--job-id", "job-1"},
	"recruit job list":                     {"recruit", "job", "list", "--job-ids", "job-1,job-2", "--creator-user-ids", "user-1,user-2", "--keyword", "fixture", "--cursor", "cursor-1", "--size", "7"},
	"sheet +list-sheets":                   {"sheet", "+list-sheets", "--node", "node-1"},
	"sheet +read":                          {"sheet", "+read", "--node", "node-1", "--sheet-id", "Sheet1"},

	"minutes +detail":             {"minutes", "+detail", "--ids", "u1,u2"},
	"minutes +latest":             {"minutes", "+latest", "--keyword", "fixture"},
	"minutes +list-all":           {"minutes", "+list-all", "--limit", "7"},
	"minutes +record-pause":       {"minutes", "+record-pause", "--id", "u1", "--yes"},
	"minutes +replace-batch":      {"minutes", "+replace-batch", "--id", "u1", "--pair", "old=>new", "--yes"},
	"minutes +search":             {"minutes", "+search", "--query", "fixture", "--cursor", "cursor-1"},
	"minutes +share":              {"minutes", "+share", "--ids", "u1,u2", "--member-uids", "user-1,user-2", "--permission", "view", "--yes"},
	"minutes +speaker-replace":    {"minutes", "+speaker-replace", "--id", "u1", "--from", "old", "--to", "new", "--target-uid", "user-1", "--yes"},
	"minutes +summary":            {"minutes", "+summary", "--id", "u1", "--content", "fixture", "--yes"},
	"minutes +transcript":         {"minutes", "+transcript", "--keyword", "fixture"},
	"minutes +upload-and-analyze": {"minutes", "+upload-and-analyze", "--resume-id", "u1", "--yes"},
	"minutes audio-memo list":     {"minutes", "audio-memo", "list", "--max", "7"},
	"minutes get batch":           {"minutes", "get", "batch", "--ids", "u1,u2"},
	"minutes hot-word add":        {"minutes", "hot-word", "add", "--words", "DWS,Minutes"},
	"minutes list all":            {"minutes", "list", "all", "--end", "2026-03-10T23:59:59+08:00"},
	"minutes list mine":           {"minutes", "list", "mine", "--start", "2026-03-10T00:00:00+08:00"},
	"minutes replace-text":        {"minutes", "replace-text", "--id", "u1", "--search", "old", "--replace", "new"},
	"minutes tag query":           {"minutes", "tag", "query", "--tag-id", "tag-1"},
	"minutes update title":        {"minutes", "update", "title", "--id", "u1", "--title", "Fixture Minutes"},
	"minutes upload complete":     {"minutes", "upload", "complete", "--session-id", "session-1"},
	"todo +assign":                {"todo", "+assign", "--task", "Fixture Todo", "--to", "Fixture User", "--yes"},
	"todo +assign-multi":          {"todo", "+assign-multi", "--task", "Fixture Todo", "--to", "Fixture User,User Two", "--yes"},
	"todo +comment":               {"todo", "+comment", "--task-id", "task-1", "--content", "fixture comment", "--yes"},
	"todo +complete":              {"todo", "+complete", "--task-id", "task-1", "--yes"},
	"todo +create":                {"todo", "+create", "--title", "Fixture Todo", "--executors", "user-1,user-2", "--due", "2026-03-10T18:00:00+08:00", "--yes"},
	"todo +due-today":             {"todo", "+due-today", "--role-types", "executor"},
	"todo +get-my-tasks":          {"todo", "+get-my-tasks", "--role-types", "executor", "--priority", "40", "--page", "2", "--size", "7"},
	"todo +get-related-tasks":     {"todo", "+get-related-tasks", "--role-types", "creator,executor", "--status", "false"},
	"todo +list-comment":          {"todo", "+list-comment", "--task-id", "task-1", "--page", "2"},
	"todo +remind":                {"todo", "+remind", "--task", "Fixture Todo", "--at", "2026-03-10T18:00:00+08:00", "--yes"},
	"todo +reminder":              {"todo", "+reminder", "--task-id", "task-1", "--base-time", "customTime", "--at", "2026-03-10T18:00:00+08:00", "--yes"},
	"todo +reopen":                {"todo", "+reopen", "--task-id", "task-1", "--yes"},
	"todo +search":                {"todo", "+search", "--query", "fixture", "--status", "false"},
	"todo +todo-done":             {"todo", "+todo-done", "--task", "Fixture Todo", "--yes"},
	"todo +update":                {"todo", "+update", "--task-id", "task-1", "--title", "Fixture Updated Todo", "--yes"},
	"todo comment add":            {"todo", "comment", "add", "--task-id", "task-1", "--content", "fixture comment", "--yes"},
	"todo comment list":           {"todo", "comment", "list", "--task-id", "task-1", "--page", "2", "--size", "7"},
	"todo task add-executor":      {"todo", "task", "add-executor", "--task-id", "task-1", "--executors", "user-1,user-2", "--yes"},
	"todo task add-participant":   {"todo", "task", "add-participant", "--task-id", "task-1", "--participants", "user-1,user-2", "--yes"},
	"todo task add-reminder":      {"todo", "task", "add-reminder", "--task-id", "task-1", "--base-time", "customTime", "--reminder-time-stamp", "2026-03-10T18:00:00+08:00", "--yes"},
	"todo task create":            {"todo", "task", "create", "--title", "Fixture Todo", "--executors", "user-1,user-2", "--due", "2026-03-10T18:00:00+08:00", "--yes"},
	"todo task create-sub":        {"todo", "task", "create-sub", "--parent-id", "task-parent", "--title", "Fixture Sub Todo", "--executors", "user-1", "--yes"},
	"todo task done":              {"todo", "task", "done", "--task-id", "task-1", "--status", "true", "--yes"},
	"todo task get":               {"todo", "task", "get", "--task-id", "task-1"},
	"todo task list":              {"todo", "task", "list", "--role-types", "executor", "--page", "2", "--size", "7"},
	"todo task update":            {"todo", "task", "update", "--task-id", "task-1", "--done", "true", "--yes"},
	"wiki +member-add":            {"wiki", "+member-add", "--workspace", "workspace-1", "--user", "user-1", "--role", "READER", "--yes"},
	"wiki +member-remove":         {"wiki", "+member-remove", "--workspace", "workspace-1", "--user", "user-1", "--yes"},
	"wiki +member-update":         {"wiki", "+member-update", "--workspace", "workspace-1", "--user", "user-1", "--role", "EDITOR", "--yes"},
	"wiki +move":                  {"wiki", "+move", "--workspace", "workspace-1", "--node", "node-1", "--folder", "folder-1", "--yes"},
	"wiki +move-to-drive":         {"wiki", "+move-to-drive", "--node", "node-1", "--folder", "folder-1", "--yes"},
	"wiki +node-copy":             {"wiki", "+node-copy", "--workspace", "workspace-1", "--node", "node-1", "--folder", "folder-1", "--yes"},
	"wiki +node-delete":           {"wiki", "+node-delete", "--workspace", "workspace-1", "--node", "node-1", "--yes"},
}

// A command can expose more than one mutually exclusive canonical route. In
// that case the shared command template above cannot contain every canonical
// flag at once, so select a fixture-specific complete invocation here.
var paramAliasCompleteCommandVariants = map[string]map[string][]string{
	"dev app get": {
		"app-key": {"dev", "app", "get", "--app-key", "app-key-1"},
	},
	"event +listen-im": {
		"open-dingtalk-id": {"event", "+listen-im", "--open-dingtalk-id", appFixtureCurrentDOpenID, "--events", "message,reaction", "--query", "fixture", "--duration", "1s", "--max-events", "1"},
		"user-query":       {"event", "+listen-im", "--user-query", "Fixture User", "--events", "message,reaction", "--query", "fixture", "--duration", "1s", "--max-events", "1"},
		"chat-id":          {"event", "+listen-im", "--chat-id", "fixture-conversation", "--events", "message,reaction", "--query", "fixture", "--duration", "1s", "--max-events", "1"},
		"chat-query":       {"event", "+listen-im", "--chat-query", "Fixture Group", "--events", "message,reaction", "--query", "fixture", "--duration", "1s", "--max-events", "1"},
	},
	"event consume": {
		"open-dingtalk-id": {"event", "consume", "--subscribe-id", "subscription-1", "--open-dingtalk-id", appFixtureCurrentDOpenID, "--group", "fixture-conversation", "--query", "fixture", "--output-dir", "/tmp/dws-event-fixture", "--filter-json", `{"rules":[]}`},
	},
	"markdown create": {
		"file": {"markdown", "create", "--file", "../../README.md", "--name", "fixture.md", "--space-id", "space-1"},
	},
	"markdown diff": {
		"file": {"markdown", "diff", "--node", "node-1", "--file", "../../README.md", "--context", "3"},
	},
	"markdown overwrite": {
		"file":    {"markdown", "overwrite", "--node", "node-1", "--file", "../../README.md", "--name", "fixture.md", "--space-id", "space-1", "--yes"},
		"dry-run": {"markdown", "overwrite", "--node", "node-1", "--content", "# Fixture", "--name", "fixture.md", "--space-id", "space-1", "--dry-run"},
	},
	"markdown patch": {
		"dry-run": {"markdown", "patch", "--node", "node-1", "--pattern", "old", "--content", "new", "--regex", "--dry-run"},
	},

	"doc +copy": {
		"folder":    {"doc", "+copy", "--node", "node-1", "--folder", "folder-1", "--yes"},
		"workspace": {"doc", "+copy", "--node", "node-1", "--workspace", "workspace-1", "--yes"},
	},
	"doc block insert": {
		"parent-block": {"doc", "block", "insert", "--node", "node-1", "--parent-block", "parent-block-1", "--index", "0", "--content", "fixture paragraph", "--yes"},
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
		"group": {"chat", "message", "send", "--group", "fixture-conversation", "--content", "hello fixture", "--idempotency-key", "param-alias-equivalence-group", "--yes"},
		"file":  {"chat", "message", "send", "--group", "fixture-conversation", "--msg-type", "file", "--file", "../../go.mod", "--dentry-id", "1", "--space-id", "2", "--idempotency-key", "param-alias-equivalence-file", "--yes"},
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
	{command: "chat +messages-reply", emitted: "chat", canonical: "group"},
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

// paramAliasAITableDeleteDisableCompleteCommands contains complete invocations
// for every AITable delete/disable command whose confirmation boundary is
// reached by aliases introduced in the AITable expansion. These templates are
// intentionally separate from paramAliasCompleteCommands: that map mirrors
// the reviewed validation fixture one-for-one, while this matrix exhaustively
// proves the safety boundary for generated aliases beyond the fixture sample.
var paramAliasAITableDeleteDisableCompleteCommands = map[string][]string{
	"aitable +advperm-disable":      {"aitable", "+advperm-disable", "--base-id", "base-1", "--yes"},
	"aitable +base-delete":          {"aitable", "+base-delete", "--base-id", "base-1", "--yes"},
	"aitable +chart-delete":         {"aitable", "+chart-delete", "--base-id", "base-1", "--dashboard-id", "dashboard-1", "--chart-id", "chart-1", "--yes"},
	"aitable +dashboard-delete":     {"aitable", "+dashboard-delete", "--base-id", "base-1", "--dashboard-id", "dashboard-1", "--yes"},
	"aitable +field-delete":         {"aitable", "+field-delete", "--base-id", "base-1", "--table-id", "table-1", "--field-id", "field-1", "--yes"},
	"aitable +form-delete":          {"aitable", "+form-delete", "--base-id", "base-1", "--table-id", "table-1", "--view-id", "view-1", "--yes"},
	"aitable +record-delete":        {"aitable", "+record-delete", "--base-id", "base-1", "--table-id", "table-1", "--record-ids", "record-1", "--yes"},
	"aitable +role-delete":          {"aitable", "+role-delete", "--base-id", "base-1", "--role-id", "role-1", "--yes"},
	"aitable +section-delete":       {"aitable", "+section-delete", "--base-id", "base-1", "--section-id", "section-1", "--yes"},
	"aitable +table-delete":         {"aitable", "+table-delete", "--base-id", "base-1", "--table-id", "table-1", "--yes"},
	"aitable +view-delete":          {"aitable", "+view-delete", "--base-id", "base-1", "--table-id", "table-1", "--view-id", "view-1", "--yes"},
	"aitable +workflow-disable":     {"aitable", "+workflow-disable", "--base-id", "base-1", "--workflow-id", "workflow-1", "--yes"},
	"aitable advperm disable":       {"aitable", "advperm", "disable", "--base-id", "base-1", "--yes"},
	"aitable advperm role-delete":   {"aitable", "advperm", "role-delete", "--base-id", "base-1", "--role-id", "role-1", "--yes"},
	"aitable base delete":           {"aitable", "base", "delete", "--base-id", "base-1", "--yes"},
	"aitable chart delete":          {"aitable", "chart", "delete", "--base-id", "base-1", "--dashboard-id", "dashboard-1", "--chart-id", "chart-1", "--yes"},
	"aitable dashboard delete":      {"aitable", "dashboard", "delete", "--base-id", "base-1", "--dashboard-id", "dashboard-1", "--yes"},
	"aitable field delete":          {"aitable", "field", "delete", "--base-id", "base-1", "--table-id", "table-1", "--field-id", "field-1", "--yes"},
	"aitable form delete":           {"aitable", "form", "delete", "--base-id", "base-1", "--table-id", "table-1", "--view-id", "view-1", "--yes"},
	"aitable form questions delete": {"aitable", "form", "questions", "delete", "--base-id", "base-1", "--table-id", "table-1", "--field-id", "field-1", "--yes"},
	"aitable record delete":         {"aitable", "record", "delete", "--base-id", "base-1", "--table-id", "table-1", "--record-ids", "record-1", "--yes"},
	"aitable table delete":          {"aitable", "table", "delete", "--base-id", "base-1", "--table-id", "table-1", "--yes"},
	"aitable view delete":           {"aitable", "view", "delete", "--base-id", "base-1", "--table-id", "table-1", "--view-id", "view-1", "--yes"},
	"aitable workflow disable":      {"aitable", "workflow", "disable", "--base-id", "base-1", "--workflow-id", "workflow-1", "--yes"},
}

// paramAliasNewAITableDeleteDisableCases is the exhaustive set of alias
// tuples newly introduced by this change on AITable delete/disable commands
// that require confirmation. Every tuple must remain on both sides of the
// confirmation gate: rejected with zero calls before --yes, and exactly
// payload-equivalent to its canonical spelling after --yes.
var paramAliasNewAITableDeleteDisableCases = []struct {
	command   string
	emitted   string
	canonical string
}{
	{command: "aitable +advperm-disable", emitted: "base", canonical: "base-id"},
	{command: "aitable +advperm-disable", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +base-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +base-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +chart-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +chart-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +dashboard-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +dashboard-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +field-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +field-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +field-delete", emitted: "table", canonical: "table-id"},
	{command: "aitable +form-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +form-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +form-delete", emitted: "table", canonical: "table-id"},
	{command: "aitable +record-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +record-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +record-delete", emitted: "table", canonical: "table-id"},
	{command: "aitable +role-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +role-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +section-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +section-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +table-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +table-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +table-delete", emitted: "table", canonical: "table-id"},
	{command: "aitable +view-delete", emitted: "base", canonical: "base-id"},
	{command: "aitable +view-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +view-delete", emitted: "table", canonical: "table-id"},
	{command: "aitable +workflow-disable", emitted: "base", canonical: "base-id"},
	{command: "aitable +workflow-disable", emitted: "base-token", canonical: "base-id"},
	{command: "aitable +workflow-disable", emitted: "flow-id", canonical: "workflow-id"},
	{command: "aitable advperm disable", emitted: "base-token", canonical: "base-id"},
	{command: "aitable advperm role-delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable base delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable chart delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable dashboard delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable field delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable field delete", emitted: "table", canonical: "table-id"},
	{command: "aitable form delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable form delete", emitted: "table", canonical: "table-id"},
	{command: "aitable form questions delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable form questions delete", emitted: "table", canonical: "table-id"},
	{command: "aitable record delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable record delete", emitted: "table", canonical: "table-id"},
	{command: "aitable table delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable table delete", emitted: "table", canonical: "table-id"},
	{command: "aitable view delete", emitted: "base-token", canonical: "base-id"},
	{command: "aitable view delete", emitted: "table", canonical: "table-id"},
	{command: "aitable workflow disable", emitted: "base-token", canonical: "base-id"},
	{command: "aitable workflow disable", emitted: "flow-id", canonical: "workflow-id"},
}

// paramAliasNewConfirmationCases selects newly reviewed aliases for commands
// whose declared runtime safety requires confirmation. The full matrix below
// proves all spellings preserve the confirmed payload; this smaller matrix
// proves aliases cannot cross the confirmation boundary before any transport
// call is made.
var paramAliasNewConfirmationCases = []struct {
	command   string
	emitted   string
	canonical string
}{
	{command: "aitable workflow run", emitted: "base-token", canonical: "base-id"},
	{command: "aitable workflow run", emitted: "flow-id", canonical: "workflow-id"},
	{command: "aitable workflow run", emitted: "table", canonical: "table-id"},
	{command: "drive +delete", emitted: "file-id", canonical: "node"},
	{command: "drive +publish-unset", emitted: "document-url", canonical: "node"},
	{command: "drive +recycle-restore", emitted: "recycle-item-id", canonical: "id"},
	{command: "drive +rename", emitted: "new-name", canonical: "name"},
	{command: "drive +upload", emitted: "source-file", canonical: "file"},
	{command: "drive +version-revert", emitted: "version-number", canonical: "version"},
}

// Candidate confirmation cases become active with the joint draft. One write
// workflow per product plus TODO's reminder workflow proves semantic aliasing
// cannot move execution across the shared --yes barrier.
var paramAliasCandidateConfirmationCases = []struct {
	command   string
	emitted   string
	canonical string
}{
	{command: "minutes +record-pause", emitted: "uuid", canonical: "id"},
	{command: "todo +create", emitted: "deadline", canonical: "due"},
	{command: "todo +reminder", emitted: "reminder-time-stamp", canonical: "at"},
	{command: "wiki +node-copy", emitted: "node-id", canonical: "node"},
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
	paramAliasPayloadCaseKey("aitable +export-data", "export-format"):                true, // shortcut-local export format keeps the final payload
	paramAliasPayloadCaseKey("aitable +find-record", "base-id"):                      true, // shortcut Base ID compatibility
	paramAliasPayloadCaseKey("aitable +find-record", "table-id"):                     true, // shortcut Table ID compatibility
	paramAliasPayloadCaseKey("aitable +record-query", "base"):                        true, // concept alias on a shortcut read
	paramAliasPayloadCaseKey("aitable +record-share-links", "base-id"):               true, // observed experiment Base ID spelling
	paramAliasPayloadCaseKey("aitable +record-share-links", "table-id"):              true, // observed experiment Table ID spelling
	paramAliasPayloadCaseKey("aitable +workflow-list", "max-results"):                true, // shortcut pagination-size alias
	paramAliasPayloadCaseKey("aitable attachment upload", "file-size"):               true, // byte-size command override
	paramAliasPayloadCaseKey("aitable base list", "next-cursor"):                     true, // cursor concept alias
	paramAliasPayloadCaseKey("aitable base update", "description"):                   true, // plain description alias on a write command
	paramAliasPayloadCaseKey("aitable field search-options", "query"):                true, // search keyword concept alias
	paramAliasPayloadCaseKey("aitable workflow get", "flow-id"):                      true, // workflow ID concept alias
	paramAliasPayloadCaseKey("aitable workflow history", "base-token"):               true, // Base ID concept alias
	paramAliasPayloadCaseKey("aitable workflow history", "end-time"):                 true, // upper time-bound override
	paramAliasPayloadCaseKey("aitable workflow history", "flow-id"):                  true, // workflow ID concept alias
	paramAliasPayloadCaseKey("aitable workflow history", "page-index"):               true, // zero-based page override
	paramAliasPayloadCaseKey("aitable workflow history", "page-size"):                true, // page-size concept alias
	paramAliasPayloadCaseKey("aitable workflow history", "start-time"):               true, // lower time-bound override
	paramAliasPayloadCaseKey("aitable workflow run", "base-token"):                   true, // Base ID concept alias on a confirmed write
	paramAliasPayloadCaseKey("aitable workflow run", "flow-id"):                      true, // workflow ID concept alias on a confirmed write
	paramAliasPayloadCaseKey("aitable workflow run", "table"):                        true, // Table ID concept alias on a confirmed write
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
	paramAliasPayloadCaseKey("chat +messages-reply", "conversation-id"):              true, // renamed conversation Primary keeps final reply payload
	paramAliasPayloadCaseKey("chat message send", "file-path"):                       true, // renamed local-file Primary reaches the same final payload
	paramAliasPayloadCaseKey("doc +doc-append", "text"):                              true, // shortcut content rename keeps append payload
	paramAliasPayloadCaseKey("doc block insert", "text"):                             true, // block write content compatibility alias
	paramAliasPayloadCaseKey("doc block update", "text"):                             true, // update uses the same typed compatibility path
	paramAliasPayloadCaseKey("doc block insert", "parent-block-id"):                  true, // scoped block-role alias
	paramAliasPayloadCaseKey("doc comment delete", "comment-id"):                     true, // destructive comment-key alias
	paramAliasPayloadCaseKey("doc comment reply", "mentioned-open-conversation-ids"): true, // list-valued group mention role
	paramAliasPayloadCaseKey("drive info", "workspace"):                              true, // published numeric storage-space compatibility remains payload-equivalent
	paramAliasPayloadCaseKey("mail folder update", "folder-id"):                      true, // write-command identifier alias
	paramAliasPayloadCaseKey("report list", "from-date"):                             true, // date-range concept alias
}

// Candidate representatives exercise the final transport boundary for each
// Minutes/TODO/Wiki alias family. They are required only when the exact fixture
// exists in the loaded reviewed table, so the tests are mergeable before the
// joint draft is promoted to internal/cli/param_concepts.json.
var paramAliasCandidateRepresentativePayloadCases = map[string]bool{
	paramAliasPayloadCaseKey("minutes +latest", "query"):               true,
	paramAliasPayloadCaseKey("minutes +transcript", "query"):           true,
	paramAliasPayloadCaseKey("minutes get batch", "uuids"):             true,
	paramAliasPayloadCaseKey("minutes update title", "task-uuid"):      true,
	paramAliasPayloadCaseKey("minutes upload complete", "upload-id"):   true,
	paramAliasPayloadCaseKey("todo +create", "deadline"):               true,
	paramAliasPayloadCaseKey("todo +get-my-tasks", "current-page"):     true,
	paramAliasPayloadCaseKey("todo +reminder", "reminder-time-stamp"):  true,
	paramAliasPayloadCaseKey("todo comment add", "text"):               true,
	paramAliasPayloadCaseKey("todo task add-executor", "executor-ids"): true,
	paramAliasPayloadCaseKey("todo task get", "todo-id"):               true,
	paramAliasPayloadCaseKey("todo task update", "status"):             true,
	paramAliasPayloadCaseKey("wiki +member-add", "user-id"):            true,
	paramAliasPayloadCaseKey("wiki +member-remove", "uid"):             true,
	paramAliasPayloadCaseKey("wiki +member-update", "user-id"):         true,
	paramAliasPayloadCaseKey("wiki +move-to-drive", "node-id"):         true,
	paramAliasPayloadCaseKey("wiki +node-copy", "node-id"):             true,
}

// paramAliasCalendarPayloadCases keeps the full reviewed Calendar expansion
// separate from the long-lived app-c race process. Each case still executes
// both canonical and alias argv through the real PreParse/Cobra path and
// compares the final captured transport calls; the owning top-level test runs
// these allocations in a short-lived race-instrumented subprocess so all Root
// registrations are released together when that process exits.
var paramAliasCalendarPayloadCases = map[string]bool{
	paramAliasPayloadCaseKey("calendar +agenda", "from"):                    true,
	paramAliasPayloadCaseKey("calendar +agenda", "to"):                      true,
	paramAliasPayloadCaseKey("calendar +agenda", "max-results"):             true,
	paramAliasPayloadCaseKey("calendar +agenda", "next-cursor"):             true,
	paramAliasPayloadCaseKey("calendar +agenda", "calendar-book-id"):        true,
	paramAliasPayloadCaseKey("calendar +attendee-list", "event-id"):         true,
	paramAliasPayloadCaseKey("calendar +attendee-list", "calendar-book-id"): true,
	paramAliasPayloadCaseKey("calendar +book", "summary"):                   true,
	paramAliasPayloadCaseKey("calendar +book", "attendee-names"):            true,
	paramAliasPayloadCaseKey("calendar +book-search", "keyword"):            true,
	paramAliasPayloadCaseKey("calendar +book-search", "search"):             true,
	paramAliasPayloadCaseKey("calendar +book-search", "name"):               true,
	paramAliasPayloadCaseKey("calendar +cancel-event", "event-id"):          true,
	paramAliasPayloadCaseKey("calendar +cancel-event", "id"):                true,
	paramAliasPayloadCaseKey("calendar +free", "name"):                      true,
	paramAliasPayloadCaseKey("calendar +free-slots", "start-hour"):          true,
	paramAliasPayloadCaseKey("calendar +free-slots", "end-hour"):            true,
	paramAliasPayloadCaseKey("calendar +free-slots", "day-offset"):          true,
	paramAliasPayloadCaseKey("calendar +freebusy", "user-ids"):              true,
	paramAliasPayloadCaseKey("calendar +freebusy", "room-ids"):              true,
	paramAliasPayloadCaseKey("calendar +freebusy", "room-id"):               true,
	paramAliasPayloadCaseKey("calendar +my-free", "from"):                   true,
	paramAliasPayloadCaseKey("calendar +my-free", "to"):                     true,
	paramAliasPayloadCaseKey("calendar +invite", "id"):                      true,
	paramAliasPayloadCaseKey("calendar +invite", "participant-names"):       true,
	paramAliasPayloadCaseKey("calendar +reschedule", "id"):                  true,
	paramAliasPayloadCaseKey("calendar +reschedule", "from"):                true,
	paramAliasPayloadCaseKey("calendar +reschedule", "to"):                  true,
	paramAliasPayloadCaseKey("calendar +room-groups", "page-size"):          true,
	paramAliasPayloadCaseKey("calendar +room-groups", "page-index"):         true,
	paramAliasPayloadCaseKey("calendar +room-search", "query"):              true,
	paramAliasPayloadCaseKey("calendar +suggest-time", "duration-minutes"):  true,
	paramAliasPayloadCaseKey("calendar +suggest-time", "attendee-names"):    true,
	paramAliasPayloadCaseKey("calendar +conflicts", "day-offset"):           true,
	paramAliasPayloadCaseKey("calendar busy search", "room-id"):             true,
	paramAliasPayloadCaseKey("calendar event create", "reminder-minutes"):   true,
	paramAliasPayloadCaseKey("calendar event create", "tz"):                 true,
	paramAliasPayloadCaseKey("calendar event create", "room-id"):            true,
	paramAliasPayloadCaseKey("calendar event respond", "response-status"):   true,
	paramAliasPayloadCaseKey("calendar event suggest", "duration-minutes"):  true,
	paramAliasPayloadCaseKey("calendar event update", "tz"):                 true,
	paramAliasPayloadCaseKey("calendar room add", "room-id"):                true,
	paramAliasPayloadCaseKey("calendar room delete", "room-id"):             true,
	paramAliasPayloadCaseKey("calendar room search", "room-group-id"):       true,
	paramAliasPayloadCaseKey("calendar +create", "summary"):                 true,
	paramAliasPayloadCaseKey("calendar +create", "description"):             true,
	paramAliasPayloadCaseKey("calendar +create", "user-ids"):                true,
	paramAliasPayloadCaseKey("calendar +create", "room-ids"):                true,
	paramAliasPayloadCaseKey("calendar +create", "room-id"):                 true,
	paramAliasPayloadCaseKey("calendar +create", "calendar-book-id"):        true,
	paramAliasPayloadCaseKey("calendar +create", "to"):                      true,
	paramAliasPayloadCaseKey("calendar +create", "from"):                    true,
	paramAliasPayloadCaseKey("calendar +get", "event-id"):                   true,
	paramAliasPayloadCaseKey("calendar +get", "calendar-book-id"):           true,
	paramAliasPayloadCaseKey("calendar +room-find", "from"):                 true,
	paramAliasPayloadCaseKey("calendar +room-find", "to"):                   true,
	paramAliasPayloadCaseKey("calendar +room-find", "page-size"):            true,
	paramAliasPayloadCaseKey("calendar +room-find", "page-index"):           true,
	paramAliasPayloadCaseKey("calendar +room-find", "room-group-id"):        true,
	paramAliasPayloadCaseKey("calendar +room-find", "query"):                true,
	paramAliasPayloadCaseKey("calendar +rsvp", "event-id"):                  true,
	paramAliasPayloadCaseKey("calendar +rsvp", "response-status"):           true,
	paramAliasPayloadCaseKey("calendar +search-event", "keyword"):           true,
	paramAliasPayloadCaseKey("calendar +search-event", "from"):              true,
	paramAliasPayloadCaseKey("calendar +search-event", "to"):                true,
	paramAliasPayloadCaseKey("calendar +search-event", "next-cursor"):       true,
	paramAliasPayloadCaseKey("calendar +search-event", "max-results"):       true,
	paramAliasPayloadCaseKey("calendar +suggestion", "user-ids"):            true,
	paramAliasPayloadCaseKey("calendar +suggestion", "duration-minutes"):    true,
	paramAliasPayloadCaseKey("calendar +suggestion", "from"):                true,
	paramAliasPayloadCaseKey("calendar +suggestion", "to"):                  true,
	paramAliasPayloadCaseKey("calendar +suggestion", "tz"):                  true,
	paramAliasPayloadCaseKey("calendar +update", "event-id"):                true,
	paramAliasPayloadCaseKey("calendar +update", "from"):                    true,
	paramAliasPayloadCaseKey("calendar +update", "summary"):                 true,
	paramAliasPayloadCaseKey("calendar +update", "description"):             true,
	paramAliasPayloadCaseKey("calendar +update", "add-user-ids"):            true,
	paramAliasPayloadCaseKey("calendar +update", "remove-user-ids"):         true,
}

// paramAliasCalendarConfirmationCases selects one newly reviewed alias for
// every Calendar Shortcut whose runtime contract requires user confirmation.
// The complete Calendar matrix proves confirmed canonical/alias payload
// equality; these representatives additionally prove semantic normalization
// cannot cross the confirmation boundary before the first transport call.
var paramAliasCalendarConfirmationCases = map[string]bool{
	paramAliasPayloadCaseKey("calendar +book", "summary"):          true,
	paramAliasPayloadCaseKey("calendar +cancel-event", "event-id"): true,
	paramAliasPayloadCaseKey("calendar +create", "summary"):        true,
	paramAliasPayloadCaseKey("calendar +invite", "id"):             true,
	paramAliasPayloadCaseKey("calendar +reschedule", "from"):       true,
	paramAliasPayloadCaseKey("calendar +rsvp", "response-status"):  true,
	paramAliasPayloadCaseKey("calendar +update", "event-id"):       true,
}

func TestCrossPlatformCoverageReviewedParamAliasesHaveCompleteTemplatesAndRepresentativeFinalPayloads(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	activeCommands := make(map[string]bool)
	activeFixtureCases := make(map[string]bool)
	activeCases := 0
	executedRepresentatives := make(map[string]bool)
	for _, fixture := range concepts.Fixture {
		if strings.HasPrefix(fixture.Expect, "did-you-mean:") {
			continue
		}
		activeCommands[fixture.Command] = true
		activeCases++
		caseKey := paramAliasPayloadCaseKey(fixture.Command, fixture.Emitted)
		activeFixtureCases[caseKey] = true
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

		if !paramAliasRepresentativePayloadCases[caseKey] && !paramAliasCandidateRepresentativePayloadCases[caseKey] {
			continue
		}
		executedRepresentatives[caseKey] = true
		t.Run(fixture.Command+"/"+fixture.Emitted, func(t *testing.T) {
			assertParamAliasFinalPayloadEquivalent(t, fixture.Command, canonicalArgs, aliasArgs)
		})
	}

	if activeCases == 0 {
		t.Fatal("reviewed fixture contains no active alias cases")
	}
	templateCommands := make(map[string]bool, len(paramAliasCompleteCommands)+len(paramAliasCandidateCompleteCommands))
	for command := range paramAliasCompleteCommands {
		if !activeCommands[command] {
			t.Errorf("complete-command E2E template %q has no active reviewed fixture", command)
		}
		templateCommands[command] = true
	}
	for command := range paramAliasCandidateCompleteCommands {
		if activeCommands[command] {
			templateCommands[command] = true
		}
	}
	for command := range activeCommands {
		if !templateCommands[command] {
			t.Errorf("active reviewed command %q has no complete-command E2E template", command)
		}
	}
	if len(activeCommands) != len(templateCommands) {
		t.Fatalf("complete-command coverage = %d templates for %d active commands (%d active cases)", len(templateCommands), len(activeCommands), activeCases)
	}
	for caseKey := range paramAliasRepresentativePayloadCases {
		if !executedRepresentatives[caseKey] {
			t.Errorf("representative final-payload case %q has no active reviewed fixture", caseKey)
		}
	}
	activeRepresentatives := len(paramAliasRepresentativePayloadCases)
	for caseKey := range paramAliasCandidateRepresentativePayloadCases {
		if !activeFixtureCases[caseKey] {
			continue
		}
		activeRepresentatives++
		if !executedRepresentatives[caseKey] {
			t.Errorf("candidate representative final-payload case %q was not executed", caseKey)
		}
	}
	if len(executedRepresentatives) != activeRepresentatives {
		t.Fatalf("representative final-payload coverage = %d, want %d", len(executedRepresentatives), activeRepresentatives)
	}
}

func TestCrossPlatformCoverageReviewedCalendarParamAliasesReachCanonicalEquivalentFinalPayloads(t *testing.T) {
	if os.Getenv(paramAliasCalendarPayloadChildEnv) != "1" {
		command := exec.Command(
			os.Args[0],
			"-test.run=^TestCrossPlatformCoverageReviewedCalendarParamAliasesReachCanonicalEquivalentFinalPayloads$",
			"-test.count=1",
			"-test.timeout=5m",
		)
		command.Env = append(os.Environ(), paramAliasCalendarPayloadChildEnv+"=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("Calendar param-alias payload subprocess failed: %v\n%s", err, strings.TrimSpace(string(output)))
		}
		return
	}

	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	executed := make(map[string]bool)
	executedConfirmation := make(map[string]bool)
	for _, fixture := range concepts.Fixture {
		caseKey := paramAliasPayloadCaseKey(fixture.Command, fixture.Emitted)
		if !paramAliasCalendarPayloadCases[caseKey] {
			continue
		}
		executed[caseKey] = true
		fixture := fixture
		t.Run(fixture.Command+"/"+fixture.Emitted, func(t *testing.T) {
			complete, ok := paramAliasCompleteCommand(fixture.Command, fixture.Expect)
			if !ok {
				t.Fatal("reviewed Calendar alias has no complete-command E2E template")
			}
			canonicalArgs := append([]string(nil), complete...)
			aliasArgs, replacements := replaceLongFlag(canonicalArgs, fixture.Expect, fixture.Emitted)
			if replacements != 1 {
				t.Fatalf("complete Calendar command must contain canonical --%s exactly once; replacements=%d args=%v", fixture.Expect, replacements, canonicalArgs)
			}
			assertParamAliasFinalPayloadEquivalent(t, fixture.Command, canonicalArgs, aliasArgs)
			if paramAliasCalendarConfirmationCases[caseKey] {
				executedConfirmation[caseKey] = true
				assertParamAliasCannotBypassConfirmation(t, aliasArgs)
			}
		})
	}

	for caseKey := range paramAliasCalendarPayloadCases {
		if !executed[caseKey] {
			t.Errorf("Calendar final-payload case %q has no active reviewed fixture", caseKey)
		}
	}
	if len(executed) != len(paramAliasCalendarPayloadCases) {
		t.Fatalf("Calendar final-payload coverage = %d, want %d", len(executed), len(paramAliasCalendarPayloadCases))
	}
	for caseKey := range paramAliasCalendarConfirmationCases {
		if !executedConfirmation[caseKey] {
			t.Errorf("Calendar confirmation case %q has no active reviewed fixture", caseKey)
		}
	}
	if len(executedConfirmation) != len(paramAliasCalendarConfirmationCases) {
		t.Fatalf("Calendar confirmation coverage = %d, want %d", len(executedConfirmation), len(paramAliasCalendarConfirmationCases))
	}
}

func assertParamAliasCannotBypassConfirmation(t *testing.T, aliasArgs []string) {
	t.Helper()
	unconfirmedArgs, removals := removeExactArg(aliasArgs, "--yes")
	if removals != 1 {
		t.Fatalf("confirmation template must contain --yes exactly once; removals=%d args=%v", removals, aliasArgs)
	}

	caller := &paramAliasCaptureCaller{}
	ctx, err := executeParamAliasPayloadE2E(t, caller, unconfirmedArgs...)
	if ctx == nil {
		t.Fatal("unconfirmed Calendar alias command skipped PreParse")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("unconfirmed Calendar alias command error = %#v, want confirmation_required\nargs=%v", err, unconfirmedArgs)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unconfirmed Calendar alias crossed the transport boundary: args=%v calls=%#v", unconfirmedArgs, caller.calls)
	}
}

// TestCrossPlatformCoverageReviewedProductTemplatedParamAliasesCannotBypassConfirmation
// exercises every distinct reviewed mutating complete-command template in the
// reviewed product expansions. The fixture gate already proves every
// alias resolves through PreParse; this gate removes the confirmation flag
// from one active alias invocation per distinct template and requires the
// runtime boundary to stop it before the first transport call. An explicit
// --dry-run is a reviewed preview path and must not carry a bypass flag.
func TestCrossPlatformCoverageReviewedProductTemplatedParamAliasesCannotBypassConfirmation(t *testing.T) {
	concepts, err := cli.LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}

	requiredTemplates := make(map[string]bool)
	coveredTemplates := make(map[string]bool)
	for _, fixture := range concepts.Fixture {
		if strings.HasPrefix(fixture.Expect, "did-you-mean:") {
			continue
		}
		product, _, _ := strings.Cut(fixture.Command, " ")
		switch product {
		case "attendance", "mail", "oa", "ding", "report", "sheet", "whiteboard", "markdown",
			"aisearch", "contact", "live", "devdoc", "hrbrain", "pat",
			"agoal", "audit", "dev", "devapp", "event", "mcp", "recruit":
		default:
			continue
		}
		complete, ok := paramAliasCompleteCommand(fixture.Command, fixture.Expect)
		if !ok {
			continue
		}
		_, yesCount := removeExactArg(complete, "--yes")
		_, userSayYesCount := removeExactArg(complete, "--user-say-yes")
		confirmationCount := yesCount + userSayYesCount
		confirmationArg := "--yes"
		if userSayYesCount == 1 {
			confirmationArg = "--user-say-yes"
		}
		_, dryRunCount := removeExactArg(complete, "--dry-run")
		if dryRunCount > 1 {
			t.Errorf("template must contain --dry-run at most once: command=%q args=%v", fixture.Command, complete)
			continue
		}
		if meta, exists := cli.ResolveMeta(fixture.Command); exists {
			switch meta.Safety.Confirmation {
			case "user_required":
				if dryRunCount == 1 {
					if confirmationCount != 0 {
						t.Errorf("Schema-confirmed dry-run template must not contain a confirmation bypass flag: command=%q args=%v", fixture.Command, complete)
					}
					continue
				}
				if confirmationCount != 1 {
					t.Errorf("Schema-confirmed template must contain exactly one reviewed confirmation flag: command=%q confirmation=%q args=%v", fixture.Command, meta.Safety.Confirmation, complete)
					continue
				}
			case "not_required":
				if confirmationCount != 0 {
					t.Errorf("Schema-unconfirmed template must not contain a confirmation bypass flag: command=%q confirmation=%q args=%v", fixture.Command, meta.Safety.Confirmation, complete)
					continue
				}
			}
		}
		if confirmationCount == 0 {
			continue
		}
		if confirmationCount != 1 {
			t.Errorf("confirmation template must contain exactly one reviewed confirmation flag: command=%q args=%v", fixture.Command, complete)
			continue
		}

		templateKey := fixture.Command + "\x00" + strings.Join(complete, "\x00")
		requiredTemplates[templateKey] = true
		if coveredTemplates[templateKey] {
			continue
		}
		aliasArgs, replacements := replaceLongFlag(complete, fixture.Expect, fixture.Emitted)
		if replacements != 1 {
			t.Errorf("confirmation template for %q/%q must contain canonical --%s exactly once; replacements=%d args=%v", fixture.Command, fixture.Emitted, fixture.Expect, replacements, complete)
			continue
		}
		coveredTemplates[templateKey] = true
		t.Run(fixture.Command+"/"+fixture.Emitted, func(t *testing.T) {
			assertTemplatedParamAliasCannotBypassConfirmation(t, fixture.Command, confirmationArg, aliasArgs)
		})
	}

	if len(requiredTemplates) == 0 {
		t.Fatal("reviewed complete-command templates contain no confirmation cases")
	}
	if len(coveredTemplates) != len(requiredTemplates) {
		t.Fatalf("templated confirmation coverage = %d, want %d", len(coveredTemplates), len(requiredTemplates))
	}
}

func assertTemplatedParamAliasCannotBypassConfirmation(t *testing.T, command, confirmationArg string, aliasArgs []string) {
	t.Helper()
	unconfirmedArgs, removals := removeExactArg(aliasArgs, confirmationArg)
	if removals != 1 {
		t.Fatalf("confirmation template must contain %s exactly once; removals=%d args=%v", confirmationArg, removals, aliasArgs)
	}

	caller := &paramAliasCaptureCaller{}
	ctx, err := executeParamAliasPayloadE2E(t, caller, unconfirmedArgs...)
	if ctx == nil {
		t.Fatal("unconfirmed alias command skipped PreParse")
	}
	var appErr *apperrors.Error
	if errors.As(err, &appErr) && appErr.Reason == "confirmation_required" {
		if len(caller.calls) != 0 {
			t.Fatalf("unconfirmed alias crossed the transport boundary before confirmation: args=%v calls=%#v", unconfirmedArgs, caller.calls)
		}
		return
	}
	t.Fatalf("unconfirmed alias command error = %#v, want confirmation_required\ncommand=%q args=%v calls=%#v", err, command, unconfirmedArgs, caller.calls)
}

func assertParamAliasFinalPayloadEquivalent(t *testing.T, command string, canonicalArgs, aliasArgs []string) {
	t.Helper()
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
	normalizeParamAliasVolatileDefaults(command, canonicalCaller, aliasCaller)
	if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
		t.Fatalf("final transport calls differ\ncanonical args: %v\nalias args: %v\ncanonical calls: %#v\nalias calls: %#v", canonicalArgs, aliasArgs, canonicalCaller.calls, aliasCaller.calls)
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

func TestCrossPlatformCoverageNewAITableDeleteDisableAliasesPreserveConfirmationAndPayload(t *testing.T) {
	coveredCommands := make(map[string]bool)
	reviewedAliases := make(map[string]string, len(paramAliasNewAITableDeleteDisableCases))
	for _, test := range paramAliasNewAITableDeleteDisableCases {
		test := test
		caseKey := paramAliasPayloadCaseKey(test.command, test.emitted)
		if previous, duplicate := reviewedAliases[caseKey]; duplicate {
			t.Fatalf("duplicate AITable delete/disable alias case %q: --%s and --%s", caseKey, previous, test.canonical)
		}
		reviewedAliases[caseKey] = test.canonical
		t.Run(test.command+"/"+test.emitted, func(t *testing.T) {
			complete, ok := paramAliasAITableDeleteDisableCompleteCommands[test.command]
			if !ok {
				t.Fatal("reviewed AITable delete/disable alias has no complete safety template")
			}
			coveredCommands[test.command] = true

			canonicalArgs := append([]string(nil), complete...)
			aliasArgs, replacements := replaceLongFlag(canonicalArgs, test.canonical, test.emitted)
			if replacements != 1 {
				t.Fatalf("complete command must contain canonical --%s exactly once; replacements=%d args=%v", test.canonical, replacements, canonicalArgs)
			}
			unconfirmedArgs, removals := removeExactArg(aliasArgs, "--yes")
			if removals != 1 {
				t.Fatalf("safety template must contain --yes exactly once; removals=%d args=%v", removals, aliasArgs)
			}

			entry, exists := cli.LookupParamAlias(test.command)
			target, active := entry.ResolveAlias(test.emitted)
			if !exists || !active || target != test.canonical {
				t.Fatalf("reviewed AITable alias --%s resolution = exists:%v active:%v target:%q, want --%s", test.emitted, exists, active, target, test.canonical)
			}

			unconfirmedCaller := &paramAliasCaptureCaller{}
			ctx, unconfirmedErr := executeParamAliasPayloadE2E(t, unconfirmedCaller, unconfirmedArgs...)
			if ctx == nil {
				t.Fatal("unconfirmed AITable alias command skipped PreParse")
			}
			var appErr *apperrors.Error
			if !errors.As(unconfirmedErr, &appErr) || appErr.Reason != "confirmation_required" {
				t.Fatalf("unconfirmed AITable alias command error = %#v, want confirmation_required\nargs=%v", unconfirmedErr, unconfirmedArgs)
			}
			if len(unconfirmedCaller.calls) != 0 {
				t.Fatalf("unconfirmed AITable alias crossed the transport boundary: args=%v calls=%#v", unconfirmedArgs, unconfirmedCaller.calls)
			}

			canonicalCaller := &paramAliasCaptureCaller{}
			_, canonicalErr := executeParamAliasPayloadE2E(t, canonicalCaller, canonicalArgs...)
			if canonicalErr != nil {
				t.Fatalf("confirmed canonical command failed: %v\nargs=%v\ncalls=%#v", canonicalErr, canonicalArgs, canonicalCaller.calls)
			}
			if len(canonicalCaller.calls) == 0 {
				t.Fatalf("confirmed canonical command reached no final transport payload: args=%v", canonicalArgs)
			}

			aliasCaller := &paramAliasCaptureCaller{}
			aliasCtx, aliasErr := executeParamAliasPayloadE2E(t, aliasCaller, aliasArgs...)
			if aliasErr != nil {
				t.Fatalf("confirmed alias command failed: %v\nargs=%v\ncalls=%#v", aliasErr, aliasArgs, aliasCaller.calls)
			}
			if aliasCtx == nil {
				t.Fatal("confirmed AITable alias command skipped PreParse")
			}
			if !reflect.DeepEqual(aliasCaller.calls, canonicalCaller.calls) {
				t.Fatalf("confirmed final transport calls differ\ncanonical args: %v\nalias args: %v\ncanonical calls: %#v\nalias calls: %#v", canonicalArgs, aliasArgs, canonicalCaller.calls, aliasCaller.calls)
			}
		})
	}

	for command := range paramAliasAITableDeleteDisableCompleteCommands {
		if !coveredCommands[command] {
			t.Errorf("AITable delete/disable safety template %q has no reviewed alias case", command)
		}
	}
	if len(coveredCommands) != len(paramAliasAITableDeleteDisableCompleteCommands) {
		t.Fatalf("AITable delete/disable safety coverage = %d commands, want %d", len(coveredCommands), len(paramAliasAITableDeleteDisableCompleteCommands))
	}

	activeAliases := 0
	for command := range paramAliasAITableDeleteDisableCompleteCommands {
		entry, exists := cli.LookupParamAlias(command)
		if !exists {
			t.Errorf("AITable delete/disable safety command %q has no generated alias entry", command)
			continue
		}
		for emitted, canonical := range entry.Aliases {
			activeAliases++
			caseKey := paramAliasPayloadCaseKey(command, emitted)
			reviewedCanonical, reviewed := reviewedAliases[caseKey]
			if !reviewed {
				t.Errorf("active AITable delete/disable alias %q --%s -> --%s has no confirmation/payload case", command, emitted, canonical)
				continue
			}
			if reviewedCanonical != canonical {
				t.Errorf("reviewed AITable delete/disable alias %q --%s target = --%s, generated --%s", command, emitted, reviewedCanonical, canonical)
			}
		}
	}
	if activeAliases != len(reviewedAliases) {
		t.Fatalf("AITable delete/disable generated alias coverage = %d, want %d reviewed cases", activeAliases, len(reviewedAliases))
	}
}

func TestCrossPlatformCoverageNewParamAliasesCannotBypassConfirmation(t *testing.T) {
	tests := append([]struct {
		command   string
		emitted   string
		canonical string
	}{}, paramAliasNewConfirmationCases...)
	for _, candidate := range paramAliasCandidateConfirmationCases {
		entry, exists := cli.LookupParamAlias(candidate.command)
		target, active := entry.ResolveAlias(candidate.emitted)
		if exists && active && target == candidate.canonical {
			tests = append(tests, candidate)
		}
	}

	for _, test := range tests {
		test := test
		t.Run(test.command+"/"+test.emitted, func(t *testing.T) {
			complete, ok := paramAliasCompleteCommand(test.command, test.canonical)
			if !ok {
				t.Fatal("reviewed confirmation alias has no complete-command E2E template")
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
				t.Fatalf("reviewed confirmation alias --%s resolution = exists:%v active:%v target:%q, want --%s", test.emitted, exists, active, target, test.canonical)
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
		return strings.Contains(err.Error(), "下载地址必须是合法的 HTTPS URL")
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
	if ok {
		return complete, true
	}
	complete, ok = paramAliasCandidateCompleteCommands[command]
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
