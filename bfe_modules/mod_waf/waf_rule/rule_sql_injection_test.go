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

/*
DESCRIPTION
    Test cases for rule_func_sql_injection.go

*/
package waf_rule

import "testing"
import "strings"
import "fmt"

//matchSqlStrings
func TestMatchSqlStrings(t *testing.T) {
	// for reg regexp
	matchStrings := []string{
		"and",
		"or",
		"union",
		"select",
		"from",
		"concat",
		"where",
		"substr",
		"len",
		"length"}

	rule := NewRuleSqlInjection()
	rule.Init()

	keyValues := make(map[string][]string)

	keyValues["1"] = append(keyValues["1"], "111")
	keyValues["1"] = append(keyValues["1"], "union")

	ret := matchSuspStrings(keyValues, matchStrings)
	if ret == true {
		t.Error("TestMatchSqlStrings", "case 1", ret)
	}

	keyValues["2"] = append(keyValues["1"], "substr")
	keyValues["2"] = append(keyValues["1"], "union")

	ret = matchSuspStrings(keyValues, matchStrings)
	if ret == false {
		t.Error("TestMatchSqlStrings", "case 2", ret)
	}
}

//matchBlankToken
func TestMatchBlankToken(t *testing.T) {
	var token byte
	token = 0x00
	ret := matchBlankToken(token)
	if ret == true {
		t.Error("TestMatchBlankToken", "case 1", token, ret)
	}

	token = 0x0c
	ret = matchBlankToken(token)
	if ret == false {
		t.Error("TestMatchBlankToken", "case 2", token, ret)
	}
}

//replaceBlankSpace
func TestReplaceBlankSpace(t *testing.T) {
	url := "http://sss.baidu.com/ddds/pp/qq/a.php?ssee"
	ret := replaceBlankSpace(url)
	if ret != "http://sss.baidu.com/ddds/pp/qq/a.php?ssee" {
		t.Errorf("TestReplaceBlankSpace\n  case 1\n  %s\n  %s\n", url, ret)
	}

	tmp := []byte{0x09, 0x0B, 0x0C, 0x0D, 0x20, 0xA0}
	url = string(tmp)
	ret = replaceBlankSpace(url)
	if ret != "      " {
		t.Errorf("TestReplaceBlankSpace\n  case 2\n  %s\n  %s\n", url, ret)
	}
}

//removeBetweenBytes
func TestRemoveBetweenBytes(t *testing.T) {
	str := "aabbccabcccccc#ssii#pp#p0x1133s"
	ret := removeBetweenBytes(str, '#', 10)
	if ret != "aabbccabcccccc " {
		t.Errorf("TestRemoveBetweenBytes\n  case 1\n  %s\n  %s\n", str, ret)
	}

	str = "aabbccabcccccc#ssiippp0x1133s"
	ret = removeBetweenBytes(str, '#', '0')
	if ret != "aabbccabcccccc x1133s" {
		t.Errorf("TestRemoveBetweenBytes\n  case 2\n  %s\n  %s\n", str, ret)
	}
}

//removeBetweenStrByte
func TestRemoveBetweenStrByte(t *testing.T) {
	str := "aabbccabcccccc#ssiippp0x1133s"
	ret := removeBetweenStrByte(str, "cc", 10)
	if ret != "aabb" {
		t.Errorf("TestRemoveBetweenStrByte\n  case 1\n  %s\n  %s\n", str, ret)
	}

	str = "aabbccabcccccc#ssiippp0x1133s"
	ret = removeBetweenStrByte(str, "cc", '0')
	if ret != "aabb x1133s" {
		t.Errorf("TestRemoveBetweenStrByte\n  case 2\n  %s\n  %s\n", str, ret)
	}

	str = "aabbccabcccccc#ssiippp0x1133s"
	ret = removeBetweenStrByte(str, "xx", '0')
	if ret != "aabbccabcccccc#ssiippp0x1133s" {
		t.Errorf("TestRemoveBetweenStrByte\n  case 3\n  %s\n  %s\n", str, ret)
	}
}

//processNormalComment
func TestProcessNormalComment(t *testing.T) {
	str := "/*aabbccabcccccc#ssiippp0x1133s */"
	ret, size := processNormalComment(str)
	if ret != "" {
		t.Errorf("TestProcessNormalComment\n  case 1\n  %s\n  %s\n", str, ret)
	}
	if size != len(str) {
		t.Errorf("TestProcessNormalComment\n  case 1 size:\n  %d\n  %d\n", len(str), size)
	}

	str = "/*aabbccabcccccc#ssiippp0x1133s"
	ret, size = processNormalComment(str)
	if ret != "" {
		t.Errorf("TestProcessNormalComment\n  case 2\n  %s\n  %s\n", str, ret)
	}
	if size != len(str) {
		t.Errorf("TestProcessNormalComment\n  case 2 size:\n  %d\n  %d\n", 2, size)
	}

	str = "/*ccc#ssiippp*/0x1133s"
	ret, size = processNormalComment(str)
	if ret != "0x1133s" {
		t.Errorf("TestProcessNormalComment\n  case 4\n  %s\n  %s\n", str, ret)
	}
	if size != len("/*ccc#ssiippp*/") {
		t.Errorf("TestProcessNormalComment\n  case 4 size:\n  %d\n  %d\n", len("/*ccc#ssiippp*/"), size)
	}

	str = "/*ccc#ssiippp*/0x11/**/33s"
	ret, size = processNormalComment(str)
	if ret != "0x11/**/33s" {
		t.Errorf("TestProcessNormalComment\n  case 5\n  %s\n  %s\n", str, ret)
	}
	if size != len("/*ccc#ssiippp*/") {
		t.Errorf("TestProcessNormalComment\n  case 5 size:\n  %d\n  %d\n", len("/*ccc#ssiippp*/"), size)
	}

	str = "/*/**/"
	ret, size = processNormalComment(str)
	if ret != "" {
		t.Errorf("TestProcessNormalComment\n  case 6\n  %s\n  %s\n", str, ret)
	}
	if size != len("/*/**/") {
		t.Errorf("TestProcessNormalComment\n  case 6 size:\n  %d\n  %d\n", len("/*/**/"), size)
	}
}

//formatUrl
func TestFormatUrl(t *testing.T) {
	str := "aaaa(bbbb)cccc\"dsdsd\"pppp'k'ppp/"
	ret := formatUrl(str)
	if ret != "aaaa (bbbb)cccc \"dsdsd \"pppp 'k 'ppp /" {
		t.Errorf("TestFormatUrl\n  case 1\n  %s\n  %s\n", str, ret)
	}

	str = "aaaa(bbbb)c   ccc\"dsd   sd\"pp   pp'k' /"
	ret = formatUrl(str)
	if ret != "aaaa (bbbb)c   ccc \"dsd   sd \"pp   pp 'k '  /" {
		t.Errorf("TestFormatUrl\n  case 2\n  %s\n  %s\n", str, ret)
	}
}

//checkTokens
func TestCheckTokens(t *testing.T) {
	str := []string{
		"aaaa(bbbb) cccc\" dsdsd\" pppp ' k 'ppp  /",
		"(aaaa bbbb",
		"123 bbb",
		"aaa bb=b",
		"aaa bbb cc\"c",
		"aaa bbb ccc"}
	results := []bool{
		true,
		true,
		true,
		true,
		true,
		false}

	for i, s := range str {
		tokens := strings.Split(s, " ")
		ret := checkTokens(tokens)
		if ret != results[i] {
			t.Errorf("TestCheckTokens\n  case %d\n  %s\n  %v\n", i, s, ret)
		}
	}
}

//processSpecialComment
func TestProcessSpecialComment(t *testing.T) {
	comment := []string{
		"/*!12345....*/",
		"/*!2345x....*/",
		"/*!2s453....*/",
		"/*!123 ....*/",
		"/*!x123 ....*/",
		"/*!12345../*1*/..*/",
		"/*!2345x../*1*/..*/",
		"/*!2345x../*!1*/..*/",
		"/*!2345x../*!12345xx*/..*/",
		"/*!2345x../*!12345*/..*/",
		"/*!2345x../*!12345*/./*normal*/12345/*!55555dy*/.*/",
		"/*!12345and 2 /*ss*/ /*! d /*!1111156 /* case */ */ */ */",
		"/*!12345and 2 /*ss*/ /*! d /*!1111156 /* case */ */ */"}

	results := []string{
		" .... ",
		" 2345x.... ",
		" 2s453.... ",
		" 123 .... ",
		" x123 .... ",
		" .. .. ",
		" 2345x.. .. ",
		" 2345x.. 1 .. ",
		" 2345x.. xx .. ",
		" 2345x..  .. ",
		" 2345x..  . 12345. ",
		" and 2     d  56        ",
		" and 2     d  56      "}

	for i, s := range comment {
		ret, _ := processSpecialComment(s)
		if ret != results[i] {
			t.Errorf("TestProcessSpecialComment\n  case %d\n  '%s'\n  '%s'\n  '%s'\n", i, s, ret, results[i])
		}
	}
}

//noiseFiltering
func __TestNoiseFiltering(t *testing.T) {
	str := []string{
		"/*!and 2/**/*/",
		"a/*and 2  */b",
		"a/*!12345and 2 /*ss*/ /*! d /*!1111156 /* case */ */ */ */b",
		"/*!and 2/**/*/",
		"/**/  /*2*/ /*55*/",
		"1/**/1/**/1/**//**/",
		"/*!abc/*1*/cc/**/c*/",
		"/*!#asdfsd*/#aa",
		"/*aaaaaaaaa"}

	results := []string{
		" and 2 ",
		"a b",
		"a and 2 d 56 b",
		" and 2 ",
		" ",
		"1 1 1 ",
		" abc cc c",
		" ",
		" "}

	for i, s := range str {
		ret, _ := noiseFiltering(s)
		if ret != results[i] {
			t.Errorf("TestNoiseFiltering\n  case %d\n  %s\n  %s\n  %s\n", i, s, ret, results[i])
		}
	}
}

//test replaceMultipleBlankSpaces
func TestReplaceMultipleBlankSpaces(t *testing.T) {
	str := []string{
		"fds  ss 1  + 1    + 1  ",
		"       ",
		"",
		" 中国  北京 haidian ",
		" aasf ds fda   sadf",
		"   aasf   ds   fda   sadf   "}

	results := []string{
		"fds ss 1 + 1 + 1",
		"",
		"",
		"中国 北京 haidian",
		"aasf ds fda sadf",
		"aasf ds fda sadf"}

	for i, s := range str {
		ret := replaceMultipleBlankSpaces(s)
		if ret != results[i] {
			t.Errorf("TestReplaceMultipleBlankSpaces\n  case %d\n  %s\n  %s\n  %s\n", i, s, ret, results[i])
		}
	}
}

//test removeBlankSpaceForOperators
func TestRemoveBlankSpaceForOperators(t *testing.T) {
	s := " 1 + 1 + 1 "
	result := " 1+1+1 "
	ret := removeBlankSpaceForOperators(s)

	if ret != result {
		t.Errorf("TestRemoveBlankSpaceForOperators\n  case 1\n  %s\n  %s\n  %s\n", s, ret, result)
	}
}

// test reformParamters
func TestReformParamters(t *testing.T) {
	testData := make(map[string]string)

	testData["key1"] = "`load_file`"

	testData["key2"] = string([]byte{'-', '-', 0x01,
		'-', '-', 0x08, '-', '-', 0x1f, '-', '-', 'a'})

	testData["key3"] = string([]byte{0x09, 0x0B, 0x0C, 0x0D, 0x20, 0xA0})

	testData["key4"] = string([]byte{'-', '-', 0x0a, 'a'})

	testData["key5"] = string([]byte{'b', '-', '-', 0x04, 0x0a, 'a'})

	testData = reformParamters(testData)

	if testData["key1"] != " load_file " {
		t.Errorf("TestReformParamters\n  case 1\n  '%s' != ' load_file '\n", testData["key1"])
	}

	if testData["key2"] != "-- -- -- --a" {
		t.Errorf("TestReformParamters\n  case 2\n  '%s' != '-- -- -- --a'\n", testData["key2"])
	}

	if testData["key3"] != "      " {
		t.Errorf("TestReformParamters\n  case 3\n  '%s' != '      '\n", testData["key3"])
	}

	target := string([]byte{'-', '-', 0x20, 0x0a, 'a'})
	if testData["key4"] != target {
		t.Errorf("TestReformParamters\n  case 4\n  '%s' != '%s'\n", testData["key4"], target)
	}

	target = string([]byte{'b', '-', '-', 0x20, 0x0a, 'a'})
	if testData["key5"] != target {
		t.Errorf("TestReformParamters\n  case 5\n  '%s' != '%s'\n", testData["key5"], target)
	}

	//fmt.Printf("%v\n", testData)
}

func TestCheckValueString(t *testing.T) {
	type testCase struct {
		Value string
		Ret   bool
	}

	cases := []testCase{
		{"aas`", false},
		{"aas`a`", false},
		{"aas`a-- b`", true},
		{"aas`a/*b`", true},
		{"aas`a*/b`", true},
		{"aas`a#b`", true},
		{"aas`ab`xx``dd`#`", true},
	}

	for i := 0; i < len(cases); i++ {
		if checkValueString(cases[i].Value, '`', false) != cases[i].Ret {
			t.Errorf("TestCheckValueString  case %d\n case : %s", i+1, cases[i].Value)
		}
	}

	cases = []testCase{
		{"vv''", false},
		{"vv-- '", true},
		{"vv'--'", false},
		{"vv'-- '", true},
		{"vv'-/* '", true},
		{"vv'-*/ '", true},
		{"vv'-*# '", true},
		{"vv'-* 'dd's'", false},
		{"vv'-* 'dd's#'", true},
		{"vv#'-* 'dd's'", true},
	}
	for i := 0; i < len(cases); i++ {
		if checkValueString(cases[i].Value, '\'', true) != cases[i].Ret {
			t.Errorf("TestCheckValueString  case %d\n case : %s", i+1, cases[i].Value)
		}
	}
}

func TestFindSqlWord(t *testing.T) {
	type testCase struct {
		Value string
		Ret   bool
		Pos   int
	}
	cases := []testCase{
		{"vvaa", false, -1},
		{"vvunion", false, -1},
		{"union1", false, -1},
		{"uniona", false, -1},
		{"_union", false, -1},
		{"_ union ", true, 2},
		{"_ union", true, 2},
		{"uniona  union", true, 8},
		{"uniona  1union", true, 9},
		{"uniona  1union_ union ", true, 16},
		{"unionunion union", true, 11},
	}

	for i := 0; i < len(cases); i++ {
		ret, pos := findSqlWord(cases[i].Value, "union", 0, isAZ, isAZ09)
		if ret != cases[i].Ret || pos != cases[i].Pos {
			t.Errorf("TestFindSqlWord  case %d\n case : %s, %v, %d, ret: %v, %d",
				i+1, cases[i].Value, cases[i].Ret, cases[i].Pos, ret, pos)
		}
	}

	cases = []testCase{
		{"nunion", true, 1},
	}

	for i := 0; i < len(cases); i++ {
		ret, pos := findSqlWord(cases[i].Value, "union", 0, isAZExN, isAZ09)
		if ret != cases[i].Ret || pos != cases[i].Pos {
			t.Errorf("TestFindSqlWord case %d\n case : %s, %v, %d, ret: %v, %d",
				i+1, cases[i].Value, cases[i].Ret, cases[i].Pos, ret, pos)
		}
	}

	cases = []testCase{
		{"abcunion", true, 3},
	}

	for i := 0; i < len(cases); i++ {
		ret, pos := findSqlWord(cases[i].Value, "union", 0, alwaysFalse, isAZ09)
		if ret != cases[i].Ret || pos != cases[i].Pos {
			t.Errorf("TestFindSqlWord case %d\n case : %s, %v, %d, ret: %v, %d",
				i+1, cases[i].Value, cases[i].Ret, cases[i].Pos, ret, pos)
		}
	}

	cases = []testCase{
		{"vvaa", false, -1},
		{"vvselect", false, -1},
		{"select1", false, -1},
		{"selecta", false, -1},
		{"_select", false, -1},
		{"_ select ", true, 2},
		{"_ select", true, 2},
		{"selecta  select", true, 9},
		{"selecta  1select", true, 10},
		{"selecta  1select_ select ", true, 18},
	}

	for i := 0; i < len(cases); i++ {
		ret, pos := findSqlWord(cases[i].Value, "select", 0, isAZ, isAZ09)
		if ret != cases[i].Ret || pos != cases[i].Pos {
			t.Errorf("TestFindSqlWord  case %d\n case : %s, %v, %d, ret: %v, %d",
				i+1, cases[i].Value, cases[i].Ret, cases[i].Pos, ret, pos)
		}
	}
}

func compareArray(a []int, b []int) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

/*
func __testFindAllWords(t *testing.T) {
	type testCase struct {
		Value string
		Word  string
		Pos   []int
	}
	cases := []testCase{
		{"vvaa", "union", []int{}},
		{"vvunion", "union", []int{}},
		{"uniona  union", "union", []int{8}},
		{"union  union xx union", "union", []int{0, 7, 16}},
		{"( ( ( ) ", "(", []int{0, 2, 4}},
	}

	for i := 0; i < len(cases); i++ {
		pos := findAllWords(cases[i].Value, cases[i].Word)
		if compareArray(pos, cases[i].Pos) == false {
			t.Errorf("TestFindAllWords case %d\n case : %s, %v, ret: %d",
				i+1, cases[i].Value, cases[i].Pos, pos)
		}
	}
}
*/

// testing for checkParamterSqlWords
func TestCheckParamterSqlWords(t *testing.T) {
	type testCase struct {
		Value string
		Ret   bool
	}

	cases := []testCase{
		{"vv''", true},
		{"v union", true},
		{" union select load_file ( ) ", false},
		{"union select from", false},
		{"union select into outfile '", false},
		{" union select into dumpfile \"", false},
		{" union select load_file )( ) ", false},
		{" union select load_file )( ", true},
		{"unionselectintooutfile'", true},
	}

	for i := 0; i < len(cases); i++ {
		if checkParamterSqlWords(cases[i].Value) != cases[i].Ret {
			t.Errorf("TestCheckParamterSqlWords  case %d\n case : %s",
				i+1, cases[i].Value)
		}
	}
}

// testing for checkParamterSqlWords2
func TestCheckParamterSqlWords2(t *testing.T) {
	type testCase struct {
		Value string
		Ret   bool
	}

	cases := []testCase{
		{"vv''", true},
		{"v select  from  ", true},
		{" select  from  > ", false},
		{"11 select from some ", false},
		{"22 select all from ", false},
		{"22 limit select from ", false},
		{"22 = select from ", false},
	}

	for i := 0; i < len(cases); i++ {
		if checkParamterSqlWords2(cases[i].Value) != cases[i].Ret {
			t.Errorf("TestCheckParamterSqlWords2  case %d\n case : %s",
				i+1, cases[i].Value)
		}
	}
}

// testing for checkParamterSqlWords3
func TestCheckParamterSqlWords3(t *testing.T) {
	type testCase struct {
		Value string
		Ret   bool
	}

	cases := []testCase{
		{"vv''", true},
		{"( select  )( from  )  ", true},
		{" ( select  () from  ) ", false},
		{"() select  () from  ) ", false},
	}

	for i := 0; i < len(cases); i++ {
		if checkParamterSqlWords3(cases[i].Value) != cases[i].Ret {
			t.Errorf("TestCheckParamterSqlWords3  case %d\n case : %s",
				i+1, cases[i].Value)
		}
	}
}

// testing for getAllWords
func TestGetAllWords(t *testing.T) {
	type testCase struct {
		Value string
		Pos   []int
	}

	cases := []testCase{
		{"vv''", []int{}},
		{"((((", []int{0, 1, 2, 3}},
		{"( select  )( from  )  ", []int{0, 11}},
		{" ( select  () (", []int{1, 11, 14}},
	}

	for i := 0; i < len(cases); i++ {
		positions := getAllWords(cases[i].Value, "(", 0, isAZ09, isAZ09)

		if compareArray(positions, cases[i].Pos) == false {
			t.Errorf("TestFindAllWordsSimple  case %d\n ret : %v",
				i+1, positions)
		}
	}

	cases = []testCase{
		{"vv''", []int{}},
		{"nunion  ", []int{1}},
		{"union  dds union", []int{0, 11}},
		{"union  dds unions union", []int{0, 18}},
	}

	for i := 0; i < len(cases); i++ {
		ret := getAllWords(cases[i].Value, "union", 0, isAZExN, isAZ09)

		if len(ret) == len(cases[i].Pos) {
			for j := 0; j < len(ret); j++ {
				if ret[j] != cases[i].Pos[j] {
					t.Errorf("TestFindStartPoints  case %d\n case : %s",
						i+1, cases[i].Value)
				}
			}
		} else {
			t.Errorf("TestFindStartPoints  case %d\n case : %s",
				i+1, cases[i].Value)
		}
	}
}

// testing for makeSqlSyntaxTrees
func TestMakeSqlSyntaxTrees(t *testing.T) {
	type testCase struct {
		Value       string
		UnionIndexs []int
	}

	cases := []testCase{
		{"union  select from ", []int{0}},
		{"union  dds union select 1a(z) from 2a(z) from", []int{0, 11}},
		{"union  dds unions union", []int{0, 18}},
	}

	for i := 0; i < len(cases); i++ {
		trees := makeSqlSyntaxTrees(cases[i].Value, cases[i].UnionIndexs)
		for _, tree := range trees {
			fmt.Printf("%v\n", tree)
			checkBetweenSelectAndFroms(cases[i].Value, tree.SelectIndex, tree.FromIndexs)
		}

	}
}

//testing for blank replace and noise filting
func __TestBlankReplaceAndNoiseFilting(t *testing.T) {
	testStrings := []string{
		"id=1/*!and%202/**/*/",
		"id=1/*!and%202/**/-1=1*/",
		"id=3/*!or%201/*!%2b0*/",
		"id=3/*!or%200/*!%2b/*!1*/",
		"id=3/*!or%200/*!%2b/*!111111*/",
		"id=1234%20union/**/select%201,2,load_file/**/(0x2F6574632F706173737764),/**/4",
		"id=1234%20union/*!select*/%20/*!1,2,load_file/**/(0x2F6574632F706173737764),/**/4*/",
		"id=1234%20union/*!select*/%20/*!1,2,load_file*//**/(0x2F6574632F706173737764),/**/4",
		"id=1234%20/*!union/*!00000*//*!11112select*/%20/*!1,2,/*!load_file*//**/(/*!0x2F6574632F706173737764*/),/**/4",
		"id=1234%20--%20selec/**/t%201,12,3,4%0aunion--%20asdfas%0aselect(1),2,load_file/**/(/*!0x2F6574632F706173737764*/),/**/4",
		"id=1234%20%23%20selec/*!t%201,12,3,%0a/*4*/%0aunion%23%20asdfas%0aselect(1),2,load_file/**/(/*!0x2F6574632F706173737764*/),/**/4"}

	targetStings := []string{
		"id=1 and 2",
		"id=1 and 2-1=1",
		"id=3 or 1+0",
		"id=3 or 0+1",
		"id=3 or 0+1",
		"id=1234 union select 1,2,load_file(0x2F6574632F706173737764),4",
		"id=1234 union select 1,2,load_file(0x2F6574632F706173737764),4",
		"id=1234 union select 1,2,load_file(0x2F6574632F706173737764),4",
		"id=1234 union select 1,2,load_file(0x2F6574632F706173737764),4",
		"id=1234 union select (1),2,load_file(0x2F6574632F706173737764),4",
		"id=1234 union select (1),2,load_file(0x2F6574632F706173737764),4"}

	for i := 0; i < len(testStrings); i++ {
		ret := replaceBlankSpace(testStrings[i])
		ret, _ = noiseFiltering(ret)

		if ret != targetStings[i] {
			t.Errorf("\n  case %d\n  %s\n  %s\n  %s\n", i, testStrings[i], ret, targetStings[i])
		}
	}
}

func TestAppendPath(t *testing.T) {
	uriKeyValues := make(map[string][]string)
	uriKeyValues["name"] = []string{"", "abc", "/a/b", "a/b/c"}
	rets := make(map[string][]string)
	rets["name"] = []string{"", "abc", "/a/b", "a/b/c", "a", "b"}

	keyValues := appendPath(uriKeyValues)
	for i := 0; i < len(keyValues["name"]); i++ {
		if keyValues["name"][i] != rets["name"][i] {
			t.Errorf("appendPath(): test error")
		}
	}
}

func TestSkipSafeParamsCheck(t *testing.T) {
	uriKeyValues := make(map[string][]string)

	uriKeyValues["query"] = []string{"a", "b", "c"}
	uriKeyValues["word"] = []string{"ab", "bc"}

	if len(skipSafeParamsCheck(uriKeyValues, 1)) != 0 {
		t.Error("skipSafeParamsCheck(): case 0 should return null map")
	}

	uriKeyValues["q"] = []string{"1", "2"}
	uriKeyValues["w"] = []string{"11"}
	uriKeyValues["bk_key"] = []string{"22"}

	if len(skipSafeParamsCheck(uriKeyValues, 2)) != 0 {
		t.Error("skipSafeParamsCheck(): case 1 should return null map")
	}

	tmpKeyValues := skipSafeParamsCheck(uriKeyValues, 1)
	if len(tmpKeyValues) != 3 {
		t.Error("skipSafeParamsCheck(): case 2 should return map that len = 3!")
	}

	i := 3
	for key, values := range tmpKeyValues {
		tmpValues, find := uriKeyValues[key]
		if !find {
			t.Errorf("skipSafeParamsCheck(): case %d error!", i)
		} else {
			if len(values) != len(tmpValues) {
				t.Errorf("skipSafeParamsCheck(): case %d error!", i)
			}
		}
	}
}

func TestIsAZExN(t *testing.T) {
	chars := []byte{'a', 'n', '_', '0', 'N'}
	rets := []bool{true, false, true, false, false}

	for i := 0; i < len(chars); i++ {
		if rets[i] != isAZExN(chars[i]) {
			t.Errorf("isAZExN(): case %d should return %v", i, rets[i])
		}
	}
}

//test for checkBetweenUnionSelect
func TestCheckBetweenUnionSelect(t *testing.T) {
	type testCase struct {
		Value string
		Ret   bool
	}

	cases := []testCase{
		{" all distinct", true},
		{" ", true},
		{"distinct", true},
		{" al", false},
		{" distinct check (", false},
		{" ( check hello", false},
		{"niceToMeetYou", false},
	}

	for i := 0; i < len(cases); i++ {
		if checkBetweenUnionSelect(cases[i].Value) != cases[i].Ret {
			t.Errorf("TestCheckBetweenUnionSelect case %d\n case : %s",
				i+1, cases[i].Value)
		}
	}
}

var alwaysFalse = func(byte) bool { return false }
