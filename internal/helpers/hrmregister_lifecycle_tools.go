// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

// Hrmregister lifecycle tool names are kept here as the reviewed gateway
// contract used by the hand-written Shortcut adapters. The generated
// hrmregister.go remains owned by cmd/sync-oss and must not be edited by hand.
const (
	HrmregisterToolGetRosterValueDBG            = "get_roster_value_dbg"
	HrmregisterToolGetRosterFieldsDBG           = "get_roster_fields_dbg"
	HrmregisterToolUpdateEmployeeRoster         = "update_employee_roster"
	HrmregisterToolListContractLegalEntities    = "list_contract_legal_entities"
	HrmregisterToolListEmployeeDataSources      = "list_employee_data_sources"
	HrmregisterToolQueryEmployeeSalaryData      = "query_employee_salary_data"
	HrmregisterToolGetTerminationByID           = "get_termination_by_id"
	HrmregisterToolGetEmployeeTerminationReason = "get_employee_termination_reason"
	HrmregisterToolQueryTerminationEmployees    = "query_termination_employees"
	HrmregisterToolQueryPerformanceRecords      = "query_performance_records"
	HrmregisterToolQueryContractLedgers         = "query_contract_ledgers"
	HrmregisterToolCountEmployeeChangeRecords   = "count_employee_change_records"
	HrmregisterToolQueryEmployeeChangeEvents    = "query_employee_change_events"
	HrmregisterToolQueryPendingTransfers        = "query_pending_transfers"
	HrmregisterToolGetTransferForm              = "get_transfer_form"
	HrmregisterToolQueryCurrentPosition         = "query_current_position"
	HrmregisterToolCancelTransferApproval       = "cancel_transfer_approval"
	HrmregisterToolCreateTransferApproval       = "create_transfer_approval"
	HrmregisterToolRevokeTerminationProcess     = "revoke_termination_process"
	HrmregisterToolUpdateTerminationInformation = "update_termination_information"
	HrmregisterToolQueryPendingTerminations     = "query_pending_terminations"
	HrmregisterToolQueryLifecycleRecipients     = "query_hrm_lifecycle_recipients"
	HrmregisterToolQueryPendingRegularizations  = "query_pending_regularizations"
	HrmregisterToolGetRegularizationForm        = "get_regularization_form"
	HrmregisterToolGetEmployeeMaterialFiles     = "get_employee_material_files"
	HrmregisterToolQueryPreEntryEmployees       = "query_pre_entry_employees"
	HrmregisterToolConfirmTermination           = "confirm_termination"
	HrmregisterToolGetConfirmTerminationForm    = "get_confirm_termination_form"
	HrmregisterToolQueryPreEntryByNameMobile    = "query_pre_entry_by_name_mobile"
	HrmregisterToolConfirmEntry                 = "confirm_entry"
	HrmregisterToolGetConfirmEntryForm          = "get_confirm_entry_form"
	HrmregisterToolInvitePerfectInfo            = "invite_perfect_info"
	HrmregisterToolAddPreEntryEmployee          = "add_pre_entry_employee"
)
