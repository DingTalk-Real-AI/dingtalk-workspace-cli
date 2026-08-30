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

package personal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// MessageEventOutput is the stable business-facing output for personal
// message receive events. Schema output is generated from these tags so the
// documented fields cannot drift from the values written by consume.
type MessageEventOutput struct {
	Type                 string                `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID              string                `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp            int64                 `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID          string                `json:"subscribe_id" description:"订阅 ID"`
	MessageID            string                `json:"message_id" description:"开放消息 ID" format:"open_message_id"`
	ConversationID       string                `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Sender               string                `json:"sender" description:"发送人展示名"`
	SenderOpenDingTalkID string                `json:"sender_open_dingtalk_id" description:"发送人开放 ID" format:"open_dingtalk_id"`
	Content              string                `json:"content" description:"消息正文"`
	CreateTime           string                `json:"create_time" description:"消息创建时间"`
	EventTime            int64                 `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
	QuotedMessage        *MessageEventContext  `json:"quoted_message,omitempty" description:"引用回复所引用的原消息；非引用回复时不输出"`
	ForwardMessages      []MessageEventContext `json:"forward_messages,omitempty" description:"合并转发包含的原消息列表；非合并转发时不输出"`
}

// MessageEventContext preserves the business context nested under a quoted
// reply or merged-forward message. Keep these fields structured instead of
// parsing the localized outer content summary.
type MessageEventContext struct {
	MessageID            string `json:"message_id" description:"内部消息的开放消息 ID" format:"open_message_id"`
	ConversationID       string `json:"conversation_id" description:"内部消息原来所在的会话 ID" format:"open_conversation_id"`
	Sender               string `json:"sender" description:"内部消息发送人展示名；服务端未提供时可能为空或为 null 字符串"`
	SenderOpenDingTalkID string `json:"sender_open_dingtalk_id" description:"内部消息发送人开放 ID；服务端未提供时为空" format:"open_dingtalk_id"`
	Content              string `json:"content" description:"内部消息正文；媒体消息可能包含 mediaId 等下载定位信息"`
	CreateTime           string `json:"create_time" description:"内部消息创建时间"`
}

type ReadEventOutput struct {
	Type                 string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID              string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp            int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID          string `json:"subscribe_id" description:"订阅 ID"`
	MessageID            string `json:"message_id" description:"被读取消息的开放消息 ID" format:"open_message_id"`
	ConversationID       string `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Reader               string `json:"reader" description:"读取消息的用户展示名"`
	ReaderOpenDingTalkID string `json:"reader_open_dingtalk_id" description:"读取消息的用户开放 ID" format:"open_dingtalk_id"`
	Sender               string `json:"sender" description:"原消息发送人展示名"`
	SenderOpenDingTalkID string `json:"sender_open_dingtalk_id" description:"原消息发送人开放 ID" format:"open_dingtalk_id"`
	ReadTime             string `json:"read_time" description:"消息读取时间"`
	EventTime            int64  `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
}

type RecallEventOutput struct {
	Type                   string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID                string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp              int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID            string `json:"subscribe_id" description:"订阅 ID"`
	MessageID              string `json:"message_id" description:"被撤回消息的开放消息 ID" format:"open_message_id"`
	ConversationID         string `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Recaller               string `json:"recaller" description:"撤回消息的用户展示名"`
	RecallerOpenDingTalkID string `json:"recaller_open_dingtalk_id" description:"撤回消息的用户开放 ID" format:"open_dingtalk_id"`
	Sender                 string `json:"sender" description:"原消息发送人展示名"`
	SenderOpenDingTalkID   string `json:"sender_open_dingtalk_id" description:"原消息发送人开放 ID" format:"open_dingtalk_id"`
	RecallTime             string `json:"recall_time" description:"消息撤回时间"`
	EventTime              int64  `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
}

type ReactionEventOutput struct {
	Type                   string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID                string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp              int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID            string `json:"subscribe_id" description:"订阅 ID"`
	MessageID              string `json:"message_id" description:"收到表情回应的开放消息 ID" format:"open_message_id"`
	ConversationID         string `json:"conversation_id" description:"会话 ID" format:"open_conversation_id"`
	Operator               string `json:"operator" description:"执行表情回应操作的用户展示名"`
	OperatorOpenDingTalkID string `json:"operator_open_dingtalk_id" description:"执行表情回应操作的用户开放 ID" format:"open_dingtalk_id"`
	ReactionName           string `json:"reaction_name" description:"表情回应名称"`
	ReactionText           string `json:"reaction_text" description:"表情回应文本"`
	OperationType          string `json:"operation_type" description:"表情回应操作类型"`
	OperationTime          string `json:"operation_time" description:"表情回应操作时间"`
	Sender                 string `json:"sender" description:"原消息发送人展示名"`
	SenderOpenDingTalkID   string `json:"sender_open_dingtalk_id" description:"原消息发送人开放 ID" format:"open_dingtalk_id"`
	EventTime              int64  `json:"event_time" description:"消息事件时间戳" format:"timestamp_ms"`
}

type baseEventOutput struct {
	Type        string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID     string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp   int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID string `json:"subscribe_id" description:"订阅 ID"`
}

// GroupLifecycleEventOutput is intentionally conservative until stable group
// event payload samples are available. Payload keeps unknown business fields
// while transport identity and routing metadata remain available only in raw
// output.
type GroupLifecycleEventOutput struct {
	Type        string         `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID     string         `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp   int64          `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID string         `json:"subscribe_id" description:"订阅 ID"`
	Payload     map[string]any `json:"payload" description:"群生命周期事件业务数据，字段以服务端实际推送为准" additional_properties:"true"`
}

type OAApprovalTaskCreatedOutput struct {
	Type              string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID           string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp         int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID       string `json:"subscribe_id" description:"订阅 ID"`
	ProcessInstanceID string `json:"process_instance_id" description:"审批实例 ID"`
	ProcessCode       string `json:"process_code" description:"审批流程模板编码"`
	TaskID            string `json:"task_id" description:"审批任务 ID"`
	Title             string `json:"title" description:"审批标题"`
	Status            string `json:"status" description:"审批任务状态"`
	CreateTime        int64  `json:"create_time" description:"审批任务创建时间" format:"timestamp_ms"`
	EventTime         int64  `json:"event_time" description:"审批任务事件业务时间" format:"timestamp_ms"`
}

type OAApprovalTaskFinishedOutput struct {
	Type              string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID           string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp         int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID       string `json:"subscribe_id" description:"订阅 ID"`
	ProcessInstanceID string `json:"process_instance_id" description:"审批实例 ID"`
	ProcessCode       string `json:"process_code" description:"审批流程模板编码"`
	TaskID            string `json:"task_id" description:"审批任务 ID"`
	Title             string `json:"title" description:"审批标题"`
	Status            string `json:"status" description:"审批任务状态"`
	Result            string `json:"result" description:"审批任务处理结果，值以服务端实际推送为准"`
	CreateTime        int64  `json:"create_time" description:"审批任务创建时间" format:"timestamp_ms"`
	FinishTime        int64  `json:"finish_time" description:"审批任务完成时间" format:"timestamp_ms"`
	EventTime         int64  `json:"event_time" description:"审批任务事件业务时间" format:"timestamp_ms"`
}

type OAApprovalTaskRedirectedOutput struct {
	Type              string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID           string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp         int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID       string `json:"subscribe_id" description:"订阅 ID"`
	ProcessInstanceID string `json:"process_instance_id" description:"审批实例 ID"`
	ProcessCode       string `json:"process_code" description:"审批流程模板编码"`
	TaskID            string `json:"task_id" description:"原审批任务 ID"`
	Title             string `json:"title" description:"审批标题"`
	Status            string `json:"status" description:"原审批任务状态"`
	Result            string `json:"result" description:"审批任务转交结果，值以服务端实际推送为准"`
	CreateTime        int64  `json:"create_time" description:"原审批任务创建时间" format:"timestamp_ms"`
	FinishTime        int64  `json:"finish_time" description:"原审批任务转交完成时间" format:"timestamp_ms"`
	EventTime         int64  `json:"event_time" description:"审批任务转交事件业务时间" format:"timestamp_ms"`
}

type OAApprovalInstanceStartedOutput struct {
	Type              string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID           string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp         int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID       string `json:"subscribe_id" description:"订阅 ID"`
	ProcessInstanceID string `json:"process_instance_id" description:"审批实例 ID"`
	ProcessCode       string `json:"process_code" description:"审批流程模板编码"`
	Title             string `json:"title" description:"审批标题"`
	Status            string `json:"status" description:"审批实例状态"`
	CreateTime        int64  `json:"create_time" description:"审批实例创建时间" format:"timestamp_ms"`
	EventTime         int64  `json:"event_time" description:"审批实例事件业务时间" format:"timestamp_ms"`
}

type OAApprovalInstanceCCOutput struct {
	Type              string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID           string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp         int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID       string `json:"subscribe_id" description:"订阅 ID"`
	ProcessInstanceID string `json:"process_instance_id" description:"审批实例 ID"`
	ProcessCode       string `json:"process_code" description:"审批流程模板编码"`
	Title             string `json:"title" description:"审批标题"`
	Status            string `json:"status" description:"审批实例到达抄送节点时的状态"`
	CreateTime        int64  `json:"create_time" description:"审批实例创建时间" format:"timestamp_ms"`
	EventTime         int64  `json:"event_time" description:"审批抄送事件业务时间" format:"timestamp_ms"`
}

type OAApprovalInstanceTerminatedOutput struct {
	Type              string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID           string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp         int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID       string `json:"subscribe_id" description:"订阅 ID"`
	ProcessInstanceID string `json:"process_instance_id" description:"审批实例 ID"`
	ProcessCode       string `json:"process_code" description:"审批流程模板编码"`
	Title             string `json:"title" description:"审批标题"`
	Status            string `json:"status" description:"审批实例状态"`
	CreateTime        int64  `json:"create_time" description:"审批实例创建时间" format:"timestamp_ms"`
	FinishTime        int64  `json:"finish_time" description:"审批实例终止时间" format:"timestamp_ms"`
	EventTime         int64  `json:"event_time" description:"审批实例终止事件业务时间" format:"timestamp_ms"`
}

type OAApprovalInstanceFinishedOutput struct {
	Type              string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID           string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp         int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID       string `json:"subscribe_id" description:"订阅 ID"`
	ProcessInstanceID string `json:"process_instance_id" description:"审批实例 ID"`
	ProcessCode       string `json:"process_code" description:"审批流程模板编码"`
	Title             string `json:"title" description:"审批标题"`
	Status            string `json:"status" description:"审批实例状态"`
	Result            string `json:"result" description:"审批实例处理结果，值以服务端实际推送为准"`
	CreateTime        int64  `json:"create_time" description:"审批实例创建时间" format:"timestamp_ms"`
	FinishTime        int64  `json:"finish_time" description:"审批实例完成时间" format:"timestamp_ms"`
	EventTime         int64  `json:"event_time" description:"审批实例事件业务时间" format:"timestamp_ms"`
}

type TodoTaskCreatedOutput struct {
	Type            string   `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID         string   `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp       int64    `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID     string   `json:"subscribe_id" description:"订阅 ID"`
	TaskID          string   `json:"task_id" description:"待办任务 ID"`
	Subject         string   `json:"subject" description:"待办标题"`
	CreatorID       string   `json:"creator_id" description:"创建者 staffId"`
	ExecutorIDs     []string `json:"executor_ids" description:"执行者 staffId 列表"`
	ParticipantIDs  []string `json:"participant_ids" description:"参与者 staffId 列表"`
	Priority        int64    `json:"priority" description:"待办优先级"`
	StatusStage     int64    `json:"status_stage" description:"状态阶段：0 未开始、1 进行中、2 正常完成、3 异常完成"`
	PlanStartDate   *int64   `json:"plan_start_date,omitempty" description:"计划开始时间" format:"timestamp_ms"`
	PlanFinishDate  *int64   `json:"plan_finish_date,omitempty" description:"计划结束时间" format:"timestamp_ms"`
	StartDate       *int64   `json:"start_date,omitempty" description:"实际开始时间" format:"timestamp_ms"`
	FinishDate      *int64   `json:"finish_date,omitempty" description:"实际结束时间" format:"timestamp_ms"`
	Description     string   `json:"description" description:"待办描述"`
	Source          string   `json:"source" description:"待办来源"`
	SourceID        string   `json:"source_id" description:"来源业务 ID"`
	BizTag          string   `json:"biz_tag" description:"业务标识"`
	ParentID        *string  `json:"parent_id,omitempty" description:"父任务 ID"`
	IsMultiExecutor bool     `json:"is_multi_executor" description:"是否多执行者待办"`
	SceneType       string   `json:"scene_type" description:"待办场景类型"`
	CreateTime      int64    `json:"create_time" description:"待办创建时间" format:"timestamp_ms"`
}

type TodoTaskUpdatedOutput struct {
	Type            string   `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID         string   `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp       int64    `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID     string   `json:"subscribe_id" description:"订阅 ID"`
	TaskID          string   `json:"task_id" description:"待办任务 ID"`
	Subject         string   `json:"subject" description:"待办标题"`
	CreatorID       string   `json:"creator_id" description:"创建者 staffId"`
	ExecutorIDs     []string `json:"executor_ids" description:"执行者 staffId 列表"`
	ParticipantIDs  []string `json:"participant_ids" description:"参与者 staffId 列表"`
	Priority        int64    `json:"priority" description:"待办优先级"`
	StatusStage     int64    `json:"status_stage" description:"新状态阶段：0 未开始、1 进行中、2 正常完成、3 异常完成"`
	OldStatusStage  int64    `json:"old_status_stage" description:"更新前状态阶段"`
	PlanStartDate   *int64   `json:"plan_start_date,omitempty" description:"计划开始时间" format:"timestamp_ms"`
	PlanFinishDate  *int64   `json:"plan_finish_date,omitempty" description:"计划结束时间" format:"timestamp_ms"`
	StartDate       *int64   `json:"start_date,omitempty" description:"实际开始时间" format:"timestamp_ms"`
	FinishDate      *int64   `json:"finish_date,omitempty" description:"实际结束时间" format:"timestamp_ms"`
	Description     string   `json:"description" description:"待办描述"`
	Source          string   `json:"source" description:"待办来源"`
	SourceID        string   `json:"source_id" description:"来源业务 ID"`
	BizTag          string   `json:"biz_tag" description:"业务标识"`
	ParentID        *string  `json:"parent_id,omitempty" description:"父任务 ID"`
	IsMultiExecutor bool     `json:"is_multi_executor" description:"是否多执行者待办"`
	SceneType       string   `json:"scene_type" description:"待办场景类型"`
	CreateTime      int64    `json:"create_time" description:"待办创建时间" format:"timestamp_ms"`
	UpdateTime      int64    `json:"update_time" description:"待办更新时间" format:"timestamp_ms"`
}

type TodoTaskDeletedOutput struct {
	Type        string `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID     string `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp   int64  `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID string `json:"subscribe_id" description:"订阅 ID"`
	TaskID      string `json:"task_id" description:"待办任务 ID"`
	Subject     string `json:"subject" description:"被删除的待办标题"`
	CreatorID   string `json:"creator_id" description:"创建者 staffId"`
	CreateTime  int64  `json:"create_time" description:"待办创建时间" format:"timestamp_ms"`
	DeleteTime  int64  `json:"delete_time" description:"待办删除时间" format:"timestamp_ms"`
}

type GroupMemberEventOutput struct {
	Type                   string                   `json:"type" description:"事件类型，固定为当前 event_key"`
	EventID                string                   `json:"event_id" description:"事件 ID，可用于去重"`
	Timestamp              int64                    `json:"timestamp" description:"事件发生时间戳" format:"timestamp_ms"`
	SubscribeID            string                   `json:"subscribe_id" description:"订阅 ID"`
	ConversationID         string                   `json:"conversation_id" description:"发生成员变更的群会话 ID" format:"open_conversation_id"`
	Operator               string                   `json:"operator" description:"执行成员变更操作的用户展示名，系统操作或成员自行退出时可能为空"`
	OperatorOpenDingTalkID string                   `json:"operator_open_dingtalk_id" description:"执行成员变更操作的用户开放 ID，系统操作或成员自行退出时可能为空" format:"open_dingtalk_id"`
	Members                []GroupMemberEventMember `json:"members" description:"本次加入或退出的成员列表"`
	EventTime              int64                    `json:"event_time" description:"群成员变更事件时间戳" format:"timestamp_ms"`
}

type GroupMemberEventMember struct {
	Nick           string `json:"nick" description:"成员展示名"`
	OpenDingTalkID string `json:"open_dingtalk_id" description:"成员开放 ID" format:"open_dingtalk_id"`
}

type personalEventData struct {
	EventID      string          `json:"eventId"`
	EventKey     string          `json:"eventKey"`
	OccurredAtMS int64           `json:"occurredAtMs"`
	SubID        string          `json:"subId"`
	Payload      json.RawMessage `json:"payload"`
}

type personalMessagePayload struct {
	EventTime int64 `json:"event_time"`
	Body      struct {
		CreateTime           string                   `json:"createTime"`
		Sender               string                   `json:"sender"`
		OpenMessageID        string                   `json:"openMessageId"`
		SenderOpenDingTalkID string                   `json:"senderOpenDingTalkId"`
		OpenConversationID   string                   `json:"openConversationId"`
		Content              string                   `json:"content"`
		QuotedMessage        *personalMessageContext  `json:"quotedMessage"`
		ForwardMessages      []personalMessageContext `json:"forwardMessages"`
	} `json:"body"`
}

type personalMessageContext struct {
	CreateTime           string `json:"createTime"`
	Sender               string `json:"sender"`
	OpenMessageID        string `json:"openMessageId"`
	SenderOpenDingTalkID string `json:"senderOpenDingTalkId"`
	OpenConversationID   string `json:"openConversationId"`
	Content              string `json:"content"`
}

type personalReadPayload struct {
	EventTime int64 `json:"event_time"`
	Body      struct {
		MessageID            string `json:"openMessageId"`
		ConversationID       string `json:"openConversationId"`
		Reader               string `json:"reader"`
		ReaderOpenDingTalkID string `json:"readerOpenDingTalkId"`
		Sender               string `json:"sender"`
		SenderOpenDingTalkID string `json:"senderOpenDingTalkId"`
		ReadTime             string `json:"msgReadTime"`
	} `json:"body"`
}

type personalRecallPayload struct {
	EventTime int64 `json:"event_time"`
	Body      struct {
		MessageID              string `json:"openMessageId"`
		ConversationID         string `json:"openConversationId"`
		Recaller               string `json:"recaller"`
		RecallerOpenDingTalkID string `json:"recallerOpenDingTalkId"`
		Sender                 string `json:"sender"`
		SenderOpenDingTalkID   string `json:"senderOpenDingTalkId"`
		RecallTime             string `json:"msgRecallTime"`
	} `json:"body"`
}

type personalReactionPayload struct {
	EventTime int64                `json:"event_time"`
	Body      personalReactionBody `json:"body"`
}

type personalReactionBody struct {
	MessageID              string `json:"openSourceMessageId"`
	ConversationID         string `json:"openConversationId"`
	Operator               string `json:"oper"`
	OperatorOpenDingTalkID string `json:"-"`
	ReactionName           string `json:"emotionName"`
	ReactionText           string `json:"emotionText"`
	OperationType          string `json:"operateType"`
	OperationTime          string `json:"operateTime"`
	Sender                 string `json:"sender"`
	SenderOpenDingTalkID   string `json:"senderOpenDingTalkId"`
}

type personalGroupMemberPayload struct {
	EventTime int64                   `json:"event_time"`
	Body      personalGroupMemberBody `json:"body"`
}

type personalGroupMemberBody struct {
	ConversationID         string                      `json:"openConversationId"`
	Operator               string                      `json:"operNick"`
	OperatorOpenDingTalkID string                      `json:"-"`
	Members                []personalGroupMemberRecord `json:"members"`
}

type personalGroupMemberRecord struct {
	Nick           string `json:"nick"`
	OpenDingTalkID string `json:"openDingTalkId"`
}

type personalOAApprovalPayload struct {
	EventTime int64                  `json:"event_time"`
	Body      personalOAApprovalBody `json:"body"`
}

type personalOAApprovalBody struct {
	ProcessInstanceID string `json:"processInstanceId"`
	ProcessCode       string `json:"processCode"`
	TaskID            string `json:"taskId"`
	Title             string `json:"title"`
	Status            string `json:"status"`
	Result            string `json:"result"`
	CreateTime        int64  `json:"createTime"`
	FinishTime        int64  `json:"finishTime"`
}

type personalTodoPayload struct {
	Body personalTodoBody `json:"body"`
}

type personalTodoBody struct {
	TaskID          string   `json:"taskId"`
	Subject         string   `json:"subject"`
	CreatorID       string   `json:"creatorId"`
	ExecutorIDs     []string `json:"executorIds"`
	ParticipantIDs  []string `json:"participantIds"`
	Priority        int64    `json:"priority"`
	StatusStage     int64    `json:"statusStage"`
	OldStatusStage  int64    `json:"oldStatusStage"`
	PlanStartDate   *int64   `json:"planStartDate"`
	PlanFinishDate  *int64   `json:"planFinishDate"`
	StartDate       *int64   `json:"startDate"`
	FinishDate      *int64   `json:"finishDate"`
	Description     string   `json:"description"`
	Source          string   `json:"source"`
	SourceID        string   `json:"sourceId"`
	BizTag          string   `json:"bizTag"`
	ParentID        *string  `json:"parentId"`
	IsMultiExecutor bool     `json:"isMultiExecutor"`
	SceneType       string   `json:"sceneType"`
	CreateTime      int64    `json:"createTime"`
	UpdateTime      int64    `json:"updateTime"`
	DeleteTime      int64    `json:"deleteTime"`
}

func (b *personalReactionBody) UnmarshalJSON(data []byte) error {
	// encoding/json otherwise falls back to case-insensitive field matching.
	// Read this protocol field from a map so only operOpenDingtalkId is accepted.
	type bodyAlias personalReactionBody
	var decoded bodyAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["operOpenDingtalkId"]; ok {
		if err := json.Unmarshal(raw, &decoded.OperatorOpenDingTalkID); err != nil {
			return fmt.Errorf("decode operOpenDingtalkId: %w", err)
		}
	}

	*b = personalReactionBody(decoded)
	return nil
}

func (b *personalGroupMemberBody) UnmarshalJSON(data []byte) error {
	// Keep the protocol spelling strict: encoding/json would otherwise accept
	// operOpenDingTalkId through case-insensitive fallback matching.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	type bodyAlias personalGroupMemberBody
	var decoded bodyAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if raw, ok := fields["operOpenDingtalkId"]; ok {
		if err := json.Unmarshal(raw, &decoded.OperatorOpenDingTalkID); err != nil {
			return fmt.Errorf("decode operOpenDingtalkId: %w", err)
		}
	}

	*b = personalGroupMemberBody(decoded)
	return nil
}

// ProjectOutput converts the transport envelope into the stable personal
// event output. On malformed Data it returns the original envelope together
// with an error; the formatter logs the warning and still emits that envelope.
func ProjectOutput(ev transport.Event) (any, error) {
	data, err := decodePersonalEventData(ev.Data)
	if err != nil {
		return ev, fmt.Errorf("decode personal event data: %w", err)
	}

	eventType := firstNonEmptyOutput(ev.EventType, data.EventKey)
	eventID := firstNonEmptyOutput(data.EventID, ev.EventID)
	timestamp := data.OccurredAtMS
	if timestamp == 0 {
		timestamp = ev.EventBornTime
	}
	subscribeID := firstNonEmptyOutput(ev.SubscribeID, data.SubID)

	if isMessageReceiveEvent(eventType) {
		var payload personalMessagePayload
		if err := decodeRequiredPayload(data.Payload, &payload); err != nil {
			return ev, fmt.Errorf("decode personal message payload: %w", err)
		}
		var quotedMessage *MessageEventContext
		if payload.Body.QuotedMessage != nil {
			projected := projectMessageEventContext(*payload.Body.QuotedMessage)
			quotedMessage = &projected
		}
		var forwardMessages []MessageEventContext
		if len(payload.Body.ForwardMessages) > 0 {
			forwardMessages = make([]MessageEventContext, 0, len(payload.Body.ForwardMessages))
			for _, message := range payload.Body.ForwardMessages {
				forwardMessages = append(forwardMessages, projectMessageEventContext(message))
			}
		}
		return MessageEventOutput{
			Type:                 eventType,
			EventID:              eventID,
			Timestamp:            timestamp,
			SubscribeID:          subscribeID,
			MessageID:            payload.Body.OpenMessageID,
			ConversationID:       payload.Body.OpenConversationID,
			Sender:               payload.Body.Sender,
			SenderOpenDingTalkID: payload.Body.SenderOpenDingTalkID,
			Content:              payload.Body.Content,
			CreateTime:           payload.Body.CreateTime,
			EventTime:            payload.EventTime,
			QuotedMessage:        quotedMessage,
			ForwardMessages:      forwardMessages,
		}, nil
	}

	base := baseEventOutput{
		Type:        eventType,
		EventID:     eventID,
		Timestamp:   timestamp,
		SubscribeID: subscribeID,
	}
	switch {
	case isReadEvent(eventType):
		return projectReadEvent(ev, base, data.Payload)
	case isRecallEvent(eventType):
		return projectRecallEvent(ev, base, data.Payload)
	case isReactionEvent(eventType):
		return projectReactionEvent(ev, base, data.Payload)
	case isGroupMemberEvent(eventType):
		return projectGroupMemberEvent(ev, base, data.Payload)
	case isGroupLifecycleEvent(eventType):
		payload, err := decodeConservativePayload(data.Payload)
		if err != nil {
			return ev, fmt.Errorf("decode personal group lifecycle payload: %w", err)
		}
		return GroupLifecycleEventOutput{
			Type:        base.Type,
			EventID:     base.EventID,
			Timestamp:   base.Timestamp,
			SubscribeID: base.SubscribeID,
			Payload:     payload,
		}, nil
	case isOAEvent(eventType):
		return projectOAApprovalEvent(ev, base, data.Payload)
	case isTodoEvent(eventType):
		return projectTodoEvent(ev, base, data.Payload)
	default:
		return ev, fmt.Errorf("unsupported personal event type %q", eventType)
	}
}

func projectTodoEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalTodoPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal Todo payload: %w", err)
	}
	if strings.TrimSpace(payload.Body.TaskID) == "" {
		return ev, fmt.Errorf("decode personal Todo payload: taskId is required")
	}

	switch base.Type {
	case EventTodoTaskCreated:
		return TodoTaskCreatedOutput{
			Type:            base.Type,
			EventID:         base.EventID,
			Timestamp:       base.Timestamp,
			SubscribeID:     base.SubscribeID,
			TaskID:          payload.Body.TaskID,
			Subject:         payload.Body.Subject,
			CreatorID:       payload.Body.CreatorID,
			ExecutorIDs:     payload.Body.ExecutorIDs,
			ParticipantIDs:  payload.Body.ParticipantIDs,
			Priority:        payload.Body.Priority,
			StatusStage:     payload.Body.StatusStage,
			PlanStartDate:   payload.Body.PlanStartDate,
			PlanFinishDate:  payload.Body.PlanFinishDate,
			StartDate:       payload.Body.StartDate,
			FinishDate:      payload.Body.FinishDate,
			Description:     payload.Body.Description,
			Source:          payload.Body.Source,
			SourceID:        payload.Body.SourceID,
			BizTag:          payload.Body.BizTag,
			ParentID:        payload.Body.ParentID,
			IsMultiExecutor: payload.Body.IsMultiExecutor,
			SceneType:       payload.Body.SceneType,
			CreateTime:      payload.Body.CreateTime,
		}, nil
	case EventTodoTaskUpdated:
		return TodoTaskUpdatedOutput{
			Type:            base.Type,
			EventID:         base.EventID,
			Timestamp:       base.Timestamp,
			SubscribeID:     base.SubscribeID,
			TaskID:          payload.Body.TaskID,
			Subject:         payload.Body.Subject,
			CreatorID:       payload.Body.CreatorID,
			ExecutorIDs:     payload.Body.ExecutorIDs,
			ParticipantIDs:  payload.Body.ParticipantIDs,
			Priority:        payload.Body.Priority,
			StatusStage:     payload.Body.StatusStage,
			OldStatusStage:  payload.Body.OldStatusStage,
			PlanStartDate:   payload.Body.PlanStartDate,
			PlanFinishDate:  payload.Body.PlanFinishDate,
			StartDate:       payload.Body.StartDate,
			FinishDate:      payload.Body.FinishDate,
			Description:     payload.Body.Description,
			Source:          payload.Body.Source,
			SourceID:        payload.Body.SourceID,
			BizTag:          payload.Body.BizTag,
			ParentID:        payload.Body.ParentID,
			IsMultiExecutor: payload.Body.IsMultiExecutor,
			SceneType:       payload.Body.SceneType,
			CreateTime:      payload.Body.CreateTime,
			UpdateTime:      payload.Body.UpdateTime,
		}, nil
	case EventTodoTaskDeleted:
		return TodoTaskDeletedOutput{
			Type:        base.Type,
			EventID:     base.EventID,
			Timestamp:   base.Timestamp,
			SubscribeID: base.SubscribeID,
			TaskID:      payload.Body.TaskID,
			Subject:     payload.Body.Subject,
			CreatorID:   payload.Body.CreatorID,
			CreateTime:  payload.Body.CreateTime,
			DeleteTime:  payload.Body.DeleteTime,
		}, nil
	default:
		return ev, fmt.Errorf("unsupported personal Todo event type %q", base.Type)
	}
}

func projectMessageEventContext(message personalMessageContext) MessageEventContext {
	return MessageEventContext{
		MessageID:            message.OpenMessageID,
		ConversationID:       message.OpenConversationID,
		Sender:               message.Sender,
		SenderOpenDingTalkID: message.SenderOpenDingTalkID,
		Content:              message.Content,
		CreateTime:           message.CreateTime,
	}
}

func decodeConservativePayload(raw json.RawMessage) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, fmt.Errorf("payload is missing")
	}
	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("payload is empty")
	}
	for key := range payload {
		switch strings.ToLower(key) {
		case "uid", "corpid", "clientid", "filtersubid", "bizid", "orgid", "sourceid":
			delete(payload, key)
		}
	}
	return payload, nil
}

func projectReadEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalReadPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal read payload: %w", err)
	}
	return ReadEventOutput{
		Type:                 base.Type,
		EventID:              base.EventID,
		Timestamp:            base.Timestamp,
		SubscribeID:          base.SubscribeID,
		MessageID:            payload.Body.MessageID,
		ConversationID:       payload.Body.ConversationID,
		Reader:               payload.Body.Reader,
		ReaderOpenDingTalkID: payload.Body.ReaderOpenDingTalkID,
		Sender:               payload.Body.Sender,
		SenderOpenDingTalkID: payload.Body.SenderOpenDingTalkID,
		ReadTime:             payload.Body.ReadTime,
		EventTime:            payload.EventTime,
	}, nil
}

func projectRecallEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalRecallPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal recall payload: %w", err)
	}
	return RecallEventOutput{
		Type:                   base.Type,
		EventID:                base.EventID,
		Timestamp:              base.Timestamp,
		SubscribeID:            base.SubscribeID,
		MessageID:              payload.Body.MessageID,
		ConversationID:         payload.Body.ConversationID,
		Recaller:               payload.Body.Recaller,
		RecallerOpenDingTalkID: payload.Body.RecallerOpenDingTalkID,
		Sender:                 payload.Body.Sender,
		SenderOpenDingTalkID:   payload.Body.SenderOpenDingTalkID,
		RecallTime:             payload.Body.RecallTime,
		EventTime:              payload.EventTime,
	}, nil
}

func projectReactionEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalReactionPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal reaction payload: %w", err)
	}
	return ReactionEventOutput{
		Type:                   base.Type,
		EventID:                base.EventID,
		Timestamp:              base.Timestamp,
		SubscribeID:            base.SubscribeID,
		MessageID:              payload.Body.MessageID,
		ConversationID:         payload.Body.ConversationID,
		Operator:               payload.Body.Operator,
		OperatorOpenDingTalkID: payload.Body.OperatorOpenDingTalkID,
		ReactionName:           payload.Body.ReactionName,
		ReactionText:           payload.Body.ReactionText,
		OperationType:          payload.Body.OperationType,
		OperationTime:          payload.Body.OperationTime,
		Sender:                 payload.Body.Sender,
		SenderOpenDingTalkID:   payload.Body.SenderOpenDingTalkID,
		EventTime:              payload.EventTime,
	}, nil
}

func projectGroupMemberEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalGroupMemberPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal group member payload: %w", err)
	}
	if strings.TrimSpace(payload.Body.ConversationID) == "" {
		return ev, fmt.Errorf("decode personal group member payload: openConversationId is required")
	}
	if len(payload.Body.Members) == 0 {
		return ev, fmt.Errorf("decode personal group member payload: members is required")
	}

	members := make([]GroupMemberEventMember, 0, len(payload.Body.Members))
	for _, member := range payload.Body.Members {
		members = append(members, GroupMemberEventMember{
			Nick:           member.Nick,
			OpenDingTalkID: member.OpenDingTalkID,
		})
	}
	return GroupMemberEventOutput{
		Type:                   base.Type,
		EventID:                base.EventID,
		Timestamp:              base.Timestamp,
		SubscribeID:            base.SubscribeID,
		ConversationID:         payload.Body.ConversationID,
		Operator:               payload.Body.Operator,
		OperatorOpenDingTalkID: payload.Body.OperatorOpenDingTalkID,
		Members:                members,
		EventTime:              payload.EventTime,
	}, nil
}

func projectOAApprovalEvent(ev transport.Event, base baseEventOutput, raw json.RawMessage) (any, error) {
	var payload personalOAApprovalPayload
	if err := decodeRequiredPayload(raw, &payload); err != nil {
		return ev, fmt.Errorf("decode personal OA payload: %w", err)
	}
	if strings.TrimSpace(payload.Body.ProcessInstanceID) == "" {
		return ev, fmt.Errorf("decode personal OA payload: processInstanceId is required")
	}
	if isOAApprovalTaskEvent(base.Type) && strings.TrimSpace(payload.Body.TaskID) == "" {
		return ev, fmt.Errorf("decode personal OA payload: taskId is required for %s", base.Type)
	}

	switch base.Type {
	case EventOAApprovalTaskCreated:
		return OAApprovalTaskCreatedOutput{
			Type:              base.Type,
			EventID:           base.EventID,
			Timestamp:         base.Timestamp,
			SubscribeID:       base.SubscribeID,
			ProcessInstanceID: payload.Body.ProcessInstanceID,
			ProcessCode:       payload.Body.ProcessCode,
			TaskID:            payload.Body.TaskID,
			Title:             payload.Body.Title,
			Status:            payload.Body.Status,
			CreateTime:        payload.Body.CreateTime,
			EventTime:         payload.EventTime,
		}, nil
	case EventOAApprovalTaskFinished:
		return OAApprovalTaskFinishedOutput{
			Type:              base.Type,
			EventID:           base.EventID,
			Timestamp:         base.Timestamp,
			SubscribeID:       base.SubscribeID,
			ProcessInstanceID: payload.Body.ProcessInstanceID,
			ProcessCode:       payload.Body.ProcessCode,
			TaskID:            payload.Body.TaskID,
			Title:             payload.Body.Title,
			Status:            payload.Body.Status,
			Result:            payload.Body.Result,
			CreateTime:        payload.Body.CreateTime,
			FinishTime:        payload.Body.FinishTime,
			EventTime:         payload.EventTime,
		}, nil
	case EventOAApprovalTaskRedirected:
		return OAApprovalTaskRedirectedOutput{
			Type:              base.Type,
			EventID:           base.EventID,
			Timestamp:         base.Timestamp,
			SubscribeID:       base.SubscribeID,
			ProcessInstanceID: payload.Body.ProcessInstanceID,
			ProcessCode:       payload.Body.ProcessCode,
			TaskID:            payload.Body.TaskID,
			Title:             payload.Body.Title,
			Status:            payload.Body.Status,
			Result:            payload.Body.Result,
			CreateTime:        payload.Body.CreateTime,
			FinishTime:        payload.Body.FinishTime,
			EventTime:         payload.EventTime,
		}, nil
	case EventOAApprovalInstanceStarted:
		return OAApprovalInstanceStartedOutput{
			Type:              base.Type,
			EventID:           base.EventID,
			Timestamp:         base.Timestamp,
			SubscribeID:       base.SubscribeID,
			ProcessInstanceID: payload.Body.ProcessInstanceID,
			ProcessCode:       payload.Body.ProcessCode,
			Title:             payload.Body.Title,
			Status:            payload.Body.Status,
			CreateTime:        payload.Body.CreateTime,
			EventTime:         payload.EventTime,
		}, nil
	case EventOAApprovalInstanceCC:
		return OAApprovalInstanceCCOutput{
			Type:              base.Type,
			EventID:           base.EventID,
			Timestamp:         base.Timestamp,
			SubscribeID:       base.SubscribeID,
			ProcessInstanceID: payload.Body.ProcessInstanceID,
			ProcessCode:       payload.Body.ProcessCode,
			Title:             payload.Body.Title,
			Status:            payload.Body.Status,
			CreateTime:        payload.Body.CreateTime,
			EventTime:         payload.EventTime,
		}, nil
	case EventOAApprovalInstanceTerminated:
		return OAApprovalInstanceTerminatedOutput{
			Type:              base.Type,
			EventID:           base.EventID,
			Timestamp:         base.Timestamp,
			SubscribeID:       base.SubscribeID,
			ProcessInstanceID: payload.Body.ProcessInstanceID,
			ProcessCode:       payload.Body.ProcessCode,
			Title:             payload.Body.Title,
			Status:            payload.Body.Status,
			CreateTime:        payload.Body.CreateTime,
			FinishTime:        payload.Body.FinishTime,
			EventTime:         payload.EventTime,
		}, nil
	case EventOAApprovalInstanceFinished:
		return OAApprovalInstanceFinishedOutput{
			Type:              base.Type,
			EventID:           base.EventID,
			Timestamp:         base.Timestamp,
			SubscribeID:       base.SubscribeID,
			ProcessInstanceID: payload.Body.ProcessInstanceID,
			ProcessCode:       payload.Body.ProcessCode,
			Title:             payload.Body.Title,
			Status:            payload.Body.Status,
			Result:            payload.Body.Result,
			CreateTime:        payload.Body.CreateTime,
			FinishTime:        payload.Body.FinishTime,
			EventTime:         payload.EventTime,
		}, nil
	default:
		return ev, fmt.Errorf("unsupported personal OA event type %q", base.Type)
	}
}

func decodeRequiredPayload(raw json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("payload is missing")
	}

	var payloadObject map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &payloadObject); err != nil {
		return err
	}
	if len(payloadObject) == 0 {
		return fmt.Errorf("payload is empty")
	}

	body, ok := payloadObject["body"]
	if !ok {
		return fmt.Errorf("payload body is missing")
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || bytes.Equal(body, []byte("null")) {
		return fmt.Errorf("payload body is missing")
	}
	var bodyObject map[string]json.RawMessage
	if err := json.Unmarshal(body, &bodyObject); err != nil {
		return fmt.Errorf("decode payload body: %w", err)
	}
	if len(bodyObject) == 0 {
		return fmt.Errorf("payload body is empty")
	}

	return json.Unmarshal(trimmed, target)
}

func decodePersonalEventData(raw string) (personalEventData, error) {
	encoded := []byte(strings.TrimSpace(raw))
	if len(encoded) == 0 {
		return personalEventData{}, fmt.Errorf("empty data")
	}

	// Some gateways wrap the JSON object in one or more JSON strings. Peel
	// those wrappers without changing the raw transport envelope.
	for depth := 0; depth < 2; depth++ {
		var quoted string
		if err := json.Unmarshal(encoded, &quoted); err != nil {
			break
		}
		encoded = []byte(strings.TrimSpace(quoted))
	}

	var data personalEventData
	if err := json.Unmarshal(encoded, &data); err != nil {
		return personalEventData{}, err
	}
	if data.EventKey == "" && data.EventID == "" && data.OccurredAtMS == 0 && data.SubID == "" && len(data.Payload) == 0 {
		return personalEventData{}, fmt.Errorf("data is not a personal event object")
	}
	return data, nil
}

func outputSchema(eventKey string) map[string]any {
	outputType := outputTypeForEvent(eventKey)
	schema := schemaForStruct(outputType)
	properties := schema["properties"].(map[string]any)
	if property, ok := properties["type"].(map[string]any); ok {
		property["enum"] = []string{eventKey}
	}
	return schema
}

func schemaForStruct(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	properties := make(map[string]any, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		property := schemaForType(field.Type)
		if description := field.Tag.Get("description"); description != "" {
			property["description"] = description
		}
		if format := field.Tag.Get("format"); format != "" {
			property["format"] = format
		}
		if field.Tag.Get("additional_properties") == "true" {
			property["additionalProperties"] = true
		}
		properties[name] = property
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

func schemaForType(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		return schemaForStruct(t)
	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": schemaForType(t.Elem()),
		}
	default:
		return map[string]any{"type": schemaType(t)}
	}
}

func transportEnvelopeSchema(eventKey string) map[string]any {
	eventType := reflect.TypeOf(transport.Event{})
	properties := make(map[string]any, eventType.NumField())
	for i := 0; i < eventType.NumField(); i++ {
		field := eventType.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		property := map[string]any{"type": schemaType(field.Type)}
		switch name {
		case "type":
			property["description"] = "transport frame 类型"
			property["enum"] = []string{string(transport.FrameTypeEvent)}
		case "event_type":
			property["description"] = "事件类型"
			property["enum"] = []string{eventKey}
		case "data":
			property["description"] = "服务端业务 payload JSON 字符串"
			property["content_media_type"] = "application/json"
		case "headers":
			property["description"] = "Stream transport headers"
			property["additionalProperties"] = map[string]any{"type": "string"}
		case "event_id":
			property["description"] = "transport 事件 ID"
		case "subscribe_id":
			property["description"] = "个人事件订阅 ID"
		}
		properties[name] = property
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

func outputTypeForEvent(eventKey string) reflect.Type {
	switch {
	case isMessageReceiveEvent(eventKey):
		return reflect.TypeOf(MessageEventOutput{})
	case isReadEvent(eventKey):
		return reflect.TypeOf(ReadEventOutput{})
	case isRecallEvent(eventKey):
		return reflect.TypeOf(RecallEventOutput{})
	case isReactionEvent(eventKey):
		return reflect.TypeOf(ReactionEventOutput{})
	case isGroupMemberEvent(eventKey):
		return reflect.TypeOf(GroupMemberEventOutput{})
	case isGroupLifecycleEvent(eventKey):
		return reflect.TypeOf(GroupLifecycleEventOutput{})
	case eventKey == EventOAApprovalTaskCreated:
		return reflect.TypeOf(OAApprovalTaskCreatedOutput{})
	case eventKey == EventOAApprovalTaskFinished:
		return reflect.TypeOf(OAApprovalTaskFinishedOutput{})
	case eventKey == EventOAApprovalTaskRedirected:
		return reflect.TypeOf(OAApprovalTaskRedirectedOutput{})
	case eventKey == EventOAApprovalInstanceStarted:
		return reflect.TypeOf(OAApprovalInstanceStartedOutput{})
	case eventKey == EventOAApprovalInstanceCC:
		return reflect.TypeOf(OAApprovalInstanceCCOutput{})
	case eventKey == EventOAApprovalInstanceTerminated:
		return reflect.TypeOf(OAApprovalInstanceTerminatedOutput{})
	case eventKey == EventOAApprovalInstanceFinished:
		return reflect.TypeOf(OAApprovalInstanceFinishedOutput{})
	case eventKey == EventTodoTaskCreated:
		return reflect.TypeOf(TodoTaskCreatedOutput{})
	case eventKey == EventTodoTaskUpdated:
		return reflect.TypeOf(TodoTaskUpdatedOutput{})
	case eventKey == EventTodoTaskDeleted:
		return reflect.TypeOf(TodoTaskDeletedOutput{})
	default:
		return reflect.TypeOf(baseEventOutput{})
	}
}

func isReadEvent(eventKey string) bool {
	return eventKey == EventReadO2O || eventKey == EventReadGroup
}

func isRecallEvent(eventKey string) bool {
	return eventKey == EventRecallO2O || eventKey == EventRecallGroup
}

func isReactionEvent(eventKey string) bool {
	return eventKey == EventReactionO2O || eventKey == EventReactionGroup
}

func isGroupMemberEvent(eventKey string) bool {
	return eventKey == EventGroupMemberAdded || eventKey == EventGroupMemberExited
}

func isGroupLifecycleEvent(eventKey string) bool {
	return eventKey == EventGroupUpdated ||
		eventKey == EventGroupDisbanded
}

func isOAEvent(eventKey string) bool {
	return eventKey == EventOAApprovalTaskCreated ||
		eventKey == EventOAApprovalTaskFinished ||
		eventKey == EventOAApprovalTaskRedirected ||
		eventKey == EventOAApprovalInstanceStarted ||
		eventKey == EventOAApprovalInstanceCC ||
		eventKey == EventOAApprovalInstanceTerminated ||
		eventKey == EventOAApprovalInstanceFinished
}

func isTodoEvent(eventKey string) bool {
	return eventKey == EventTodoTaskCreated ||
		eventKey == EventTodoTaskUpdated ||
		eventKey == EventTodoTaskDeleted
}

func isOAApprovalTaskEvent(eventKey string) bool {
	return eventKey == EventOAApprovalTaskCreated ||
		eventKey == EventOAApprovalTaskFinished ||
		eventKey == EventOAApprovalTaskRedirected
}

func schemaType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Bool:
		return "boolean"
	default:
		return "object"
	}
}

func firstNonEmptyOutput(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
