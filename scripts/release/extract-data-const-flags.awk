# Print the flags word of every LC_SEGMENT_64 named __DATA_CONST found in
# `otool -l` output, one per line, across every slice of a universal binary.
#
# A segname line is only honored when it directly follows a cmdsize line, so
# section entries (sectname/segname pairs without a preceding cmdsize) can
# never contribute a flags word. Each captured segment resets its state after
# the flags line for the same reason.
$1 == "cmd" { is_data_const = 0 }
$1 == "cmdsize" { expect_segname = 1; next }
expect_segname && $1 == "segname" {
	expect_segname = 0
	if ($2 == "__DATA_CONST") {
		is_data_const = 1
	}
	next
}
expect_segname { expect_segname = 0 }
is_data_const && $1 == "flags" { print $2; is_data_const = 0 }
