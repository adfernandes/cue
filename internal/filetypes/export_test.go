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

package filetypes

// AspectNamesForTest exposes the aspect-name table so external tests
// can check it stays in one-to-one correspondence with the Boolean
// aspect fields of types.cue's #FileInfo template.
func AspectNamesForTest() []string {
	return aspectNames[:]
}
