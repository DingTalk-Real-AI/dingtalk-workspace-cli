// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package output

import (
	"bufio"
	"encoding/json"
	"io"
)

// writeNDJSON renders payload as newline-delimited JSON (https://ndjson.org):
//   - a top-level array       → one element per line
//   - an object that wraps a  → one element of that list per line
//     well-known list key       (items / results / data / records / value / ...)
//   - anything else           → a single line containing the whole value
//
// This is the streaming-friendly counterpart to `-f json`: each line is an
// independent, compact JSON document so consumers can `jq -c`, `while read`,
// or pipe into log pipelines without buffering the whole response.
//
// TODO(#252): consider honouring --fields per-line projection here too (today
// WriteFiltered already applies SelectFields before Write is reached, so this
// works, but a dedicated test would be good). Also decide whether non-list
// payloads should error under `-f ndjson` instead of degrading to one line.
func writeNDJSON(w io.Writer, payload any) error {
	normalized, err := roundTripJSON(payload)
	if err != nil {
		return err
	}

	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	// json.Encoder.Encode already appends a trailing newline per call.

	switch v := normalized.(type) {
	case []any:
		for _, item := range v {
			if err := enc.Encode(item); err != nil {
				return err
			}
		}
	case map[string]any:
		if loc := findDataList(v); loc != nil {
			for _, item := range loc.list {
				if err := enc.Encode(item); err != nil {
					return err
				}
			}
		} else {
			if err := enc.Encode(v); err != nil {
				return err
			}
		}
	default:
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// roundTripJSON normalizes an arbitrary Go value into the
// map[string]any / []any / scalar shape used by the rest of this package by
// marshalling and unmarshalling it through encoding/json.
func roundTripJSON(payload any) (any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
