#include <stdarg.h>
#include <stddef.h>

extern int vsnprintf(char *buffer, size_t count, const char *format, va_list args);

// The vendor archives reference legacy MSVCRT names that UCRT toolchains omit.
static int safechat_vsnprintf(char *buffer, size_t count, const char *format,
                             va_list args) {
  return vsnprintf(buffer, count, format, args);
}

int safechat_msvcrt_vsnprintf(char *buffer, size_t count, const char *format,
                             va_list args) __asm__("_vsnprintf");
int safechat_msvcrt_vsnprintf(char *buffer, size_t count, const char *format,
                             va_list args) {
  return safechat_vsnprintf(buffer, count, format, args);
}

int safechat_msvcrt_snprintf(char *buffer, size_t count, const char *format,
                            ...) __asm__("_snprintf");
int safechat_msvcrt_snprintf(char *buffer, size_t count, const char *format,
                            ...) {
  int result;
  va_list args;

  va_start(args, format);
  result = safechat_vsnprintf(buffer, count, format, args);
  va_end(args);
  return result;
}

#if defined(__x86_64__)
typedef int (*safechat_snprintf_fn)(char *, size_t, const char *, ...);
typedef int (*safechat_vsnprintf_fn)(char *, size_t, const char *, va_list);

safechat_snprintf_fn safechat_msvcrt_import_snprintf
    __asm__("__imp__snprintf") = safechat_msvcrt_snprintf;
safechat_vsnprintf_fn safechat_msvcrt_import_vsnprintf
    __asm__("__imp__vsnprintf") = safechat_msvcrt_vsnprintf;
#endif
