package safechat

import (
	"errors"
	"fmt"
)

// SDK-level errors (Go side)
var (
	// ErrConfigDataPathEmpty indicates DataPath is not set in Config.
	ErrConfigDataPathEmpty = errors.New("safechat: config.DataPath is required")

	// ErrConfigUserIDEmpty indicates UserID is not set in Config.
	ErrConfigUserIDEmpty = errors.New("safechat: config.UserID is required")

	// ErrNotInitialized indicates the client has not been initialized.
	ErrNotInitialized = errors.New("safechat: client not initialized")

	// ErrMaxRetryExceeded indicates max retry attempts for key acquisition exceeded.
	ErrMaxRetryExceeded = errors.New("safechat: max retry exceeded, key not available")

	// ErrKeyRestricted indicates the enterprise key is restricted (managed/blocked).
	ErrKeyRestricted = errors.New("safechat: enterprise key is restricted by admin")

	// ErrKeyRequestFailed indicates the HTTP key request failed.
	ErrKeyRequestFailed = errors.New("safechat: key request HTTP call failed")

	// ErrAlreadyInitialized indicates init was called more than once.
	ErrAlreadyInitialized = errors.New("safechat: client already initialized")
)

// CError represents an error code returned from the C library layer.
type CError struct {
	Code    int
	Message string
}

func (e *CError) Error() string {
	return fmt.Sprintf("safechat: C library error %d: %s", e.Code, e.Message)
}

// mapCError maps a C library return code to a Go error.
// Returns nil if code == 0 (FUNCTION_OK).
func mapCError(code int) error {
	if code == 0 {
		return nil
	}

	msg, ok := cErrorMessages[code]
	if !ok {
		msg = "unknown error"
	}

	return &CError{Code: code, Message: msg}
}

// C library error code constants - mapped from native.h
const (
	// Special status codes
	cFunctionOK         = 0
	cSendRequestParamOK = -15002 // Key not found, request sent via goProxy

	// Init errors (-20000 ~ -20016)
	cInitParamPathNull    = -20000
	cInitParamMyIDNull    = -20001
	cInitPathUTF8Null     = -20002
	cInitPathNotAvailable = -20003
	cInitLogInitError     = -20004
	cInitEncryptInitError = -20005
	cInitPCNativeError    = -20006
	cInitReadLocalKey     = -20013
	cInitReadKey          = -20014

	// EncryptData errors (-20100 ~ -20114)
	cEncryptDataCorpIDNull  = -20100
	cEncryptDataMsgNull     = -20101
	cEncryptDataBufNull     = -20102
	cEncryptDataRetLenNull  = -20103
	cEncryptDataProxyNull   = -20104
	cEncryptDataLenError    = -20105
	cEncryptDataKeyNegtive  = -20111
	cEncryptDataTmpKeyNull  = -20109
	cEncryptDataBuildReqErr = -20110

	// DecryptData errors (-20200 ~ -20215)
	cDecryptDataCorpIDNull  = -20200
	cDecryptDataMsgNull     = -20201
	cDecryptDataBufNull     = -20202
	cDecryptDataRetLenNull  = -20203
	cDecryptDataProxyNull   = -20204
	cDecryptDataLenError    = -20205
	cDecryptDataFormatError = -20206
	cDecryptDataDecryptErr  = -20211
	cDecryptDataKeyNegtive  = -20214
	cDecryptDataTmpKeyNull  = -20212
	cDecryptDataBuildReqErr = -20213

	// EncryptFile errors (-20300 ~ -20339)
	cEncryptFileCorpIDNull  = -20300
	cEncryptFilePathNull    = -20301
	cEncryptFileTmpKeyNull  = -20315
	cEncryptFileBuildReqErr = -20316

	// EncryptBuffer errors (-20400 ~ -20416)
	cEncryptBufferCorpIDNull  = -20400
	cEncryptBufferTmpKeyNull  = -20414
	cEncryptBufferBuildReqErr = -20415

	// DecryptFile errors (-20500 ~ -20538)
	cDecryptFileCorpIDNull  = -20500
	cDecryptFilePathNull    = -20501
	cDecryptFileHeadError   = -20505
	cDecryptFileHashError   = -20517
	cDecryptFileTmpKeyNull  = -20518
	cDecryptFileBuildReqErr = -20519

	// DecryptBuffer errors (-20600 ~ -20622)
	cDecryptBufferCorpIDNull  = -20600
	cDecryptBufferHeadNull    = -20608
	cDecryptBufferHashError   = -20618
	cDecryptBufferTmpKeyNull  = -20619
	cDecryptBufferBuildReqErr = -20620

	// SetResponse errors (-20700 ~ -20730)
	cSetResponseCorpIDNull = -20700
	cSetResponseJSONNull   = -20701
	cSetResponseKeyNagtive = -20728
	cSetResponseSaveKeyErr = -20723

	// SetPushData errors (-20800 ~ -20814)
	cSetPushDataCorpIDNull = -20800
	cSetPushDataTypeUndef  = -20814

	// V3 signing errors (-31001 ~ -34002)
	cV3BuildReq3SM2Failed  = -31006
	cV3BuildReq3SignError  = -31009
	cV3ParseResp3Downgrade = -32012
)

// cErrorMessages maps C error codes to human-readable messages.
var cErrorMessages = map[int]string{
	// Init
	-20000: "init: path parameter is NULL",
	-20001: "init: my_id parameter is NULL",
	-20002: "init: path UTF8 conversion returned NULL",
	-20003: "init: path is not available/accessible",
	-20004: "init: log initialization failed",
	-20005: "init: encryption engine initialization failed",
	-20006: "init: pcnative initialization failed",
	-20007: "init: get URL full path error",
	-20013: "init: read local key failed",
	-20014: "init: read key failed",
	-20015: "init: my_id parameter is NULL",
	-20016: "init: logFunc parameter is NULL",

	// EncryptData
	-20100: "encryptData: corp_id is NULL or empty",
	-20101: "encryptData: message content is NULL",
	-20102: "encryptData: encrypt_buf is NULL",
	-20103: "encryptData: ret_len is NULL",
	-20104: "encryptData: proxy function is NULL",
	-20105: "encryptData: data length invalid (too small or too big)",
	-20106: "encryptData: encryptMsgHelper parameter error",
	-20107: "encryptData: SM4 encryption failed",
	-20108: "encryptData: base64 encoding failed",
	-20109: "encryptData: key request already in progress",
	-20110: "encryptData: build key request failed",
	-20111: "encryptData: enterprise key is restricted",
	-20113: "encryptData: malloc encode buffer failed",
	-20114: "encryptData: malloc encrypt buffer failed",

	// DecryptData
	-20200: "decryptData: corp_id is NULL or empty",
	-20201: "decryptData: message content is NULL",
	-20202: "decryptData: decrypt_buf is NULL",
	-20203: "decryptData: ret_len is NULL",
	-20204: "decryptData: proxy function is NULL",
	-20205: "decryptData: data length invalid",
	-20206: "decryptData: message content format error (missing ||separators)",
	-20207: "decryptData: decryptMsgHelper parameter error",
	-20208: "decryptData: base64 decode failed",
	-20209: "decryptData: decode length error",
	-20210: "decryptData: decrypt buffer malloc failed",
	-20211: "decryptData: SM4 decryption failed",
	-20212: "decryptData: key request already in progress",
	-20213: "decryptData: build key request failed",
	-20214: "decryptData: enterprise key is restricted",

	// EncryptFile
	-20300: "encryptFile: corp_id is NULL or empty",
	-20301: "encryptFile: source or dest file path is NULL",
	-20302: "encryptFile: id parameter is NULL",
	-20303: "encryptFile: seq_id parameter is NULL",
	-20304: "encryptFile: proxy function is NULL",
	-20305: "encryptFile: key_info is NULL",
	-20308: "encryptFile: get file size failed",
	-20309: "encryptFile: open source or dest file failed",
	-20313: "encryptFile: encrypt block failed",
	-20315: "encryptFile: key request already in progress",
	-20316: "encryptFile: build key request failed",
	-20333: "encryptFile: create thread failed",
	-20339: "encryptFile: test environment error",

	// EncryptBuffer
	-20400: "encryptBuffer: corp_id is NULL or empty",
	-20401: "encryptBuffer: file content is NULL",
	-20407: "encryptBuffer: proxy function is NULL",
	-20413: "encryptBuffer: encrypted block error",
	-20414: "encryptBuffer: key request already in progress",
	-20415: "encryptBuffer: build key request failed",

	// DecryptFile
	-20500: "decryptFile: corp_id is NULL or empty",
	-20501: "decryptFile: source or dest file path is NULL",
	-20505: "decryptFile: file header format error",
	-20509: "decryptFile: header magic error",
	-20511: "decryptFile: open file failed",
	-20515: "decryptFile: decrypt block failed",
	-20517: "decryptFile: file hash verification failed (warning)",
	-20518: "decryptFile: key request already in progress",
	-20519: "decryptFile: build key request failed",
	-20520: "decryptFile: file length error",

	// DecryptBuffer
	-20600: "decryptBuffer: corp_id is NULL or empty",
	-20608: "decryptBuffer: parseHeadInfo returned NULL",
	-20615: "decryptBuffer: header magic error",
	-20617: "decryptBuffer: decrypt block error",
	-20618: "decryptBuffer: file hash verification failed",
	-20619: "decryptBuffer: key request already in progress",
	-20620: "decryptBuffer: build key request failed",
	-20621: "decryptBuffer: file length error",

	// SetResponse
	-20700: "setResponse: corp_id is NULL or empty",
	-20701: "setResponse: json_str is NULL or empty",
	-20703: "setResponse: find tmp corp no tmp key found",
	-20704: "setResponse: JSON parse failed",
	-20716: "setResponse: server request error",
	-20722: "setResponse: localKeyPath is NULL",
	-20723: "setResponse: save key failed",
	-20728: "setResponse: enterprise key is restricted (nagtive)",

	// SetPushData
	-20800: "setPushData: corp_id is NULL",
	-20801: "setPushData: push_data is NULL",
	-20804: "setPushData: JSON parse failed",
	-20805: "setPushData: type field is NULL",
	-20806: "setPushData: find_create_tmp_key failed",
	-20814: "setPushData: unknown push type",

	// V3 Signing
	-31001: "v3: build_key_request3 arg is NULL",
	-31006: "v3: SM2 encrypt R2 failed",
	-31009: "v3: sign step one error",
	-32005: "v3: deserialize response failed",
	-32010: "v3: new SDK request old private error",
	-32012: "v3: public server downgrade",
	-32022: "v3: MAC of response mismatch",
	-32029: "v3: verify server sign failed",

	// Misc
	-34001: "v3: setResponse not found tmp sign",
	-34002: "v3: setResponse malloc for ret buf failed",
}
