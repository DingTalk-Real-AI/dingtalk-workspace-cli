// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

// MessageIdentityCapability is the reviewed Runtime capability descriptor for
// +messages-send. Validation and Skill drift checks consume this same typed
// source; it describes only behavior that the current lower transports expose.
type MessageIdentityCapability struct {
	Identity        string
	Targets         []string
	ContentTypes    []string
	NaturalTargets  []string
	MentionTargets  []string
	IdempotencyKeys bool
	BatchLedger     bool
}

var messageIdentityCapabilities = []MessageIdentityCapability{
	{
		Identity:        "user",
		Targets:         []string{"group", "direct-user", "direct-open-dingtalk-id"},
		ContentTypes:    []string{"text", "markdown", "image-media-id", "file", "audio-as-file", "video-as-file"},
		NaturalTargets:  []string{"chat-query", "user-query"},
		MentionTargets:  []string{"open-dingtalk-id", "all"},
		IdempotencyKeys: true,
		BatchLedger:     false,
	},
	{
		Identity:        "bot",
		Targets:         []string{"group", "groups", "direct-users", "direct-open-dingtalk-ids"},
		ContentTypes:    []string{"text", "markdown"},
		NaturalTargets:  []string{},
		MentionTargets:  []string{"user-id", "open-dingtalk-id", "all"},
		IdempotencyKeys: false,
		BatchLedger:     true,
	},
	{
		Identity:        "webhook",
		Targets:         []string{"token-owned-group"},
		ContentTypes:    []string{"text", "markdown"},
		NaturalTargets:  []string{},
		MentionTargets:  []string{"user-id", "mobile", "all"},
		IdempotencyKeys: false,
		BatchLedger:     false,
	},
}

// MessageIdentityCapabilities returns defensive copies of the public matrix.
func MessageIdentityCapabilities() []MessageIdentityCapability {
	out := make([]MessageIdentityCapability, len(messageIdentityCapabilities))
	for i, capability := range messageIdentityCapabilities {
		out[i] = capability
		out[i].Targets = append([]string(nil), capability.Targets...)
		out[i].ContentTypes = append([]string(nil), capability.ContentTypes...)
		out[i].NaturalTargets = append([]string(nil), capability.NaturalTargets...)
		out[i].MentionTargets = append([]string(nil), capability.MentionTargets...)
	}
	return out
}

func messageIdentitySupportsContent(identity, contentType string) bool {
	if contentType == "audio" || contentType == "video" {
		contentType += "-as-file"
	} else if contentType == "image" {
		contentType = "image-media-id"
	}
	for _, capability := range messageIdentityCapabilities {
		if capability.Identity != identity {
			continue
		}
		for _, supported := range capability.ContentTypes {
			if supported == contentType {
				return true
			}
		}
		return false
	}
	return false
}
