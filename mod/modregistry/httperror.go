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

package modregistry

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cuelabs.dev/go/oci/ociregistry"
)

// registryError adapts an error returned by a registry operation for
// display to end users. The registry client describes HTTP failures in
// full, including content types and raw response bodies, which buries
// the useful part of errors such as rate limits behind noise; rewrite
// the message to the HTTP status plus whatever concise message the
// server provided. The returned error keeps err in its chain, so
// [errors.Is] and [errors.As] behave as before.
func registryError(err error) error {
	if err == nil {
		return nil
	}
	if herr, ok := errors.AsType[ociregistry.HTTPError](err); ok {
		return &registryHTTPError{err: err, herr: herr}
	}
	return err
}

type registryHTTPError struct {
	err  error
	herr ociregistry.HTTPError
}

func (e *registryHTTPError) Unwrap() error { return e.err }

func (e *registryHTTPError) Error() string {
	statusCode := e.herr.StatusCode()
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d %s", statusCode, http.StatusText(statusCode))
	if detail := e.detail(statusCode); detail != "" {
		sb.WriteString(": ")
		sb.WriteString(detail)
	}
	if after := retryAfter(e.herr.Response()); after != "" {
		fmt.Fprintf(&sb, " (retry after %s)", after)
	}
	return sb.String()
}

// detail returns the server-provided message worth showing alongside
// the HTTP status, if any.
func (e *registryHTTPError) detail(statusCode int) string {
	// A structured OCI error carries the server's message; its code
	// tends to restate the HTTP status, so include it only when it adds
	// information.
	if werr, ok := errors.AsType[*ociregistry.WireError](e.err); ok {
		if werr.Message != "" && !restatesStatus(werr.Message, statusCode) {
			return werr.Message
		}
		// A code such as QUOTA_EXHAUSTED reads as "quota exhausted".
		code := strings.ToLower(strings.ReplaceAll(werr.Code_, "_", " "))
		if !restatesStatus(code, statusCode) {
			return code
		}
		return ""
	}
	// The response was not a structured error; include the body only
	// when it reads as short plain text, as dumping markup or binary
	// data would bury the failure rather than explain it.
	body := bytes.TrimSpace(e.herr.ResponseBody())
	const maxBodyDetail = 256
	if len(body) == 0 || len(body) > maxBodyDetail || body[0] == '<' || !utf8.Valid(body) {
		return ""
	}
	s := strings.Join(strings.Fields(string(body)), " ")
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	if restatesStatus(s, statusCode) {
		return ""
	}
	return s
}

// restatesStatus reports whether s merely restates the given HTTP
// status, such as "toomanyrequests" or "429 Too Many Requests" for
// [http.StatusTooManyRequests].
func restatesStatus(s string, statusCode int) bool {
	norm := func(s string) string {
		var sb strings.Builder
		for _, r := range s {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				sb.WriteRune(unicode.ToLower(r))
			}
		}
		return sb.String()
	}
	s = norm(s)
	text := norm(http.StatusText(statusCode))
	return s == text || s == norm(strconv.Itoa(statusCode))+text
}

// retryAfter renders the response's Retry-After header, if any,
// as a duration or an RFC 3339 time.
func retryAfter(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return ""
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return ""
		}
		return (time.Duration(secs) * time.Second).String()
	}
	if t, err := http.ParseTime(v); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return ""
}
