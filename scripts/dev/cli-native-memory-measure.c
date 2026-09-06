/* Measure a native CLI's kernel peak RSS from a small post-exec parent.
 * Python's inherited pre-exec RSS can otherwise dominate a tiny native CLI.
 * Public wrappers use the separate simultaneous process-tree sampler.
 */
#define _DEFAULT_SOURCE
#define _DARWIN_C_SOURCE
#define _POSIX_C_SOURCE 200809L
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include <sys/resource.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

int main(int argc, char **argv) {
    if (argc < 6) return 2;
    char *end;
    unsigned long timeout = strtoul(argv[4], &end, 10);
    if (*end || !timeout || timeout > 3600) return 2;
    int out = open(argv[1], O_WRONLY | O_CREAT | O_TRUNC, 0600);
    int err = open(argv[2], O_WRONLY | O_CREAT | O_TRUNC, 0600);
    int in = open("/dev/null", O_RDONLY);
    if (out < 0 || err < 0 || in < 0) return 2;
    struct timespec started;
    if (clock_gettime(CLOCK_MONOTONIC, &started)) return 2;
    pid_t child = fork();
    if (child < 0) return 2;
    if (!child) {
        if (dup2(out, STDOUT_FILENO) < 0 || dup2(err, STDERR_FILENO) < 0 ||
            dup2(in, STDIN_FILENO) < 0 || chdir(argv[3])) _exit(126);
        close(out); close(err); close(in);
        execvp(argv[5], &argv[5]);
        perror("exec native memory target");
        _exit(127);
    }
    close(out); close(err); close(in);
    int timed_out = 0;
    int status;
    struct rusage usage;
    for (;;) {
        pid_t waited = wait4(child, &status, WNOHANG, &usage);
        if (waited == child) break;
        if (waited < 0 && errno != EINTR) return 2;
        struct timespec now;
        if (clock_gettime(CLOCK_MONOTONIC, &now)) {
            kill(child, SIGKILL);
            while (wait4(child, &status, 0, &usage) < 0 && errno == EINTR) {}
            return 2;
        }
        time_t elapsed = now.tv_sec - started.tv_sec;
        if (elapsed >= (time_t)timeout) {
            timed_out = 1;
            if (kill(child, SIGKILL) && errno != ESRCH) return 2;
            while (wait4(child, &status, 0, &usage) < 0) {
                if (errno != EINTR) return 2;
            }
            break;
        }
        struct timespec interval = {.tv_sec = 0, .tv_nsec = 1000000};
        while (nanosleep(&interval, &interval) && errno == EINTR) {}
    }
    int code = WIFEXITED(status) ? WEXITSTATUS(status) : -WTERMSIG(status);
#ifdef __APPLE__
    unsigned long long rss = (unsigned long long)usage.ru_maxrss;
#else
    unsigned long long rss = (unsigned long long)usage.ru_maxrss * 1024;
#endif
    printf("{\"returncode\":%d,\"timed_out\":%s,\"measurement\":{\"kernel_process_peak_rss_bytes\":%llu}}\n",
           code, timed_out ? "true" : "false", rss);
    return 0;
}
