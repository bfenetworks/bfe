// Copyright (c) 2026 The BFE Authors.
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

package mod_access_pb3

import (
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/bfenetworks/bfe/bfe_basic"

	bfe_access_pb3 "github.com/bfenetworks/bfe-access-pb/bfe_access_pb"
)

// maskSensitiveCredentials is the unified credential masking gate, invoked at
// the very end of requestLogGen (after all fields are assembled). It guarantees
// that the raw consumer API Key never appears in any field of the access log:
//
//   - Authenticated request: every occurrence of the raw Key is replaced with
//     the internal key id (AiBasicInfo.ClientKeyId).
//   - Unauthenticated request (e.g. INVALID_API_KEY): fields containing the
//     raw Key are emptied, so brute-forced keys are never persisted.
//   - Non-AI request (no AiBasicInfo): nothing to mask; field 49
//     (authorization) is already dropped by reqReqHeaderInfoGen.
//
// The scan is generic over the whole protobuf message (nested messages and
// repeated fields included), so fields added in the future are covered as well.
// See bfenetworks/bfe#1357.
func maskSensitiveCredentials(reqLog *bfe_access_pb3.RequestLog, req *bfe_basic.Request) {
	aiInfo := req.GetAiBasicInfo()
	if aiInfo == nil || aiInfo.ClientApiKey == "" {
		return
	}
	maskRawKeyInMessage(reqLog.ProtoReflect(), aiInfo.ClientApiKey, aiInfo.ClientKeyId)
}

// maskRawKeyInMessage recursively scans all populated fields of msg and masks
// any string value containing rawKey.
func maskRawKeyInMessage(msg protoreflect.Message, rawKey string, keyId string) {
	if !msg.IsValid() {
		return
	}

	fds := msg.Descriptor().Fields()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if !msg.Has(fd) {
			continue
		}

		switch {
		case fd.IsMap():
			// no string map fields in RequestLog; skip
			continue
		case fd.IsList():
			list := msg.Mutable(fd).List()
			switch fd.Kind() {
			case protoreflect.StringKind:
				for j := 0; j < list.Len(); j++ {
					if s := list.Get(j).String(); strings.Contains(s, rawKey) {
						list.Set(j, protoreflect.ValueOfString(maskRawKeyValue(s, rawKey, keyId)))
					}
				}
			case protoreflect.MessageKind, protoreflect.GroupKind:
				for j := 0; j < list.Len(); j++ {
					maskRawKeyInMessage(list.Get(j).Message(), rawKey, keyId)
				}
			}
		case fd.Kind() == protoreflect.StringKind:
			if s := msg.Get(fd).String(); strings.Contains(s, rawKey) {
				msg.Set(fd, protoreflect.ValueOfString(maskRawKeyValue(s, rawKey, keyId)))
			}
		case fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind:
			maskRawKeyInMessage(msg.Get(fd).Message(), rawKey, keyId)
		}
	}
}

// maskRawKeyValue replaces rawKey with keyId. When keyId is unknown
// (unauthenticated request), the whole value is emptied so the raw Key is
// never persisted.
func maskRawKeyValue(value string, rawKey string, keyId string) string {
	if keyId == "" {
		return ""
	}
	return strings.ReplaceAll(value, rawKey, keyId)
}
