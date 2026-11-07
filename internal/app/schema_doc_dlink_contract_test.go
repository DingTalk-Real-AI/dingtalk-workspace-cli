package app

import (
	"strings"
	"testing"
)

func TestDocInfoDLinkRoutingFinalSchema(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), "doc.get_document_info")
	tool := payload.Tools["doc.get_document_info"]
	if tool == nil {
		t.Fatal("missing doc.get_document_info")
	}

	selection := strings.Join(append(
		[]string{
			schemaContractString(tool["description"]),
			schemaContractString(tool["agent_summary"]),
		},
		schemaContractStringSlice(tool["use_when"])...,
	), "\n")
	for _, want := range []string{
		"extension=dlink",
		"linkSourceInfo.nodeId",
		"内容操作",
		"移动/重命名/删除",
		"顶层 nodeId",
	} {
		if !strings.Contains(selection, want) {
			t.Fatalf("doc info final Schema selection does not contain %q: %s", want, selection)
		}
	}
}

func TestDriveInfoDLinkRoutingFinalSchema(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), "drive.get_file_info")
	tool := payload.Tools["drive.get_file_info"]
	if tool == nil {
		t.Fatal("missing drive.get_file_info")
	}

	selection := strings.Join(append(
		[]string{
			schemaContractString(tool["description"]),
			schemaContractString(tool["agent_summary"]),
		},
		schemaContractStringSlice(tool["use_when"])...,
	), "\n")
	for _, want := range []string{
		"extension=dlink",
		"result.fileId",
		"dws doc info",
		"linkSourceInfo.nodeId",
		"快捷方式入口 ID",
		"移动、重命名或删除",
	} {
		if !strings.Contains(selection, want) {
			t.Fatalf("drive info final Schema selection does not contain %q: %s", want, selection)
		}
	}
	for _, forbidden := range []string{"<快捷方式nodeId>", "顶层 nodeId"} {
		if strings.Contains(selection, forbidden) {
			t.Fatalf("drive info final Schema selection contains stale field guidance %q: %s", forbidden, selection)
		}
	}
}
