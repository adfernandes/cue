// Copyright 2026 The CUE Authors
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

// This file is maintained by hand: it is the authoritative description
// of the package's API for tooling such as editor completion and hover.
// Its consistency with the registered builtins is checked by the tests
// in cuelang.org/go/pkg.

@experiment(functions)
@pure()

// Package hmac implements the Keyed-Hash Message Authentication Code (HMAC) as
// defined in U.S. Federal Information Processing Standards Publication 198.
//
// An HMAC is a cryptographic hash that uses a key to sign a message.
// The receiver verifies the hash by recomputing it using the same key.
package hmac

MD5: "MD5"

SHA1: "SHA1"

SHA224: "SHA224"

SHA256: "SHA256"

SHA384: "SHA384"

SHA512: "SHA512"

SHA512_224: "SHA512_224"

SHA512_256: "SHA512_256"

// Sign returns the HMAC signature of the data, using the provided key and hash function.
//
// Supported hash functions: "MD5", "SHA1", "SHA224", "SHA256", "SHA384", "SHA512", "SHA512_224",
// and "SHA512_256".
Sign: func(hashName: string, key: bytes | string, data: bytes | string) -> bytes
