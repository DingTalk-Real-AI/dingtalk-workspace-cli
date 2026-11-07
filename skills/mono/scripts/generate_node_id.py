import random
import sys


def get_uuid() -> str:
    """
    生成流程节点 ID，格式为 {4位hex}_{4位hex}。

    Equivalent to the TypeScript function:
        export function getUUID() {
            function s4() {
                return Math.floor((1 + Math.random()) * 0x10000)
                    .toString(16)
                    .substring(1);
            }
            return `${s4()}_${s4()}`;
        }

    Returns:
        A string in the format "xxxx_xxxx" where x is a hex digit.
    """

    def s4() -> str:
        # Math.floor((1 + Math.random()) * 0x10000) produces a number in [0x10000, 0x1FFFF]
        # .toString(16) gives "1xxxx", .substring(1) removes the leading "1"
        return format(random.randint(0x0000, 0xFFFF), "04x")

    return f"{s4()}_{s4()}"


if __name__ == "__main__":
    # 可选参数：生成数量（默认 1）
    count = int(sys.argv[1]) if len(sys.argv) > 1 else 1
    for _ in range(count):
        print(get_uuid())
