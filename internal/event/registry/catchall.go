// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package registry

// CatchAllEventTypes returns the default event-type wildcards passed in
// the Hello frame when `dws event consume` is invoked without
// --event-types.
//
// v1 returns nil = "bus-wide catch-all" — the bus already subscribes
// upstream to everything the open-platform UI ticks. The Hub's matcher
// treats nil/empty as match-everything, so the consumer gets every event
// the open platform pushes without needing an explicit list.
//
// Why we don't ship a curated default (yet): the exact event_type
// strings DingTalk emits over Stream are not yet confirmed by a P0
// run against a real app (escape-hatch row #3). Shipping a curated
// list that differs by one character from what DingTalk actually sends
// would silently filter out events users expect to see. The bus-wide
// catch-all behaviour is conservative — too inclusive rather than too
// exclusive — and avoids that failure mode.
//
// Known/expected event type strings (from open-platform docs; pending
// P0 SDK confirmation — DO NOT enable until verified):
//
//	im.message.receive_v1                  receive any IM message
//	im.message.read_v1                     message read receipt
//	im.message.reaction.created_v1         reaction added
//	im.message.reaction.deleted_v1         reaction removed
//	im.chat.member.bot.added_v1            bot added to a chat
//	im.chat.member.bot.deleted_v1          bot removed from a chat
//	im.chat.member.user.added_v1           user added to a chat
//	im.chat.disbanded_v1                   chat disbanded
//	contact.user.created_v3 / updated / deleted
//	contact.department.created_v3 / updated / deleted
//	cal.event.created_v1 / updated / deleted
//	approval.instance.status_changed
//	approval.task.created
//	attendance.check_v1
//
// Once verified, switch CatchAllEventTypes to return the slice above (and
// document the change so users can override with --event-types '*' to
// regain literal "everything the open platform sends").
func CatchAllEventTypes() []string { return nil }
