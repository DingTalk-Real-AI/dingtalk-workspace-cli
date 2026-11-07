#ifndef SAFECHAT_H
#define SAFECHAT_H
#ifdef WIN32
#ifndef DLL_EXPORT
#define CREATEDLL_API __declspec(dllimport)
#else
#define CREATEDLL_API __declspec(dllexport)
#endif
#else
#define CREATEDLL_API __attribute__((visibility("default")))
#endif

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef unsigned char byte;

// using namespace std;
// #pragma comment(lib, "safechat.lib")

#define DTMsgRetOK 0 /*operate return success*/


#define WARNING_RET_CODE 0x10000000
enum { RET_CODE_SUCCESS = 0, RET_CODE_ERROR, RET_CODE_WARNING };

// 获取返回值的类型，RET_CODE_WARNING | RET_CODE_ERROR| RET_CODE_SUCCESS
// #define GET_RET_CODETYPE(x)  (x == 0) ? RET_CODE_SUCCESS : ((-x) & WARNING_RET_CODE == WARNING_RET_CODE ?
// RET_CODE_WARNING : RET_CODE_ERROR)

/*
打印日志回调函数
@param:ret_type 打印日志类型，RET_CODE_WARNING | RET_CODE_ERROR| RET_CODE_SUCCESS
@param:msg_detail 打印日志的详细信息，char * UTF-8格式
@return 备用，暂且返回NULL
*/
typedef void *(*log_func)(int ret_type, char *msg_detail);

/*
代理回调函数
@param: corpid 企业id
@param: uid 当前用户id
@param: domain 请求的域名
@param: url 请求url
@param: param 请求参数
@return: 返回执行结果
@param: seq_id 回调函数线程识别参数
*/
typedef int (*call_proxy_func)(char *corpid, char *uid, char *domain, char *url, char *param,
                               char *seq_id);

/*
设置密钥管控标记
@param: corpid 企业id
*/
typedef int (*block_crypto_func)(char *corpid);

/*
解除密钥管控标记
@param: corpid 企业id
*/
typedef int (*cancel_block_crypto_func)(char *corpid);
/*
初始化函数
@param: path 用户数据路径， 保存key等信息
@param: my_id 当前登W录用户的id
@return: 返回执行结果
*/
CREATEDLL_API int safechatInit(char *path, char *my_id);
/*
数据加密函数
@param: corp_id 企业id
@param: staffid 员工id
@param: data_buf 加密前数据
@param: data_len data_buf长度
@param: id 对方用户id， 如果群消息设置为NULL
@param: encrypt_buf 加密后的数据指针，需要外部函数释放
@param: ret_len 加密数据的返回长度
@param: proxy 代理请求回调函数
@return: 返回执行结果
@param: seq_id 回调函数线程识别参数
*/
CREATEDLL_API int encryptData(char *corp_id, char *staffid, unsigned char *data_buf, unsigned int data_len, char *id,
                              unsigned char **encrypt_buf, unsigned int *ret_len, char *seq_id, call_proxy_func proxy);
/*
数据解密函数
@param: corp_id 企业id
@param: staffid 员工id
@param: data_buf 解密前数据
@param: data_len data_buf长度
@param: id 对方用户id， 如果群消息设置为NULL
@param: decrypt_buf 解密后的数据指针，需要外部函数释放
@param: ret_len 解密数据的返回长度
@param: proxy 代理请求回调函数
@return: 返回执行结果
@param: seq_id 回调函数线程识别参数
*/
CREATEDLL_API int decryptData(char *corp_id, char *staffid, unsigned char *data_buf, unsigned int data_len, char *id,
                              unsigned char **decrypt_buf, unsigned int *ret_len, char *seq_id, call_proxy_func proxy);

CREATEDLL_API int encryptFile(char *corp_id, char *staffid, char *id, char *in_file_path, char *out_file_path,
                              char *seq_id, call_proxy_func proxy);
CREATEDLL_API int encryptBuffer(char *corp_id, char *staffid, byte *data_buf, uint32_t data_len, char *id,
                                byte **encrypt_buf, uint32_t *ret_len, char *seq_id, call_proxy_func proxy);
CREATEDLL_API int decryptFile(char *corp_id, char *staffid, char *id, char *in_file_path, char *out_file_path,
                              char *seq_id, call_proxy_func proxy);
CREATEDLL_API int decryptBuffer(char *corp_id, char *staffid, byte *data_buf, uint32_t data_len, char *id,
                                byte **decrypt_buf, uint32_t *ret_len, char *seq_id, call_proxy_func proxy);

/*
服务器响应处理
@param: corp_id 企业id
@param: json_str json格式的服务器返回值
@return: 返回执行结果
*/
CREATEDLL_API int setResponse(char *corp_id, char *json_str, block_crypto_func block_func1);

/*
消息推送处理
@param: corp_id 企业id
@param: staffid 员工id
@param: push_data 发送请求的数据
@param: proxy 代理请求回调函数
@return: 返回执行结果
@param: seq_id 回调函数线程识别参数
*/
CREATEDLL_API int setPushData(char *corp_id, char *staffid, char *push_data, char *seq_id, call_proxy_func proxy,
                              cancel_block_crypto_func cancel_crypto_func);

#ifdef __cplusplus
}
#endif

#endif // SAFECHAT_H