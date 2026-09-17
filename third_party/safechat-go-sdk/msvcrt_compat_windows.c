/*
 * Legacy MSVCRT import stubs for the libsafechat.a.
 *
 * lib/windows_amd64/libsafechat.a , which references
 * the legacy dllimport symbols __imp__vsnprintf and __imp__vsnwprintf. An
 * msvcrt import library exports both, but the mingw-w64 UCRT import libraries
 * do not, so a UCRT link fails without the definitions below.
 *
 * Three rules keep this file from reintroducing the startup crash it exists to
 * avoid (exception 0xc00000fd, STATUS_STACK_OVERFLOW, inside init):
 *
 *   1. It is compiled for UCRT targets only. Under an msvcrt toolchain
 *      libmsvcrt.a already exports __imp__vsnprintf, __imp__snprintf and
 *      __imp__vsnwprintf, so defining our own would merely shadow the CRT.
 *
 *   2. It defines the __imp__* data symbols only, never the _vsnprintf or
 *      _snprintf function symbols themselves. Defining _vsnprintf replaces the
 *      CRT's own copy for every caller in the image, including libmingwex's
 *      __ms_vsnprintf - the msvcrt-semantics wrapper that *is* vsnprintf
 *      whenever __USE_MINGW_ANSI_STDIO is 0, and that calls _vsnprintf
 *      internally. A shim forwarding _vsnprintf to vsnprintf therefore closes
 *      the cycle _vsnprintf -> vsnprintf -> __ms_vsnprintf -> _vsnprintf, which
 *      exhausts the 2 MB stack before main() ever runs.
 *
 *   3. The implementations call __mingw_vsnprintf / __mingw_vsnwprintf, the
 *      self-contained formatters in libmingwex, and never plain vsnprintf.
 *      Those cannot route back into this file no matter how the toolchain
 *      resolves the standard names.
 *
 * safechat's own objects need none of this: csrc is built with
 * __USE_MINGW_ANSI_STDIO=1 and calls the standard names, so it binds directly
 * to __mingw_* and stays CRT-neutral. See rebuild_win_amd64.bat.
 */

#if defined(_UCRT) && defined(__x86_64__)

#include <stdarg.h>
#include <stddef.h>
#include <wchar.h>

int __mingw_vsnprintf(char *buffer, size_t count, const char *format,
                      va_list args);
int __mingw_vsnwprintf(wchar_t *buffer, size_t count, const wchar_t *format,
                       va_list args);

/* The msvcrt contract these callers were compiled against differs from C99: on
 * truncation _vsnprintf returns a negative value, while the __mingw_* (C99)
 * formatters return the length that would have been needed. Translate, so a
 * caller checking for a negative result still sees truncation.
 *
 * The NUL byte that msvcrt omits when it truncates is deliberately kept - the
 * __mingw_* formatters always write it. No caller can depend on its absence,
 * and dropping it would hand out unterminated strings. */
static int safechat_compat_vsnprintf(char *buffer, size_t count,
                                     const char *format, va_list args) {
  int needed = __mingw_vsnprintf(buffer, count, format, args);

  if (needed < 0)
    return -1;
  if (count != 0 && (size_t)needed >= count)
    return -1;
  return needed;
}

static int safechat_compat_vsnwprintf(wchar_t *buffer, size_t count,
                                      const wchar_t *format, va_list args) {
  int needed = __mingw_vsnwprintf(buffer, count, format, args);

  if (needed < 0)
    return -1;
  if (count != 0 && (size_t)needed >= count)
    return -1;
  return needed;
}

static int safechat_compat_snprintf(char *buffer, size_t count,
                                    const char *format, ...) {
  va_list args;
  int result;

  va_start(args, format);
  result = safechat_compat_vsnprintf(buffer, count, format, args);
  va_end(args);
  return result;
}

typedef int (*safechat_snprintf_fn)(char *, size_t, const char *, ...);
typedef int (*safechat_vsnprintf_fn)(char *, size_t, const char *, va_list);
typedef int (*safechat_vsnwprintf_fn)(wchar_t *, size_t, const wchar_t *,
                                      va_list);

safechat_snprintf_fn safechat_imp_snprintf
    __asm__("__imp__snprintf") = safechat_compat_snprintf;
safechat_vsnprintf_fn safechat_imp_vsnprintf
    __asm__("__imp__vsnprintf") = safechat_compat_vsnprintf;
safechat_vsnwprintf_fn safechat_imp_vsnwprintf
    __asm__("__imp__vsnwprintf") = safechat_compat_vsnwprintf;

#endif /* _UCRT && __x86_64__ */

