// Copyright 2026 CUE Authors
//
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

package encodingregistry_test

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/internal/filetypes"
	"cuelang.org/go/unstable/encodingregistry"
)

// The tests in this package register synthetic encodings in the
// process-global registry. The registry is add-only and freezes on
// first use, so tests reset it via the internal testing hook and, per
// test, use distinct encoding names to stay independent of ordering.
func reset(t *testing.T) {
	t.Helper()
	filetypes.ResetDynamicRegistryForTesting()
	t.Cleanup(filetypes.ResetDynamicRegistryForTesting)
}

// kvDecoder decodes the synthetic "kv" test format: one document of
// newline-separated key=value pairs, decoded as a struct of string
// fields.
type kvDecoder struct {
	data []byte
	err  error
	done bool
}

func newKVDecoder(f *build.File, r io.Reader) (encodingregistry.Decoder, error) {
	data, err := io.ReadAll(r)
	return &kvDecoder{data: data, err: err}, nil
}

func (d *kvDecoder) Decode() (ast.Expr, error) {
	if d.err != nil {
		return nil, d.err
	}
	if d.done {
		return nil, io.EOF
	}
	d.done = true
	st := &ast.StructLit{}
	for line := range strings.Lines(string(d.data)) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("kv: malformed line %q", line)
		}
		st.Elts = append(st.Elts, &ast.Field{
			Label: ast.NewString(key),
			Value: ast.NewString(value),
		})
	}
	return st, nil
}

// kvEncoder encodes a struct of string fields back to key=value lines.
type kvEncoder struct {
	w io.Writer
}

func newKVEncoder(f *build.File, w io.Writer) (encodingregistry.Encoder, error) {
	return &kvEncoder{w: w}, nil
}

func (e *kvEncoder) Encode(n ast.Node) error {
	st, ok := n.(*ast.StructLit)
	if !ok {
		return fmt.Errorf("kv: cannot encode %T", n)
	}
	for _, elt := range st.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			return fmt.Errorf("kv: cannot encode %T", elt)
		}
		key, _, err := ast.LabelName(field.Label)
		if err != nil {
			return err
		}
		lit, ok := field.Value.(*ast.BasicLit)
		if !ok {
			return fmt.Errorf("kv: cannot encode value %T", field.Value)
		}
		value, err := literal.Unquote(lit.Value)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(e.w, "%s=%s\n", key, value); err != nil {
			return err
		}
	}
	return nil
}

func (e *kvEncoder) Close() error { return nil }
