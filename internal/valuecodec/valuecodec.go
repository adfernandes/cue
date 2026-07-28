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

// Package valuecodec defines the value-plane codec type for the dynamic
// encoding registry: constructors that decode/encode directly through
// cue.Value. Syntax can express constraints, defaults, definitions, and
// conjunctions, but rebuilding through syntax cannot preserve evaluated graph
// identity, structure sharing, or original conjunct provenance; the direct
// value path can.
//
// It is a leaf package so that both the public registration API
// (cuelang.org/go/unstable/encodingregistry) and the encoding dispatch
// (cuelang.org/go/internal/encoding) can name the value-plane types
// without either importing the other, and without internal/filetypes —
// which stores a *Codec opaquely — needing a typed cue dependency.
package valuecodec

import (
	"io"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
)

// A ValueDecoder decodes documents of a registered encoding directly
// into cue.Value. Decode returns one value per document and io.EOF when
// no documents remain. Every value returned with a nil error must exist
// (its Exists method must report true): error values (bottom) are valid
// payload, but a zero cue.Value represents no document at all and is a
// contract violation that the caller reports as a decoding error.
type ValueDecoder interface {
	Decode() (cue.Value, error)
}

// A ValueEncoder encodes cue.Value into a registered encoding. Close
// flushes any buffered output; it does not close the underlying writer.
type ValueEncoder interface {
	Encode(cue.Value) error
	Close() error
}

// A Codec holds the value-plane constructors of a registered encoding.
// Either constructor may be nil (decode-only or encode-only). The
// decoder receives the *cue.Context to build values into.
type Codec struct {
	NewValueDecoder func(*cue.Context, *build.File, io.Reader) (ValueDecoder, error)
	NewValueEncoder func(*build.File, io.Writer) (ValueEncoder, error)
}
