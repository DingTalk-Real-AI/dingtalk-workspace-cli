# dws Command Index

Every runtime command the `dws` CLI exposes when loaded with the **pre** environment configuration.

- **Source**: `dws-wukong/envelope/channel/open/pre/config.json`
- **Products**: 13
- **Total commands**: 159
- **Generated from**: `internal/compat.BuildDynamicCommands` rendering of the pre config — the same code path the CLI uses at runtime.

> Auto-generated. Edit `pre/config.json`, not this file.

## Global flags

Every command inherits these flags (documented here once, not repeated per command):

| Flag | Purpose |
|---|---|
| `--client-id` | Override OAuth client ID (DingTalk AppKey) |
| `--client-secret` | Override OAuth client secret (DingTalk AppSecret) |
| `--debug` | Enable debug logging |
| `--dry-run` | Preview the request without executing |
| `--fields` | Comma-separated output field projection |
| `-f, --format` | Output format: `json` \| `table` \| `raw` (default `json`) |
| `--jq` | jq expression applied to JSON output |
| `--mock` | Return mock data (developer aid) |
| `-o, --output` | Write output to a file |
| `--timeout` | HTTP request timeout in seconds (default 30) |
| `--token` | Override the configured API token |
| `-v, --verbose` | Verbose logging |
| `-y, --yes` | Skip confirmation prompts (AI-agent mode) |

## Contents

- [`dws aitable` — AI Tables](#dws-aitable) · 41 commands
- [`dws attendance` — Attendance](#dws-attendance) · 4 commands
- [`dws calendar` — Calendar](#dws-calendar) · 14 commands
- [`dws chat` — Group Chat / IM](#dws-chat) · 23 commands
- [`dws contact` — Contact Directory](#dws-contact) · 6 commands
- [`dws devdoc` — Open Platform Docs](#dws-devdoc) · 1 commands
- [`dws ding` — DING Messages](#dws-ding) · 2 commands
- [`dws doc` — DingTalk Doc](#dws-doc) · 21 commands
- [`dws drive` — DingTalk Drive](#dws-drive) · 6 commands
- [`dws minutes` — AI Minutes](#dws-minutes) · 19 commands
- [`dws oa` — OA Approval](#dws-oa) · 9 commands
- [`dws report` — Reports](#dws-report) · 7 commands
- [`dws todo` — Todo Tasks](#dws-todo) · 6 commands

## `dws aitable` — AI Tables

_AI-powered spreadsheet (Base) with datasheets, fields, records, views, dashboards, charts, import/export, attachments, and templates._

**41 commands**

| Command | Description | When to use |
|---|---|---|
| `dws aitable attachment upload` | Request an upload ticket for attaching a file to an AI table attachment-type field. Returns an upload URL and token the caller uses to stream the file. | When the agent needs to attach binary assets (images, PDFs, etc.) to records before creating or updating an attachment field value. |
| `dws aitable base create` | Create a new AI table (Base) under the current user's workspace. Returns the newly-created Base ID. | When an agent needs to provision a fresh Base before populating datasheets, fields, and records. |
| `dws aitable base delete` | Permanently delete an existing AI table (Base) by ID, removing all its datasheets, views, and records. | When the agent is cleaning up a Base that is no longer needed or was created for a one-off task. |
| `dws aitable base get` | Retrieve metadata for a single AI table (Base), including name, owner, and structural summary. | When the agent needs to inspect a specific Base before performing further operations on it. |
| `dws aitable base list` | List AI tables (Bases) accessible to the current user, paginated. | When the agent needs to enumerate the user's Bases to pick one by name or index. |
| `dws aitable base search` | Search AI tables (Bases) the current user can access by keyword against the Base name. | When the agent knows a partial Base name and needs to resolve it to a Base ID. |
| `dws aitable base update` | Update mutable properties of an AI table (Base), such as its name or icon. | When the agent needs to rename or rebrand an existing Base without touching its data. |
| `dws aitable chart create` | Create a new chart inside a Base, bound to a datasheet and view with a given configuration. | When the agent is building analytics on top of a datasheet and needs to materialize a chart visualization. |
| `dws aitable chart delete` | Delete a chart from a Base by chart ID. | When the agent needs to remove an obsolete or mistakenly-created chart. |
| `dws aitable chart get` | Retrieve a chart's full configuration and metadata. | When the agent needs to inspect an existing chart to clone it or adjust its configuration. |
| `dws aitable chart share get` | Retrieve the current public-sharing configuration of a chart, including share link and permissions. | When the agent needs to check whether a chart is already shared externally before issuing a link. |
| `dws aitable chart share update` | Enable, disable, or update the public-sharing configuration of a chart. | When the agent needs to generate or revoke an external share link for a chart. |
| `dws aitable chart update` | Update an existing chart's configuration (type, dimensions, metrics, style). | When the agent iterates on a chart's visualization after reviewing the initial result. |
| `dws aitable chart widgets-example` | Return a reference JSON example of chart widget configuration accepted by chart create/update. | When the agent needs a schema template before composing chart configuration payloads. |
| `dws aitable dashboard config-example` | Return a reference JSON example of dashboard configuration accepted by dashboard create/update. | When the agent needs a schema template before composing dashboard layout payloads. |
| `dws aitable dashboard create` | Create a new dashboard inside a Base with a layout of chart widgets. | When the agent wants to group multiple charts into a single dashboard view for a report or overview page. |
| `dws aitable dashboard delete` | Delete a dashboard from a Base by dashboard ID. | When the agent is removing an outdated dashboard. |
| `dws aitable dashboard get` | Retrieve a dashboard's layout, widget list, and metadata. | When the agent needs to inspect a dashboard before updating it or cloning it. |
| `dws aitable dashboard share get` | Retrieve the current public-sharing configuration of a dashboard. | When the agent needs to verify whether a dashboard has an active external share link. |
| `dws aitable dashboard share update` | Enable, disable, or update the public-sharing configuration of a dashboard. | When the agent needs to generate or revoke an external share link for a dashboard. |
| `dws aitable dashboard update` | Update an existing dashboard's layout, widgets, or metadata. | When the agent adds, removes, or rearranges charts on an existing dashboard. |
| `dws aitable export data` | Export data from a datasheet (optionally scoped to a view) to a downloadable file such as Excel or CSV. | When the agent needs to hand off Base data to an external system or deliver it as an attachment. |
| `dws aitable field create` | Create one or more fields in a datasheet with specified types and options. | When the agent is extending a datasheet's schema to capture new attributes. |
| `dws aitable field delete` | Delete a field from a datasheet by field ID; all values in that column are removed. | When the agent is cleaning up unused or deprecated columns in a datasheet. |
| `dws aitable field get` | Retrieve field definitions for a datasheet, including type, options, and order. | When the agent needs the field schema before constructing record payloads or queries. |
| `dws aitable field update` | Update a field's name, type, or options in a datasheet. | When the agent needs to rename a column or change its type/options without recreating it. |
| `dws aitable import data` | Import previously-uploaded data (e.g. Excel) into a datasheet as records, optionally creating fields. | When the agent is bulk-loading external data into a Base after a successful import upload. |
| `dws aitable import upload` | Request an upload ticket for an import file (Excel/CSV) to be staged before calling import data. | When the agent needs to push a local dataset into a Base and must first stage the file. |
| `dws aitable record create` | Insert one or more records into a datasheet with given field values. | When the agent needs to add new rows to a datasheet, individually or in batches. |
| `dws aitable record delete` | Delete one or more records from a datasheet by record ID. | When the agent removes rows that are obsolete or were created in error. |
| `dws aitable record query` | Query records from a datasheet with optional filters, sort, view scoping, and pagination. | When the agent needs to read row data to reason about it, render it, or feed it into downstream logic. |
| `dws aitable record update` | Update field values on one or more existing records by record ID. | When the agent modifies specific row values after reading or computing new data. |
| `dws aitable table create` | Create a new datasheet (table) inside a Base. | When the agent needs another table alongside existing ones in the same Base. |
| `dws aitable table delete` | Delete a datasheet from a Base by table ID, removing all its records, views, and fields. | When the agent is disposing of a datasheet that is no longer needed. |
| `dws aitable table get` | List datasheets within a Base, returning table IDs and names. | When the agent needs to resolve a table name to an ID inside a known Base. |
| `dws aitable table update` | Update a datasheet's name or other metadata. | When the agent needs to rename a datasheet without altering its contents. |
| `dws aitable template search` | Search the AI table template gallery by keyword. | When the agent needs to suggest or bootstrap from an existing Base template rather than building from scratch. |
| `dws aitable view create` | Create a new view (grid, gallery, kanban, etc.) on a datasheet. | When the agent needs an alternate filtered/sorted presentation of the same datasheet data. |
| `dws aitable view delete` | Delete a view from a datasheet by view ID. | When the agent is cleaning up unused views. |
| `dws aitable view get` | Retrieve view definitions for a datasheet, including filter, sort, and visible-field configuration. | When the agent needs to understand or reuse a view's configuration before querying records through it. |
| `dws aitable view update` | Update a view's name, filter, sort, grouping, or visible fields. | When the agent refines an existing view's configuration after inspection. |

## `dws attendance` — Attendance

_Attendance check-in records, shifts, and aggregate statistics._

**4 commands**

| Command | Description | When to use |
|---|---|---|
| `dws attendance record get` | Query a user's detailed clock-in/clock-out attendance records for a given time range. | When the agent needs to verify punctuality, pull attendance evidence, or build an attendance report for an individual. |
| `dws attendance rules` | Query the attendance group the user belongs to along with its attendance rules (schedule, locations, shifts). | When the agent needs to know the user's expected work schedule or attendance policies before interpreting records. |
| `dws attendance shift list` | Batch-query the assigned shifts for a set of employees over a date range. | When the agent needs to plan around team shifts or compile a shift-based roster. |
| `dws attendance summary` | Retrieve an aggregated attendance summary for a single user (totals of late, early-leave, absence, overtime). | When the agent needs a quick attendance health check without pulling raw records. |

## `dws calendar` — Calendar

_Calendar events, participants, meeting rooms, and busy-status queries._

**14 commands**

| Command | Description | When to use |
|---|---|---|
| `dws calendar busy search` | Query the busy/free time windows of one or more users over a given range. | When the agent is scheduling a meeting and needs to find a slot where all attendees are free. |
| `dws calendar event create` | Create a new calendar event on the user's calendar with title, time, attendees, and optional meeting room. | When the agent schedules a meeting or reminder on behalf of the user. |
| `dws calendar event delete` | Delete an existing calendar event by event ID. | When the agent cancels a previously scheduled event. |
| `dws calendar event get` | Retrieve the full details of a calendar event, including participants, location, and body. | When the agent needs to inspect an event before updating or referencing it. |
| `dws calendar event list` | List calendar events on the user's calendar within a given time range. | When the agent needs an overview of the user's upcoming schedule or a day's agenda. |
| `dws calendar event suggest` | Suggest candidate meeting time slots based on participants' busy/free data and constraints. | When the agent is coordinating a meeting and wants ranked time suggestions rather than raw busy data. |
| `dws calendar event update` | Update an existing calendar event's fields such as time, title, participants, or location. | When the agent needs to reschedule or amend a previously created event. |
| `dws calendar participant add` | Add one or more participants to an existing calendar event. | When the agent invites additional attendees after the event has been created. |
| `dws calendar participant delete` | Remove one or more participants from an existing calendar event. | When the agent drops attendees who no longer need to join the event. |
| `dws calendar participant list` | List current participants of a calendar event along with their response status. | When the agent needs to check who is attending before sending follow-up reminders. |
| `dws calendar room add` | Book a specific meeting room onto an existing calendar event. | When the agent needs to attach a physical meeting room to an already-scheduled event. |
| `dws calendar room delete` | Release a previously booked meeting room from a calendar event. | When the agent cancels or changes the room on an existing event. |
| `dws calendar room list-groups` | List meeting room groups (usually by building or floor) available to the user. | When the agent is narrowing down rooms by location before running an availability search. |
| `dws calendar room search` | Search meeting rooms by keyword within a group, optionally filtering to rooms free during a given window via `--available`. | When the agent needs to find a suitable room, typically free at a specific time, prior to booking. |

## `dws chat` — Group Chat / IM

_Group chats, conversations, messages, and robot/webhook integrations._

**23 commands**

| Command | Description | When to use |
|---|---|---|
| `dws chat bot search` | Search robots (bots) created by the current user by keyword. | When the agent needs to resolve one of its own bots by name to a robot code before sending bot messages. |
| `dws chat conversation-info` | Retrieve basic metadata for a conversation (single chat or group chat) by conversation ID. | When the agent needs context about a conversation (name, type, member count) before operating on it. |
| `dws chat group create` | Create a new internal group chat with a set of initial members. | When the agent needs to spin up a dedicated group for a new project, incident, or discussion thread. |
| `dws chat group members` | List members of a group chat; can also be used against the current user to enumerate their groups' members. | When the agent needs the roster of a group before mentioning, removing, or auditing members. |
| `dws chat group members add` | Add one or more users to an existing group chat. | When the agent expands a group to include additional participants. |
| `dws chat group members add-bot` | Add a robot (bot) to an existing group chat so the bot can post messages there. | When the agent needs to enable bot-driven notifications in a group that does not yet contain the bot. |
| `dws chat group members remove` | Remove one or more members from a group chat. | When the agent kicks users who should no longer have access to the group. |
| `dws chat group rename` | Update the display name of a group chat. | When the agent is rebranding or clarifying the purpose of an existing group. |
| `dws chat list-top-conversations` | Fetch the list of conversations the current user has pinned to the top of their chat list. | When the agent needs to prioritize the user's most important conversations in a summary or dashboard. |
| `dws chat message list` | Pull the recent message history of a specific conversation (v2), paginated. | When the agent needs to read what has recently been said in a conversation to summarize or reason about it. |
| `dws chat message list-all` | Search all messages across the current user's conversations within a time range. | When the agent needs to audit or summarize everything the user saw across chats in a window. |
| `dws chat message list-by-sender` | Fetch messages authored by a specific sender across both single and group chats. | When the agent needs to pull everything a particular colleague said recently. |
| `dws chat message list-focused` | Fetch messages from users the current user has marked as "special focus" (starred contacts). | When the agent builds a priority-inbox view highlighting messages from important people. |
| `dws chat message list-mentions` | Fetch messages where the current user was @-mentioned. | When the agent wants to surface items that explicitly require the user's attention. |
| `dws chat message list-topic-replies` | Pull replies under a specific group topic thread. | When the agent needs the conversation tree of a threaded discussion rather than the flat message list. |
| `dws chat message list-unread-conversations` | Fetch the list of conversations that currently have unread messages for the user. | When the agent builds a "catch me up" triage view of what still needs reading. |
| `dws chat message recall-by-bot` | Recall (retract) a message previously sent by a robot in a group chat. | When the agent sent a bot message in error or with incorrect content and needs to withdraw it. |
| `dws chat message search` | Search messages by keyword across the user's conversations. | When the agent needs to locate a specific statement or link the user remembers from chat history. |
| `dws chat message send` | Send a message into a group chat or single chat as the authenticated user. | When the agent needs to relay a response to a user or notify a group on behalf of the human operator. |
| `dws chat message send-by-bot` | Send a group message as a specific robot (bot) the user owns. | When the agent posts automated notifications under a bot identity rather than as the user. |
| `dws chat message send-by-webhook` | Send a group message via a custom-robot incoming webhook URL. | When the agent needs to post to a group using a webhook without requiring full bot-permission setup. |
| `dws chat search` | Search group conversations the user belongs to by group name keyword. | When the agent needs to resolve a group name to a conversation ID. |
| `dws chat search-common` | Find group chats the current user and a specified other user both belong to. | When the agent needs an existing shared channel to contact another user without creating a new group. |

## `dws contact` — Contact Directory

_Users, departments, and directory lookups._

**6 commands**

| Command | Description | When to use |
|---|---|---|
| `dws contact dept list-members` | List members of a specific department by department ID. | When the agent needs the roster of a department to target communication or build a team overview. |
| `dws contact dept search` | Search departments in the organization's contact directory by keyword. | When the agent needs to resolve a department name to a department ID. |
| `dws contact user get` | Batch-fetch detailed profile information for one or more users by user ID. | When the agent needs names, titles, emails, or departments for a known set of user IDs. |
| `dws contact user get-self` | Retrieve the profile of the currently authenticated user. | When the agent needs to identify who it is acting on behalf of (user ID, name, org). |
| `dws contact user search` | Search users in the contact directory by keyword (name, title, etc.). | When the agent needs to resolve a person's display name to a user ID. |
| `dws contact user search-mobile` | Look up a user by mobile phone number. | When the agent has only a phone number and needs to find the corresponding DingTalk user. |

## `dws devdoc` — Open Platform Docs

_Search the DingTalk Open Platform documentation._

**1 commands**

| Command | Description | When to use |
|---|---|---|
| `dws devdoc article search` | Search the DingTalk Open Platform documentation by keyword. | When the agent needs authoritative API reference or guides to answer a developer question. |

## `dws ding` — DING Messages

_Send and recall DING messages (priority notifications)._

**2 commands**

| Command | Description | When to use |
|---|---|---|
| `dws ding message recall` | Recall (retract) a previously sent DING message. | When the agent sent a DING in error and must withdraw it before recipients act on it. |
| `dws ding message send` | Send a DING message (high-priority notification) to one or more recipients via app/SMS/phone. | When the agent needs to page recipients with urgency beyond a normal chat message. |

## `dws doc` — DingTalk Doc

_DingTalk Doc: search, browse, read/write, upload/download, files, folders, blocks, comments._

**21 commands**

| Command | Description | When to use |
|---|---|---|
| `dws doc block delete` | Delete a block from a DingTalk Doc by block ID. | When the agent is editing a document and needs to remove a specific paragraph, table, or other block. |
| `dws doc block insert` | Insert a new block (paragraph, table, image, etc.) into a DingTalk Doc at a given position. | When the agent is programmatically assembling or editing a document's content. |
| `dws doc block list` | List the blocks of a DingTalk Doc with their IDs, types, and content. | When the agent needs the structured block tree of a doc before modifying specific blocks. |
| `dws doc block update` | Update the content or properties of an existing block in a DingTalk Doc. | When the agent amends a specific paragraph or element without rewriting the whole document. |
| `dws doc comment create` | Create a document-level comment on a DingTalk Doc. | When the agent leaves feedback or follow-up notes that apply to the entire document. |
| `dws doc comment create-inline` | Create an inline (anchored) comment on a specific text range within a DingTalk Doc. | When the agent needs to attach feedback to a particular passage rather than the whole doc. |
| `dws doc comment list` | List comments on a DingTalk Doc, including replies. | When the agent is reviewing outstanding feedback or summarizing comment threads. |
| `dws doc comment reply` | Reply to an existing comment on a DingTalk Doc. | When the agent responds to a reviewer's comment inline rather than starting a new thread. |
| `dws doc copy` | Copy an existing DingTalk Doc or file to a specified destination folder. | When the agent needs to duplicate a template document into a new location for reuse. |
| `dws doc create` | Create a new DingTalk Doc (document type) in a target folder or knowledge base. | When the agent needs a fresh DingTalk Doc to write into. |
| `dws doc download` | Download a DingTalk Doc or file to a local path. | When the agent needs the raw file locally for processing or attachment. |
| `dws doc file create` | Create a new file node of a given type (doc, sheet, mind map, whiteboard, AI table, etc.) in a target folder. | When the agent provisions any non-plain-document file type inside DingTalk Docs. |
| `dws doc folder create` | Create a new folder inside a DingTalk Docs knowledge base or drive location. | When the agent organizes output into a fresh folder before writing files into it. |
| `dws doc info` | Retrieve metadata for a document or file (title, type, owner, path, permissions). | When the agent needs descriptive info about a node without fetching its full content. |
| `dws doc list` | List the child nodes (files and subfolders) of a folder or knowledge base. | When the agent traverses the document hierarchy to find or enumerate items. |
| `dws doc move` | Move a DingTalk Doc or file to a different folder location. | When the agent reorganizes document structure. |
| `dws doc read` | Read the content of a DingTalk Doc as Markdown. | When the agent needs the document body as text for summarization, Q&A, or further editing. |
| `dws doc rename` | Rename a DingTalk Doc or file. | When the agent needs to change a document's title without altering its contents or location. |
| `dws doc search` | Search DingTalk Docs the user can access by keyword. | When the agent needs to locate a document by title or content before reading or editing it. |
| `dws doc update` | Update the content of a DingTalk Doc (bulk content rewrite rather than block-level edit). | When the agent has freshly generated content and needs to overwrite a doc's body. |
| `dws doc upload` | Obtain upload credentials and URL for uploading a local file as an attachment into DingTalk Docs or a knowledge base. | When the agent needs to stage a local file for attachment into the DingTalk Docs system. |

## `dws drive` — DingTalk Drive

_DingTalk Drive file and folder management._

**6 commands**

| Command | Description | When to use |
|---|---|---|
| `dws drive commit` | Commit a file upload to DingTalk Drive after the binary has been pushed to the presigned URL. | When the agent finalizes a Drive upload step; pairs with `drive upload-info`. |
| `dws drive download` | Fetch a temporary download URL for a file stored in DingTalk Drive. | When the agent needs to retrieve a Drive-hosted file for local use or for handing to another service. |
| `dws drive info` | Retrieve metadata for a file or folder in DingTalk Drive. | When the agent inspects a Drive node before downloading, moving, or listing around it. |
| `dws drive list` | List the files and subfolders of a DingTalk Drive folder. | When the agent needs to enumerate Drive contents to find or pick items. |
| `dws drive mkdir` | Create a new folder in DingTalk Drive. | When the agent organizes Drive output into a fresh folder before uploading files. |
| `dws drive upload-info` | Obtain a presigned upload URL and token for pushing a local file into DingTalk Drive. | When the agent starts a Drive upload; pairs with `drive commit` to finalize. |

## `dws minutes` — AI Minutes

_AI meeting notes: listing, summary, todos, transcription, recording control, mind maps, speakers, hot words, uploads._

**19 commands**

| Command | Description | When to use |
|---|---|---|
| `dws minutes get batch` | Batch-fetch detailed metadata for multiple meeting notes (AI minutes) by ID. | When the agent needs to enrich a list of minutes IDs with titles, durations, and participants in one call. |
| `dws minutes get info` | Retrieve basic metadata for a single meeting note (title, owner, time, duration, participants). | When the agent needs a header view of a specific meeting note. |
| `dws minutes get keywords` | Retrieve the extracted keywords of a meeting note. | When the agent needs topical tags for a meeting without pulling the full transcript or summary. |
| `dws minutes get summary` | Retrieve the AI-generated summary of a meeting note. | When the agent needs a concise recap of a meeting for reporting or follow-up. |
| `dws minutes get todos` | Retrieve the action items (todos) extracted from a meeting note. | When the agent needs to convert meeting action items into tasks or follow up on commitments. |
| `dws minutes get transcription` | Retrieve the raw speech-to-text transcription of a meeting note. | When the agent needs the full verbatim transcript for deep analysis or quoting. |
| `dws minutes hot-word add` | Add a custom personal hot word to improve future speech-recognition accuracy on the user's minutes. | When the user has domain-specific jargon or proper nouns that the ASR model mistranscribes. |
| `dws minutes list all` | List all meeting notes the user has access to, filterable by keyword and time range. | When the agent needs a broad search across the user's full minutes library. |
| `dws minutes list mine` | List only the meeting notes the current user created. | When the agent scopes results to the user's own recordings rather than shared ones. |
| `dws minutes list shared` | List meeting notes that have been shared with the current user by others. | When the agent wants to surface meetings the user is an invited viewer of. |
| `dws minutes mind-graph create` | Generate a mind map from a meeting note asynchronously. | When the agent wants a structured mind-map visualization of a meeting's content. |
| `dws minutes mind-graph status` | Query the generation status of a mind-map job and fetch the result when ready. | When the agent polls after `mind-graph create` to retrieve the finished mind map. |
| `dws minutes replace-text` | Find and replace matching text across a meeting note's transcript paragraphs and summary. | When the agent corrects a systemic transcription mistake (e.g. wrong product name) throughout a note. |
| `dws minutes speaker replace` | Reassign speaker labels in a meeting note (e.g. map "Speaker 1" to a specific user). | When the agent cleans up speaker diarization after automatic labels came out wrong. |
| `dws minutes update summary` | Overwrite the summary content of a meeting note. | When the agent refines or replaces the AI-generated summary with a corrected or customized version. |
| `dws minutes update title` | Update the title of a meeting note. | When the agent renames a meeting note for clarity before sharing or archiving. |
| `dws minutes upload cancel` | Cancel an in-progress meeting-note file upload session. | When the agent aborts a multi-step upload due to user cancellation or upstream error. |
| `dws minutes upload complete` | Complete an upload session and create a meeting note from the uploaded audio/video. | When the agent finalizes a minutes upload, triggering transcription and AI processing. |
| `dws minutes upload create` | Create a file upload session for producing a meeting note from a local audio/video file. | When the agent begins uploading a recording to be turned into a meeting note. |

## `dws oa` — OA Approval

_OA approval workflows: list, approve, reject, revoke, records._

**9 commands**

| Command | Description | When to use |
|---|---|---|
| `dws oa approval approve` | Approve a pending approval process instance (task) as the current user. | When the agent acts on a pending approval the user has delegated it to handle. |
| `dws oa approval detail` | Retrieve full details of an approval process instance, including form fields, attachments, and state. | When the agent needs to read the content of an approval ticket before deciding on it or summarizing it. |
| `dws oa approval list-forms` | List approval process templates (forms) the current user is allowed to initiate. | When the agent needs to pick the right approval form before submitting a new request. |
| `dws oa approval list-initiated` | List approval process instances the current user has initiated. | When the agent reviews the status of approvals the user submitted. |
| `dws oa approval list-pending` | List approval process instances currently awaiting action from the current user. | When the agent surfaces "needs your approval" items in the user's inbox. |
| `dws oa approval records` | Retrieve the operation history (who approved/commented/transferred, when) of an approval instance. | When the agent explains an approval's progression or audits who handled it. |
| `dws oa approval reject` | Reject a pending approval process instance as the current user. | When the agent declines an approval on behalf of the user, optionally with a reason. |
| `dws oa approval revoke` | Revoke an approval process instance previously initiated by the current user. | When the agent withdraws an approval request the user no longer wants to pursue. |
| `dws oa approval tasks` | List pending approval task IDs assigned to the current user, used to drive approve/reject actions. | When the agent needs task IDs (not just instance IDs) before calling approve/reject. |

## `dws report` — Reports

_DingTalk Report feature: templates, entries, and statistics._

**7 commands**

| Command | Description | When to use |
|---|---|---|
| `dws report create` | Create a new report (DingTalk "Report" entry) based on a report template with filled-in content. | When the agent submits a daily/weekly report on behalf of the user. |
| `dws report detail` | Retrieve the full details of a specific report entry, including fields and recipients. | When the agent needs to read a report's content for summarization or follow-up. |
| `dws report list` | List reports the current user has received from others. | When the agent digests the user's incoming reports (e.g. team members' weeklies). |
| `dws report sent` | List reports the current user has created and sent out. | When the agent reviews the user's own reporting history. |
| `dws report stats` | Retrieve aggregated statistics for a report entry by ID (views, likes, comments, etc.). | When the agent measures engagement or reach of a report the user sent. |
| `dws report template detail` | Retrieve the detailed schema of a report template by name, including required fields. | When the agent needs to know a template's field structure before calling `report create`. |
| `dws report template list` | List the report templates the current user is allowed to use. | When the agent picks the correct report template (e.g. "weekly", "daily") before creating a report. |

## `dws todo` — Todo Tasks

_Personal todo task management._

**6 commands**

| Command | Description | When to use |
|---|---|---|
| `dws todo task create` | Create a personal todo item for the current user with title, due time, and optional executors. | When the agent captures an action item as a tracked todo in the user's DingTalk todo list. |
| `dws todo task delete` | Delete a todo item by ID. | When the agent removes a todo that is no longer relevant. |
| `dws todo task done` | Update the completion status of a todo's executor (mark done or undone). | When the agent marks an action item as completed after confirming the work is finished. |
| `dws todo task get` | Retrieve the full details of a todo item by ID. | When the agent inspects a specific todo's content, due date, and executors. |
| `dws todo task list` | List todos for the current user within the current organization. | When the agent surfaces the user's outstanding tasks or builds a daily focus list. |
| `dws todo task update` | Update a todo's title, description, due time, or executors. | When the agent edits an existing todo after new information comes in. |

