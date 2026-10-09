import random
import re
import sys
import time

# 支持的表单控件类型
COMPONENT_TYPES = [
    "AddressField",
    "CalculateField",
    "CascadeField",
    "ColumnLayout",
    "DDAttachment",
    "DDDateField",
    "DDDateRangeField",
    "DDMultiSelectField",
    "DDPhotoField",
    "DDSelectField",
    "DepartmentField",
    "ExternalContactField",
    "FormRelateField",
    "IdCardField",
    "InnerContactField",
    "InvoiceField",
    "MoneyField",
    "NumberField",
    "OcrIdCardField",
    "OcrTextField",
    "PhoneField",
    "RecipientAccountField",
    "RelateField",
    "SeqNumberField",
    "SignatureField",
    "StarRatingField",
    "TableField",
    "TextField",
    "TextNote",
    "TextareaField",
    "TimeAndLocationField",
]


def _to_base36(number: int) -> str:
    """Convert a non-negative integer to a base-36 string."""
    if number == 0:
        return "0"
    chars = "0123456789abcdefghijklmnopqrstuvwxyz"
    result = []
    while number > 0:
        result.append(chars[number % 36])
        number //= 36
    return "".join(reversed(result))


def generate_uuid(length: int = 6) -> str:
    """
    Generate a unique UUID suffix.

    Equivalent to the TypeScript function:
        export const generateUuid = (length = 6) => { ... }

    Args:
        length: Number of random digits to use (must not be 0, or duplicates may occur).

    Returns:
        An uppercase base-36 string.
    """
    # length 不能为0，否则有重复ID出现
    # Math.random().toString().substr(3, length) + Date.now()
    random_digits = str(random.random()).replace("0.", "")[:length]
    timestamp_ms = int(time.time() * 1000)
    combined = int(random_digits + str(timestamp_ms))

    # Number(...).toString(36).toUpperCase()
    uuid = _to_base36(combined).upper()

    # 如果有科学计数的id，需要兼容
    if re.match(r"^.\..+\(.+\)$", uuid):
        match = re.search(r"\w{10,}", uuid)
        if match:
            uuid = match.group(0)

    return uuid


def generate_form_component_id(component_type: str) -> str:
    """
    Generate a form component ID in the format: {ComponentType}_{UUID}

    Examples:
        TextField_1F68U92WU5Y80
        DDDateField_DATE001
        MoneyField_ZZ6UVVOE5DS0

    Args:
        component_type: One of the supported component type names.

    Returns:
        A complete form component ID string.
    """
    return f"{component_type}_{generate_uuid()}"


if __name__ == "__main__":
    args = sys.argv[1:]
    if args:
        # 先校验全部类型，再批量输出（可传入多个类型，每个输出一个 ID）
        for component_type in args:
            if component_type not in COMPONENT_TYPES:
                print(f"Error: Unknown component type '{component_type}'")
                print(f"Supported types: {', '.join(COMPONENT_TYPES)}")
                sys.exit(1)
        for component_type in args:
            print(generate_form_component_id(component_type))
    else:
        # No argument: generate one ID for each component type as demo
        print("Usage: python generate_form_component_id.py <ComponentType> [<ComponentType> ...]")
        print(f"\nSupported types ({len(COMPONENT_TYPES)}):")
        for ct in COMPONENT_TYPES:
            print(f"  {generate_form_component_id(ct)}")
