// Copyright (c) 2020 The BFE Authors.
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
package waf_rule

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"

	"github.com/baidu/go-lib/log"
)

const (
	token_in_param_low_limit = 2
)

/* match a list of sql words.
 * If at least 2 words hits, this query is suspicious. */
func matchSuspStrings(keyValues map[string][]string, matchStrings []string) bool {
	matchCounter := 0

	for _, values := range keyValues {
		matchCounter = 0
		for _, value := range values {
			for _, s := range matchStrings {
				if strings.Contains(value, s) {
					matchCounter += 1
					if matchCounter >= 2 {
						log.Logger.Info("matchSuspStrings(): %s", value)
						return true
					}
				}
			}
		}
	}

	return false
}

/* used in function replaceBlankSpace,
   test if token is a blank string. */
func matchBlankToken(token byte) bool {
	match := false

	switch token {
	case 0x09:
		fallthrough
	case 0x0B:
		fallthrough
	case 0x0C:
		fallthrough
	case 0x0D:
		fallthrough
	case 0x20:
		fallthrough
	case 0xA0:
		match = true
	}

	return match
}

/* change "0x09 0x0B 0x0C 0x0D 0x20 0xA0" to "0x20" */
func replaceBlankSpace(rawUrl string) string {
	byteStr := bytes.NewBuffer(nil)

	// a small state machine
	for i := 0; i < len(rawUrl); i++ {
		if matchBlankToken((rawUrl)[i]) {
			byteStr.WriteByte(' ')
		} else {
			byteStr.WriteByte(rawUrl[i])
		}
	}

	return byteStr.String()
}

/* change 0xA0 to 0x20 */
func replace0x0A(rawUrl string) string {
	byteStr := bytes.NewBuffer(nil)

	// a small state machine
	for i := 0; i < len(rawUrl); i++ {
		if rawUrl[i] == 0x0a {
			byteStr.WriteByte(' ')
		} else {
			byteStr.WriteByte((rawUrl)[i])
		}
	}

	return byteStr.String()
}

/* remove chars between start and end. [start, end] */
func removeBetweenBytes(urlStr string, start byte, end byte) string {
	byteStr := bytes.NewBuffer(nil)
	startRemove := false

	for i := 0; i < len(urlStr); i++ {
		ch := urlStr[i]
		if startRemove {
			if ch == end {
				// remove end
				startRemove = false
			}
		} else {
			if ch == start {
				// begin to remove
				startRemove = true
				byteStr.WriteByte(' ')
			} else {
				// normal copy
				byteStr.WriteByte(ch)
			}
		}
	}

	return byteStr.String()
}

/* remove chars between start and end. [start, end] */
func removeBetweenStrByte(urlStr string, start string, end byte) string {
	byteStr := bytes.NewBuffer(nil)
	startRemove := true

	// find first appear is enough
	pos := strings.Index(urlStr, start)
	if pos < 0 {
		return urlStr
	}

	byteStr.WriteString(urlStr[0:pos])
	startPos := pos + len(start)
	str := urlStr[startPos:]

	for i := 0; i < len(str); i++ {
		ch := str[i]

		if startRemove {
			if ch == end { // 0x0a
				// remove end
				startRemove = false
				byteStr.WriteByte(' ')
			}
			// ch is removed
		} else {
			// normal copy
			byteStr.WriteByte(ch)
		}
	}

	return byteStr.String()
}

/* process "/*... " */
func processNormalComment(comment string) (string, int) {
	byteStr := bytes.NewBuffer(nil)
	offset := 0

	// skip "/*" prefix of comment
	// for example, "/*/" is illegal comment, but would be treated as legal comment if not skip "/*"
	pos := strings.Index(comment[2:], "*/")
	if pos > -1 {
		pos += 2
	}

	if pos > 0 {
		// complete
		byteStr.WriteString(comment[pos+2:])
		offset = pos + 2
	} else {
		// incomplete. drop all left string
		offset = len(comment)
	}

	return byteStr.String(), offset
}

/* process comments in comments ... */
func processInnerComment(comment string) (string, int) {
	byteStr := bytes.NewBuffer(nil)
	offset := 0

	startPos := strings.Index(comment, "/*")
	if startPos < 0 {
		// add all remaining
		byteStr.WriteString(comment)
		offset = len(comment)
	} else {
		// get a comment inside current comment
		byteStr.WriteString(comment[0:startPos])
		offset += startPos

		if comment[startPos+2] == '!' {
			// a special comment
			// call recursively
			newStr, size := processSpecialComment(comment[startPos:])
			byteStr.WriteString(newStr)
			offset += size
		} else {
			// a normal comment
			// remove it
			newStr, size := processNormalComment(comment[startPos:])
			offset += size
			byteStr.WriteByte(' ') // " " replace "/*...*/"

			// there may be another comment after it ...
			newStr, size = processInnerComment(comment[startPos+size:])
			byteStr.WriteString(newStr)
			offset += size
		}
	}

	return byteStr.String(), offset
}

/* process "/*!.. " */
func processSpecialComment(comment string) (string, int) {
	byteStr := bytes.NewBuffer(nil)
	byteStr.WriteByte(' ') // replace "/*!...*/" with " "
	offset := 3            // remove "/*!"

	// check if "/*!NNNNN"
	pos := 0
	hasFiveNumber := true
	if len(comment) >= 8 {
		for i := 0; i < 5; i++ {
			// start from index 3 (/*!)
			if comment[3+i] >= '0' && comment[3+i] <= '9' {
				continue
			} else {
				hasFiveNumber = false
				break
			}
		}
	} else {
		hasFiveNumber = false
	}

	var keep bool
	if hasFiveNumber {
		// get the number
		n, _ := strconv.Atoi(comment[3 : 3+5])
		if n <= 50022 {
			keep = true
		} else {
			keep = false
		}

		// escape 5 number
		pos += 5
		offset += 5
	} else {
		// not five number
		keep = true
	}

	if keep {
		// continue add byte, start from current pos
		// until get "/*"
		resultStr, size := processInnerComment(comment[3+pos:])

		// remove "*/"
		last := strings.Index(resultStr, "*/")
		if last >= 0 {
			byteStr.WriteString(resultStr[0:last])
			byteStr.WriteByte(' ') // replace "*/" with ' '
			byteStr.WriteString(resultStr[last+2:])
		} else {
			// incomplete
			if strings.Index(comment, "*/") > 0 {
				// shared ending "*/", act as if compelete comment
				byteStr.WriteString(resultStr)
			} else {
				// no shared ending "*/"
				return " ", len(comment)
			}
		}
		offset += size

		return byteStr.String(), offset
	} else {
		// counter for "/*"
		// we do not using recursion,
		// so the only value is 0 or 1
		counter := 1

		// the first position result string
		index := 2

		// use a state machine.
		// state 0: has first "/*!" only.
		//			if get a new "/*", change state to 1 and counter += 1.
		//			if get a "*/",  counter -= 1 (match a "/*")
		// state 1: get a inner "/*"
		//			if we get a "*/", change state to 0.
		//			do not change state if get a "/*" (no recursion).
		state := 0

		for i := 2; i < len(comment); i++ {
			ch := comment[i]

			if ch == '*' {
				if i+1 < len(comment) && comment[i+1] == '/' {
					// meet "*/", remove a "/*"
					//hasFinishTag = true
					index = i + 1
					i += 1

					if state == 1 {
						// match inner "/**/"
						state = 0
					}

					counter -= 1
					if counter == 0 {
						// match /*! */ complete
						break
					}
				}
			} else if ch == '/' {
				if i+1 < len(comment) && comment[i+1] == '*' {
					switch state {
					case 0: // only has first "/*!"
						state = 1
						i += 1
						counter += 1 // get a new "/*"
					case 1:
						// has got "/*", so counter not add
						i += 1
					}
				}
			} else {
				//remove other char
			}

		}

		if counter == 0 {
			if index+1 < len(comment) {
				return comment[index+1:], index + 1
			} else {
				// no left string
				return " ", index
			}
		}

		// counter < 0 is impossible
		// now counter > 0
		// no "*/" find
		return " ", len(comment)
	}
}

/* replace multiple blank spaces to single blank space */
func replaceMultipleBlankSpaces(str string) string {
	oriByte := []byte(str)
	byteBuf := bytes.NewBuffer(nil)

	startId := 0
	endId := len(oriByte) - 1

	// find the end of the string ignore space character
	// its advantage is not copied, but trim need
	for endId >= 0 && oriByte[endId] == ' ' {
		endId--
	}
	endId++

	// find the start of the string ignore space character
	// its advantage is not copied, but trim need
	for startId < endId && oriByte[startId] == ' ' {
		startId++
	}

	// replace multiple blank spaces to single blank space
	// and copy the old string to new string
	pos := startId + 1
	for pos < endId {
		if oriByte[pos] == ' ' {
			byteBuf.Write(oriByte[startId:pos])
			for pos < endId && oriByte[pos+1] == ' ' {
				pos++
			}
			startId = pos
		}
		pos++
	}

	// deal with the aftermath
	byteBuf.Write(oriByte[startId:endId])

	return byteBuf.String()
}

/* remove blank space before and after "+,-,*,/" */
func removeBlankSpaceForOperators(str string) string {
	byteStr := bytes.NewBuffer(nil)
	ops := []byte{'+', '-', '*', '/', '=', '%'}

	for i := 0; i < len(str); i++ {
		if str[i] == ' ' {
			// check ' ' before operators
			if i < (len(str)-2) && isSpecialChar(str[i+1], ops) {
				continue // remove it
			}

			// check ' ' after operators
			if i > 2 && isSpecialChar(str[i-1], ops) {
				continue // remove it
			}
		}

		// other chars
		byteStr.WriteByte(str[i])
	}

	return byteStr.String()
}

// deal with /*...*/ or /*!...*/
// every time we remove a compelete /*...*/ or /*!...*/
func onComments(workStr string) (string, string) {
	var resultStr string

	// "/*X"
	if len(workStr) < 3 {
		return workStr, ""
	}

	//var tmp int
	// '!' must follow with "/*", otherwise not a command
	if workStr[2] == '!' {
		resultStr, _ = processSpecialComment(workStr)
	} else {
		resultStr, _ = processNormalComment(workStr)
	}

	// if resultStr contains "/*", it means it just remove one "/**/" header.
	// no string shuold be merged into result
	if strings.Index(resultStr, "/*") >= 0 {
		return "", resultStr
	}

	// resultStr has no comments in it.
	// all the work string has been processed.
	// the new work string either equals to resultStr,
	// or the empty string ("").

	return "", resultStr
}

/* process with sql comment */
func noiseFiltering(paramStr string) (string, error) {
	byteStr := bytes.NewBuffer(nil)
	var resultStr string
	var workStr string

	for i := 0; i < len(paramStr); i++ {
		ch := paramStr[i]

		switch ch {
		case '#':
			workStr = paramStr[i:]
			paramStr = removeBetweenBytes(workStr, '#', 10)
			i = -1 // restart; because i++ will be executed, so i = -1
			continue

		case '-':
			if i < len(paramStr)-2 {
				if paramStr[i+1] == '-' && paramStr[i+2] == ' ' {
					workStr = paramStr[i:]
					paramStr = removeBetweenStrByte(workStr, "-- ", 10)
					i = -1 // restart
					continue
				}
			}
			byteStr.WriteByte(ch)

		case '/':
			if i < len(paramStr)-1 {
				if paramStr[i+1] == '*' {
					workStr = paramStr[i:]
					resultStr, paramStr = onComments(workStr)
					byteStr.WriteByte(' ')
					byteStr.WriteString(resultStr)
					i = -1 // restart
					continue
				}
			}
			byteStr.WriteByte(ch)

		default:
			byteStr.WriteByte(ch)
		}
	}

	return byteStr.String(), nil
}

/* add a space(" ") infront of '(', '"', ''', and remove redundent spaces */
func formatUrl(urlStr string) string {
	byteStr := bytes.NewBuffer(nil)

	for i := 0; i < len(urlStr); i++ {
		switch urlStr[i] {
		case '(':
			byteStr.WriteString(" (")
		case '\'':
			byteStr.WriteString(" '")
		case '"':
			byteStr.WriteString(" \"")
		case '=':
			byteStr.WriteString(" =")
		case '+':
			byteStr.WriteString(" +")
		case '-':
			byteStr.WriteString(" -")
		case '*':
			byteStr.WriteString(" *")
		case '/':
			byteStr.WriteString(" /")
		case '%':
			byteStr.WriteString(" %")
		case '^':
			byteStr.WriteString(" ^")
		case '!':
			byteStr.WriteString(" !")
			if i+1 < len(urlStr) {
				if urlStr[i+1] == '=' {
					byteStr.WriteString("=")
					i++
				}
			}
		case '~':
			byteStr.WriteString(" ~")
		case '|':
			byteStr.WriteString(" |")
			if i+1 < len(urlStr) {
				if urlStr[i+1] == '|' {
					byteStr.WriteString("|")
					i++
				}
			}
		case '&':
			byteStr.WriteString(" &")
			if i+1 < len(urlStr) {
				if urlStr[i+1] == '&' {
					byteStr.WriteString("&")
					i++
				}
			}
		case ':':
			byteStr.WriteString(" :")
			if i+1 < len(urlStr) {
				if urlStr[i+1] == '=' {
					byteStr.WriteString("=")
					i++
				}
			}
		case '>':
			byteStr.WriteString(" >")
			if i+1 < len(urlStr) {
				if urlStr[i+1] == '=' {
					byteStr.WriteString("=")
					i++
				}
			}
		case '<':
			byteStr.WriteString(" <")
			if i+1 < len(urlStr) {
				if urlStr[i+1] == '=' {
					byteStr.WriteString("=")
					i++
					if i+2 < len(urlStr) {
						if urlStr[i+2] == '>' {
							byteStr.WriteString(">")
							i++
						}
					}
				} else if urlStr[i+1] == '>' {
					byteStr.WriteString(">")
					i++
				}
			}
		default:
			byteStr.WriteByte(urlStr[i])
		}
	}

	return byteStr.String()
}

// check if a token is numbers
func isNumberToken(token string) bool {
	pos := -1
	for i, ch := range token {
		if ch >= '0' && ch <= '9' {
			pos = i
		} else {
			break
		}
	}

	if pos >= 0 {
		return true
	}

	return false
}

var suspicious_tokens []byte
var contain_tokens []string

func init() {
	suspicious_tokens = []byte{'(', ')', '@', '!',
		'=', '>', '<',
		'+', '-', '*',
		'/', '!', '%',
		'~', ':', '^',
		'|', '&'}
	contain_tokens = []string{"(", ")", "=", ">", "<",
		"+", "-", "*", "/",
		"!", "%", "~", ":=",
		"^", "|", "&"}
}

// check suspicious words in tokens
func checkTokens(allTokens []string) bool {
	// remove blank tokens
	var tokens []string
	for _, v := range allTokens {
		if v != "" {
			tokens = append(tokens, v)
		}
	}

	if len(tokens) < token_in_param_low_limit {
		// need not to regexp check
		return false
	}

	// does first token match "true" or "false"
	firstToken := strings.ToLower(tokens[0])
	if firstToken == "true" || firstToken == "false" {
		// go to regexp check
		return true
	}

	// check first token
	if isSpecialChar(tokens[0][0], suspicious_tokens) {
		// need check with regexp
		return true
	}

	// check if first token is a number
	if isNumberToken(tokens[0]) {
		// need check with regexp
		return true
	}

	// if not a number, check second token
	if containWords(tokens[1], contain_tokens) {
		// need check with regexp
		return true
	}

	needCheck := false
	// find special words in all tokens
	for _, token := range tokens {
		if containWords(token, []string{"'", "\""}) {
			// contain special words, need to proceed on
			needCheck = true
			break
		}
	}

	return needCheck
}

// make clean and formatted parameter string
func processRawParamString(rawParam string) (string, error) {
	// remove noise chars
	clearParam, err := noiseFiltering(rawParam)
	if err != nil {
		return "", err
	}

	// change 0x0a to 0x20
	clearParam = replace0x0A(clearParam)

	// format url
	formattedParam := formatUrl(clearParam)

	// remove blanks
	formattedParam = replaceMultipleBlankSpaces(formattedParam)

	return formattedParam, nil
}

func skipSafeParamsCheck(uriKeyValues map[string][]string, level int) map[string][]string {
	suspiciousParams := make(map[string][]string)

	for key, values := range uriKeyValues {
		// skip safe keys
		key = strings.TrimSpace(key) // remove black space
		key = strings.ToLower(key)   // change to lower case

		if matchWords(key, []string{"query", "word", "wd", "note"}) {
			// this key is safe. skip it.
			continue
		}

		if level == 2 {
			if matchWords(key, []string{"q", "w", "bk_key"}) {
				// this key is safe. skip it.
				continue
			}
			if containWords(key, []string{"kw", "query", "word", "wd",
				"refer", "title"}) {
				// this key is safe. skip it.
				continue
			}
		}
		suspiciousParams[key] = values
	}

	return suspiciousParams
}

func appendPath(uriKeyValues map[string][]string) map[string][]string {
	keyValues := uriKeyValues

	for key, values := range uriKeyValues {
		for _, value := range values {
			tmp := splitPath(value)
			if tmp != "" {
				keyValues[key] = append(keyValues[key], tmp)
			}
		}
	}

	return keyValues
}

var suspicious_words []string

func init() {
	suspicious_words = []string{
		"and",
		"or",
		"union",
		"select",
		"from",
		"concat",
		"/*",
		"*/",
		"(",
		")",
		"where",
		"substr",
		"len"}
}

func hasSuspiciousWords(uriKeyValues map[string][]string) bool {
	// suspicious words check
	if matchSuspStrings(uriKeyValues, suspicious_words) {
		return true
	}

	return false
}

// extract all values from key values pairs
func extracParameters(uriKeyValues map[string][]string) map[string]string {
	kvalues := make(map[string]string)

	for k, vs := range uriKeyValues {
		for i, v := range vs {
			// in case there is multiple query paramters
			// the keys are all the same. e.g. www.baidu.com/xx.php/id=1&id=2&id=3
			// all of these paramters should be checked
			// we make a fake key here.
			key := k + string(i)
			kvalues[key] = v
		}
	}

	return kvalues
}

// modify parameters
func reformParamters(parameters map[string]string) map[string]string {
	retParams := make(map[string]string)

	for k, v := range parameters {
		// replace "`load_file`" to " load_file "
		v = strings.Replace(v, "`load_file`", " load_file ", -1)

		bstr := []byte(v)

		// check "--"
		for i := 0; i+2 < len(bstr); i++ {
			if bstr[i] == '-' && bstr[i+1] == '-' {
				if bstr[i+2] >= 0x01 && bstr[i+2] <= 0x08 {
					bstr[i+2] = 0x20
					i += 2
					continue

				} else if bstr[i+2] >= 0x0e && bstr[i+2] <= 0x1f {
					bstr[i+2] = 0x20
					i += 2
					continue

				} else if bstr[i+2] == 0x7f {
					bstr[i+2] = 0x20
					i += 2
					continue

				} else if bstr[i+2] == 0x0a {
					//bstr = bstr[0:i+2] + byte(0x20) + bstr[i+2:]
					tmp := make([]byte, len(bstr)+1)

					copy(tmp, bstr[0:i+2])
					tmp[i+2] = 0x20
					copy(tmp[i+3:], bstr[i+2:])

					bstr = tmp
					i += 3
					continue
				}
			}
		}

		// replace 0x09, 0x0B, 0x0C, 0x0D, 0x20, 0xA0, 0x7B, 0x7D to 0x20
		for i := 0; i < len(bstr); i++ {
			if bstr[i] == 0x09 ||
				bstr[i] == 0x0B ||
				bstr[i] == 0x0C ||
				bstr[i] == 0x0D ||
				bstr[i] == 0xA0 ||
				bstr[i] == 0x7B ||
				bstr[i] == 0x7D {
				bstr[i] = 0x20
			}
		}

		retParams[k] = string(bstr)
	}

	return retParams
}

func checkValueString(value string, tag byte, fromStart bool) bool {
	lastTagPos := -1 // check from last tag
	if fromStart {
		lastTagPos = 0 // check from start of a parameter
	}

	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch != tag {
			continue
		}

		// ch == tag
		// find first tag
		if lastTagPos < 0 {
			lastTagPos = i
		} else {
			// get another tag
			part := value[lastTagPos:i]

			if strings.Contains(part, "-- ") ||
				strings.Contains(part, "/*") ||
				strings.Contains(part, "*/") ||
				strings.Contains(part, "#") {

				// hit
				return true

			} else {
				// do not has suspicous words
				lastTagPos = i
			}
		}
	}

	// do not find any suspicous words
	return false
}

// check between "`", or ''' or '"'
func checkParamters(parameters map[string]string) bool {

	for _, value := range parameters {
		if checkValueString(value, '`', false) ||
			checkValueString(value, '\'', true) ||
			checkValueString(value, '"', true) {
			// call step 10
			if !step10(value) {
				return false
			}
		}
	}

	return true
}

type compFunc func(c byte) bool

// findSqlWord : find key word
// Parameters  :
//      - value     : destination string
//      - word      : key word
//      - offset    : offset
//      - leftCmp   : determin how to match the char before 'word'
//      - rightCmp  : determin how to match the char after 'word'
//
// Returns:
//      - bool  : if find, return true;
//      - int   : the index of word in value
func findSqlWord(value string, word string, offset int, leftCmp, rightCmp compFunc) (bool, int) {
	// [0:offset] has been checked, and has not found "word"
	if offset >= len(value) || offset < 0 || word == "" {
		return false, -1
	}

	workStr := value[offset:]
	if workStr == word {
		return true, 0
	}

	pos := strings.Index(workStr, word) // "union", "select", etc.
	if pos < 0 {
		return false, -1
	}

	lenValue := len(workStr)
	lenWord := len(word) // "union", "select", etc.

	if pos == 0 {
		if !rightCmp(workStr[lenWord]) {
			return true, pos
		} else {
			// skip a-z_0-9 chars
			for pos+lenWord < lenValue && rightCmp(workStr[pos+lenWord]) {
				pos++
			}
		}
	} else {
		// pos > 0
		// check before "union"
		if !leftCmp(workStr[pos-1]) {
			// check after "union"
			if pos+lenWord < lenValue {
				if !rightCmp(workStr[pos+lenWord]) {
					return true, pos
				}
			} else if pos+lenWord == lenValue {
				// "union" is the last of this string
				return true, pos
			}
		}
	}

	find, position := findSqlWord(value, word, offset+pos+lenWord, leftCmp, rightCmp)
	if !find {
		return false, -1
	}

	// (pos+lenWord) is skipped when calling findSqlWord above.
	position += (pos + lenWord)
	return true, position
}

// find all "(\s*select"
func findAllBracketAndSelect(value string) []int {
	positions := []int{}
	offset := 0

	str := value[offset:]
	pos := strings.Index(str, "select")
	for pos >= 0 {
		for i := pos - 1; i >= 0; i-- {
			if str[i] != ' ' {
				if str[i] == '(' {
					//return i
					positions = append(positions, i+offset)
				}

				break
			}
		}

		offset += pos + len("select")
		str = value[offset:]
		pos = strings.Index(str, "select")
	}

	return positions
}

func step10(value string) bool {
	// step 10.1
	if !checkParamterSqlWords(value) {
		return false
	}

	// step 10.2
	if !checkParamterSqlWords2(value) {
		return false
	}

	// step 10.3
	if !checkParamterSqlWords3(value) {
		return false
	}

	return true
}

// step 10.1
func checkParamterSqlWords(value string) bool {

	offset := 0
	// check "union select from "
	found, pos := findSqlWord(value, "union", 0, isAZExN, isAZ09)
	if !found {
		// goto step 10.1.2
		return true
	}
	offset += (pos + len("union"))

	found, pos = findSqlWord(value, "select", offset, isAZ09, isAZ09)
	if !found {
		// goto step 10.1.2
		return true
	}
	offset += (pos + len("select"))

	// use offset from "select"
	// 10.1.1, seach for "union select from"
	found, pos = findSqlWord(value, "from", offset, isAZExN, isAZ09)
	if found {
		// not safe
		return false
	}

	// use offset from "select"
	// 10.1.2, search for "union select load_file ( )"
	found, pos = findSqlWord(value, "load_file", offset, isAZ09, isAZ09)
	if found {
		offset2 := offset + pos + len("load_file")
		//found, pos = findSqlWord(value, "(", offset2)
		pos = strings.Index(value[offset2:], "(")
		if pos >= 0 {
			offset2 += (pos + 1)
			//found, pos = findSqlWord(value, ")", offset2)
			pos = strings.Index(value[offset2:], ")")
			if pos >= 0 {
				// find "union select load_file ( )"
				return false
			}
		}
	}

	// use offset from "select"
	// 10.1.3, seach for "union select into outfile [\'|\"]"
	found, pos = findSqlWord(value, "into", offset, isAZExN, isAZ09)
	if found {
		offset2 := offset + pos + len("into")
		found, pos = findSqlWord(value, "outfile", offset2, isAZ09, isAZ09)
		if found {
			offset2 += (pos + len("outfile"))
			if strings.Contains(value[offset2:], "'") ||
				strings.Contains(value[offset2:], "\"") {
				// find "union select into outfile [\'|\"]"
				return false
			}
		}
	}

	// use offset from "select"
	// 10.1.4, seach for "union select into dumpfile [\'|\"]"
	found, pos = findSqlWord(value, "into", offset, isAZExN, isAZ09)
	if found {
		offset2 := offset + pos + len("into")
		found, pos = findSqlWord(value, "dumpfile", offset2, isAZ09, isAZ09)
		if found {
			offset2 += (pos + len("dumpfile"))
			if strings.Contains(value[offset2:], "'") ||
				strings.Contains(value[offset2:], "\"") {
				// find "union select into dumpfile [\'|\"]"
				return false
			}
		}
	}

	return true
}

var words_in_step10_2 []string

func init() {
	words_in_step10_2 = []string{
		"any",
		"some",
		"all",
		"in",
		"limit",
		"like",
		"exists",
	}
}

// step 10.2
func checkParamterSqlWords2(value string) bool {
	offset := 0
	hit := false

	// special words

	// check words
	for i := 0; i < len(words_in_step10_2); i++ {
		found, _ := findSqlWord(value, words_in_step10_2[i], offset, isAZ, isAZ09)
		if found {
			hit = true
			break
		}
	}

	if !hit {
		// check tokes
		if containWords(value, []string{">", "<", "="}) {
			hit = true
		}
	}

	if !hit {
		// goto step 10.3
		return true
	}

	// search for "select from"
	found, pos := findSqlWord(value, "select", offset, isAZ09, isAZ09)
	if found {
		offset += pos + len("select")
		found, _ = findSqlWord(value, "from", offset, isAZExN, isAZ09)
		if found {
			// find "select from"
			return false
		}
	}

	return true
}

// step 10.3
func checkParamterSqlWords3(value string) bool {
	offset := 0

	if pos := strings.Index(value, "("); pos >= 0 {
		offset += pos + 1

		found, pos := findSqlWord(value, "select", offset, isAZ09, isAZ09)
		if !found {
			return true
		}
		offset = pos + len("select")

		pos = strings.Index(value[offset:], "(")
		if pos < 0 {
			return true
		}
		offset += (pos + 1)

		pos = strings.Index(value[offset:], ")")
		if pos < 0 {
			return true
		}
		offset += (pos + 1)

		found, pos = findSqlWord(value, "from", offset, isAZExN, isAZ09)
		if !found {
			return true
		}
		offset += (pos + len("from"))

		if strings.Index(value[offset:], ")") > 0 {
			// find "(select  () from  )", not safe
			return false
		}
	}

	return true
}

type SqlSyntaxTree struct {
	StartPoint         int
	SelectIndex        int
	FromIndexs         []int
	LoadFileIndex      []int
	IntoOutFileIndexs  []int
	IntoDumpFileIndexs []int
}

func getAllWords(value string, word string, offset int, leftCmp, rightCmp compFunc) []int {
	indexs := []int{}

	// get all "word"
	for found, pos := findSqlWord(value, word, offset, leftCmp, rightCmp); found; {
		// get a "word"
		indexs = append(indexs, offset+pos)

		offset += (pos + len(word))
		found, pos = findSqlWord(value, word, offset, leftCmp, rightCmp)
	}

	return indexs
}

func makeSqlSyntaxTrees(value string, startPoints []int) []SqlSyntaxTree {
	if len(startPoints) == 0 {
		// no "union"
		return nil
	}

	syntaxTrees := []SqlSyntaxTree{}

	for i := 0; i < len(startPoints); i++ {
		offset := startPoints[i]
		selectIndex := -1
		fromIndexs := []int{}
		loadFileIndexs := []int{}
		intoOutFileIndexs := []int{}
		intoDumpFileIndexs := []int{}

		// find "select"
		found, pos := findSqlWord(value, "select", offset, isAZ09, isAZ09)
		if !found {
			continue
		}

		if i < len(startPoints)-1 {
			if offset+pos > startPoints[i+1] {
				// this "select" belows to next "union"
				continue
			}
		}

		selectIndex = offset + pos
		offset += (pos + len("select"))

		// get all "from"
		fromIndexs = getAllWords(value, "from", offset, isAZExN, isAZ09)

		// get all "load_file"
		loadFileIndexs = getAllWords(value, "load_file", offset, isAZ09, isAZ09)

		// get all "into outfile"
		intoOutFileIndexs = getAllWords(value, "into outfile", offset, isAZExN, isAZ09)

		// get all "into dumpfile"
		intoDumpFileIndexs = getAllWords(value, "into dumpfile", offset, isAZExN, isAZ09)

		syntaxTrees = append(syntaxTrees,
			SqlSyntaxTree{
				startPoints[i],
				selectIndex,
				fromIndexs,
				loadFileIndexs,
				intoOutFileIndexs,
				intoDumpFileIndexs,
			})
	}

	return syntaxTrees
}

func simpleSqlSyntaxTrees(value string, startPoints []int) []SqlSyntaxTree {
	if len(startPoints) == 0 {
		// no word
		return nil
	}

	syntaxTrees := []SqlSyntaxTree{}

	for i := 0; i < len(startPoints); i++ {
		offset := startPoints[i]
		selectIndex := -1
		fromIndexs := []int{}
		loadFileIndexs := []int{}
		intoOutFileIndexs := []int{}
		intoDumpFileIndexs := []int{}

		// find "select"
		found, pos := findSqlWord(value, "select", offset, isAZ09, isAZ09)
		if !found {
			continue
		}

		if i < len(startPoints)-1 {
			if offset+pos > startPoints[i+1] {
				// this "select" belows to next "union"
				continue
			}
		}

		selectIndex = offset + pos
		offset += (pos + len("select"))

		// get all "from"
		fromIndexs = getAllWords(value, "from", offset, isAZExN, isAZ09)

		syntaxTrees = append(syntaxTrees,
			SqlSyntaxTree{
				startPoints[i],
				selectIndex,
				fromIndexs,
				loadFileIndexs,
				intoOutFileIndexs,
				intoDumpFileIndexs,
			})
	}

	return syntaxTrees
}

// check between "union" and "select"
// return true if need continue check in syntax tree
func checkBetweenUnionSelect(checkStr string) bool {
	// ' ' , '(' , and "all" is allowed
	for i := 0; i < len(checkStr); i++ {
		if checkStr[i] == ' ' {
			continue
		}

		if checkStr[i] == '(' {
			continue
		}

		if checkStr[i] == 'a' {
			if i+2 < len(checkStr) {
				if checkStr[i+1] == 'l' && checkStr[i+2] == 'l' {
					i += 2
					continue
				}
			}
		}

		if checkStr[i] == 'd' {
			if i+7 < len(checkStr) {
				if checkStr[i+1:i+8] == "istinct" {
					i += 7
					continue
				}
			}
		}

		return false
	}

	return true
}

func checkBetweenLeftBracketAndSelect(checkStr string) bool {
	// only ' ' is allowed
	for i := 0; i < len(checkStr); i++ {
		if checkStr[i] == ' ' {
			continue
		}

		return false
	}

	return true
}

func checkBetweenSelectAndFrom(checkStr string) bool {
	leftBracket := -1
	rightBracket := -1
	hasWord := false

	// only ' ' is allowed
	for i := 0; i < len(checkStr); i++ {
		if checkStr[i] == '(' {
			leftBracket = i
		} else if checkStr[i] == ')' {
			rightBracket = i
		}
	}

	if leftBracket < 0 && rightBracket < 0 {
		// not match
		return false
	}

	for i := 0; i < leftBracket+1; i++ {
		if (checkStr[i] >= 'a' && checkStr[i] <= 'z') || checkStr[i] == '_' {
			hasWord = true
			break
		}
	}

	if !hasWord {
		// not match
		return false
	}

	hasWord = false
	// check between "(" and ")"
	for i := leftBracket + 1; i < rightBracket+1; i++ {
		if (checkStr[i] >= 'a' && checkStr[i] <= 'z') || checkStr[i] == '_' {
			hasWord = true
			break
		}
	}

	if !hasWord {
		// not match
		return false
	}

	return true
}

// return true if match
func checkBetweenFromAndWord(value string, start int, end int) bool {
	foundInvalidChar := false

	for i := start; i < end; i++ {
		ch := value[i]
		if ch == ' ' || ch == '(' || ch == '`' || ch == '.' {
			// valid char
			continue
		}

		if !isAZ09(ch) {
			foundInvalidChar = true
			continue
		}

		// meet "word"
		if isAZ09(ch) {
			if foundInvalidChar {
				// invalid
				return false
			}

			return true
		}
	}

	// can't find word
	return false
}

// return true if match
func checkAfterLoadFile(value string, start int, end int) bool {
	foundInvalidChar := false

	for i := start; i < end; i++ {
		if value[i] == ' ' {
			// valid char
			continue
		}

		if value[i] != '(' {
			foundInvalidChar = true
		} else {
			if foundInvalidChar {
				return false
			}
			return true
		}
	}

	// can't find word
	return false
}

// return true if match
func checkIntoOutFile(value string, start int, end int) bool {
	foundInvalidChar := false

	for i := start; i < end; i++ {
		if value[i] == ' ' {
			// valid char
			continue
		}

		if value[i] != '\'' && value[i] != '"' {
			foundInvalidChar = true
		} else {
			if foundInvalidChar {
				return false
			}

			return true
		}
	}

	// can't find word
	return false
}

// return true if match
func checkIntoDumpFile(value string, start int, end int) bool {
	foundInvalidChar := false

	for i := start; i < end; i++ {
		if value[i] == ' ' {
			// valid char
			continue
		}

		if value[i] != '\'' && value[i] != '"' {
			foundInvalidChar = true
		} else {
			if foundInvalidChar {
				return false
			}
			return true
		}
	}

	// can't find word
	return false
}

type checkFunc func(string, int, int) bool

// return true if match
func checkLeafInSyntaxTree(word string, value string,
	indexs []int, check checkFunc) bool {
	for i := 0; i < len(indexs); i++ {
		start := indexs[i] + len(word)
		end := len(value)

		if i < len(indexs)-1 {
			// if this is not the last
			end = indexs[i+1]
		}

		if check(value, start, end) {
			return true
		}
	}

	return false
}

// match regexps
// match means not safe
func match_sql_regexps(value string) bool {
	pos := getAllWords(value, "union", 0, isAZExN, isAZ09)

	if len(pos) > 0 {
		trees := makeSqlSyntaxTrees(value, pos)
		for _, tree := range trees {
			if checkSqlSyntaxTree(value, tree) {
				return true
			}
		}
	}

	if sql_match_step_4_5(value) {
		return true
	}

	return false
}

// return true if match
func checkSqlSyntaxTree(value string, tree SqlSyntaxTree) bool {
	// check "union" and "select"
	if !checkBetweenUnionSelect(
		value[tree.StartPoint+len("union") : tree.SelectIndex]) {
		// not match "union" and "select"
		// try other regexp
		return false
	}

	// check "from"
	if checkLeafInSyntaxTree("from", value, tree.FromIndexs,
		checkBetweenFromAndWord) {
		return true
	}

	// check "load_file"
	if checkLeafInSyntaxTree("load_file", value, tree.LoadFileIndex,
		checkAfterLoadFile) {
		return true
	}

	// check "into outfile"
	if checkLeafInSyntaxTree("into outfile", value, tree.IntoOutFileIndexs,
		checkIntoOutFile) {
		return true
	}

	// check "into dumpfile"
	if checkLeafInSyntaxTree("into dumpfile", value, tree.IntoDumpFileIndexs,
		checkIntoDumpFile) {
		return true
	}

	return false
}

// return true if match
func checkSimpleSyntaxTree(value string, tree SqlSyntaxTree) bool {
	// check "(" and "select"
	if tree.StartPoint+len("(") < tree.SelectIndex {
		section := value[tree.StartPoint+len("(") : tree.SelectIndex]
		if !checkBetweenLeftBracketAndSelect(section) {
			return false
		}
	}

	// check "from"
	if checkLeafInSyntaxTree("from", value, tree.FromIndexs,
		checkBetweenFromAndWord) {
		return true
	}

	return false
}

func checkBetweenSelectAndFroms(value string,
	selectIndex int, fromIndexs []int) bool {

	offset := selectIndex + len("select")
	for i := 0; i < len(fromIndexs); i++ {
		start := offset
		end := fromIndexs[i]
		section := value[start:end]

		if !checkBetweenSelectAndFrom(section) {
			// failed
			return false
		}

		offset = (end + len("from"))
	}

	return true
}

// return true if match(not safe)
func checkSimpleSyntaxTree2(value string, tree SqlSyntaxTree) bool {
	// check "(" and "select"
	if tree.StartPoint+len("(") < tree.SelectIndex {
		if !checkBetweenLeftBracketAndSelect(
			value[tree.StartPoint+len("(") : tree.SelectIndex]) {
			// safe
			return false
		}
	}

	if len(tree.FromIndexs) == 0 {
		return false
	}
	// check between "select" and "from"
	if !checkBetweenSelectAndFroms(value,
		tree.SelectIndex, tree.FromIndexs) {
		return false
	}

	// check "from"
	if !checkLeafInSyntaxTree("from", value, tree.FromIndexs,
		checkBetweenFromAndWord) {
		return false
	}

	return true
}

var words_in_regexp_step4 []string

func init() {
	words_in_regexp_step4 = []string{"in", "limit", "exists",
		"like", "any", "some", "all"}
}

// return true if match
func sql_match_step_4_5(value string) bool {
	needCheck := false

	// step 4
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch == '<' || ch == '=' || ch == '>' {
			needCheck = true
			break
		}
	}

	if !needCheck {
		//words := []string{"in", "limit", "exists",
		//	"like", "any", "some", "all"}
		for _, w := range words_in_regexp_step4 {
			found, _ := findSqlWord(value, w, 0, isAZ, isAZ09)
			if found {
				needCheck = true
				break
			}
		}
	}

	// find all position "( select"
	pos := findAllBracketAndSelect(value)

	// find '( select'
	if len(pos) > 0 {
		trees := simpleSqlSyntaxTrees(value, pos)

		for _, tree := range trees {
			// step 4
			if needCheck {
				if checkSimpleSyntaxTree(value, tree) {
					// match ( select
					return true
				}
			}

			// step 5
			if checkSimpleSyntaxTree2(value, tree) {
				// match ( select2
				return true
			}
		}
	}

	// not match
	return false
}

func checkValues(values url.Values, level int) bool {
	// skip safe keys
	uriKeyValues := skipSafeParamsCheck(values, level)
	// convert to lower case valuse
	uriKeyValues = convertToLower(uriKeyValues)

	// add path
	uriKeyValues = appendPath(uriKeyValues)

	// check suspicous words
	if !hasSuspiciousWords(uriKeyValues) {
		// not hasSuspiciousWords, safe
		return false
	}

	// extract parameters
	parameters := extracParameters(uriKeyValues)

	// step 5,6,7
	parameters = reformParamters(parameters)

	// step 8, 9, 10
	if !checkParamters(parameters) {
		return true
	}

	// check for each parameter
	for _, paramValue := range parameters {
		// make clear parameters
		formattedParamValue, err1 := processRawParamString(paramValue)
		if err1 != nil {
			return false
		}

		// POST is not considered
		if match_sql_regexps(formattedParamValue) {
			return true
		}
	}

	return false
}

type RuleSqlInjection struct {
}

func NewRuleSqlInjection() *RuleSqlInjection {
	rule := new(RuleSqlInjection)
	return rule
}

func (rule *RuleSqlInjection) Init() error {
	return nil
}

// hit the rule, return true(not safe); else return false(safe)
func (rule *RuleSqlInjection) Check(pReq *RuleRequestInfo) bool {
	if pReq.UriParsed == nil {
		return false
	}

	level := getLevel(pReq, RuleSQLInjection)

	// check query values
	return checkValues(pReq.QueryValues, level)
}

// isAZExN - check whether c is a-z_ but except char N
//
// Params:
//      - ch    : char
//
// Return:
//      - bool  : in set, return true; else trurn false
func isAZExN(ch byte) bool {
	if isAZ(ch) && ch != 'n' {
		return true
	}

	return false
}
