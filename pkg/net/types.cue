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

package net

// An #IP is an IP address in any of the forms the builtins of this
// package accept: its textual representation as a string or as bytes,
// such as "192.0.2.1" or "2001:db8::68", or a list of the address's
// byte values.
#IP: string | bytes | [...int]

// A #CIDR is a subnet in CIDR notation, such as "192.0.2.0/24", as a
// string or its bytes.
#CIDR: string | bytes
