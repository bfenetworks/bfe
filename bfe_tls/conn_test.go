// Copyright (c) 2019 The BFE Authors.
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

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_tls

import (
	"testing"
)

func TestRoundUp(t *testing.T) {
	if roundUp(0, 16) != 0 ||
		roundUp(1, 16) != 16 ||
		roundUp(15, 16) != 16 ||
		roundUp(16, 16) != 16 ||
		roundUp(17, 16) != 32 {
		t.Error("roundUp broken")
	}
}

var paddingTests = []struct {
	in          []byte
	good        bool
	expectedLen int
}{
	{[]byte{1, 2, 3, 4, 0}, true, 4},
	{[]byte{1, 2, 3, 4, 0, 1}, false, 0},
	{[]byte{1, 2, 3, 4, 99, 99}, false, 0},
	{[]byte{1, 2, 3, 4, 1, 1}, true, 4},
	{[]byte{1, 2, 3, 2, 2, 2}, true, 3},
	{[]byte{1, 2, 3, 3, 3, 3}, true, 2},
	{[]byte{1, 2, 3, 4, 3, 3}, false, 0},
	{[]byte{1, 4, 4, 4, 4, 4}, true, 1},
	{[]byte{5, 5, 5, 5, 5, 5}, true, 0},
	{[]byte{6, 6, 6, 6, 6, 6}, false, 0},
}

func TestRemovePadding(t *testing.T) {
	for i, test := range paddingTests {
		payload, good := removePadding(test.in)
		expectedGood := byte(255)
		if !test.good {
			expectedGood = 0
		}
		if good != expectedGood {
			t.Errorf("#%d: wrong validity, want:%d got:%d", i, expectedGood, good)
		}
		if good == 255 && len(payload) != test.expectedLen {
			t.Errorf("#%d: got %d, want %d", i, len(payload), test.expectedLen)
		}
	}
}

var certExampleCom = `308201713082011ba003020102021005a75ddf21014d5f417083b7a010ba2e300d06092a864886f70d01010b050030123110300e060355040a130741636d6520436f301e170d3136303831373231343135335a170d3137303831373231343135335a30123110300e060355040a130741636d6520436f305c300d06092a864886f70d0101010500034b003048024100b37f0fdd67e715bf532046ac34acbd8fdc4dabe2b598588f3f58b1f12e6219a16cbfe54d2b4b665396013589262360b6721efa27d546854f17cc9aeec6751db10203010001a34d304b300e0603551d0f0101ff0404030205a030130603551d25040c300a06082b06010505070301300c0603551d130101ff0402300030160603551d11040f300d820b6578616d706c652e636f6d300d06092a864886f70d01010b050003410059fc487866d3d855503c8e064ca32aac5e9babcece89ec597f8b2b24c17867f4a5d3b4ece06e795bfc5448ccbd2ffca1b3433171ebf3557a4737b020565350a0`

var certWildcardExampleCom = `308201743082011ea003020102021100a7aa6297c9416a4633af8bec2958c607300d06092a864886f70d01010b050030123110300e060355040a130741636d6520436f301e170d3136303831373231343231395a170d3137303831373231343231395a30123110300e060355040a130741636d6520436f305c300d06092a864886f70d0101010500034b003048024100b105afc859a711ee864114e7d2d46c2dcbe392d3506249f6c2285b0eb342cc4bf2d803677c61c0abde443f084745c1a6d62080e5664ef2cc8f50ad8a0ab8870b0203010001a34f304d300e0603551d0f0101ff0404030205a030130603551d25040c300a06082b06010505070301300c0603551d130101ff0402300030180603551d110411300f820d2a2e6578616d706c652e636f6d300d06092a864886f70d01010b0500034100af26088584d266e3f6566360cf862c7fecc441484b098b107439543144a2b93f20781988281e108c6d7656934e56950e1e5f2bcf38796b814ccb729445856c34`

var certFooExampleCom = `308201753082011fa00302010202101bbdb6070b0aeffc49008cde74deef29300d06092a864886f70d01010b050030123110300e060355040a130741636d6520436f301e170d3136303831373231343234345a170d3137303831373231343234345a30123110300e060355040a130741636d6520436f305c300d06092a864886f70d0101010500034b003048024100f00ac69d8ca2829f26216c7b50f1d4bbabad58d447706476cd89a2f3e1859943748aa42c15eedc93ac7c49e40d3b05ed645cb6b81c4efba60d961f44211a54eb0203010001a351304f300e0603551d0f0101ff0404030205a030130603551d25040c300a06082b06010505070301300c0603551d130101ff04023000301a0603551d1104133011820f666f6f2e6578616d706c652e636f6d300d06092a864886f70d01010b0500034100a0957fca6d1e0f1ef4b247348c7a8ca092c29c9c0ecc1898ea6b8065d23af6d922a410dd2335a0ea15edd1394cef9f62c9e876a21e35250a0b4fe1ddceba0f36`

var certDoubleWildcardExampleCom = `308201753082011fa003020102021039d262d8538db8ffba30d204e02ddeb5300d06092a864886f70d01010b050030123110300e060355040a130741636d6520436f301e170d3136303831373231343331335a170d3137303831373231343331335a30123110300e060355040a130741636d6520436f305c300d06092a864886f70d0101010500034b003048024100abb6bd84b8b9be3fb9415d00f22b4ddcaec7c99855b9d818c09003e084578430e5cfd2e35faa3561f036d496aa43a9ca6e6cf23c72a763c04ae324004f6cbdbb0203010001a351304f300e0603551d0f0101ff0404030205a030130603551d25040c300a06082b06010505070301300c0603551d130101ff04023000301a0603551d1104133011820f2a2e2a2e6578616d706c652e636f6d300d06092a864886f70d01010b05000341004837521004a5b6bc7ad5d6c0dae60bb7ee0fa5e4825be35e2bb6ef07ee29396ca30ceb289431bcfd363888ba2207139933ac7c6369fa8810c819b2e2966abb4b`

func TestCertificateSelection(t *testing.T) {
	config := Config{
		Certificates: []Certificate{
			{
				Certificate: [][]byte{fromHex(certExampleCom)},
			},
			{
				Certificate: [][]byte{fromHex(certWildcardExampleCom)},
			},
			{
				Certificate: [][]byte{fromHex(certFooExampleCom)},
			},
			{
				Certificate: [][]byte{fromHex(certDoubleWildcardExampleCom)},
			},
		},
	}

	config.BuildNameToCertificate()

	pointerToIndex := func(c *Certificate) int {
		for i := range config.Certificates {
			if c == &config.Certificates[i] {
				return i
			}
		}
		return -1
	}

	if n := pointerToIndex(config.getCertificateForName("example.com")); n != 0 {
		t.Errorf("example.com returned certificate %d, not 0", n)
	}
	if n := pointerToIndex(config.getCertificateForName("bar.example.com")); n != 1 {
		t.Errorf("bar.example.com returned certificate %d, not 1", n)
	}
	if n := pointerToIndex(config.getCertificateForName("foo.example.com")); n != 2 {
		t.Errorf("foo.example.com returned certificate %d, not 2", n)
	}
	if n := pointerToIndex(config.getCertificateForName("foo.bar.example.com")); n != 3 {
		t.Errorf("foo.bar.example.com returned certificate %d, not 3", n)
	}
	if n := pointerToIndex(config.getCertificateForName("foo.bar.baz.example.com")); n != 0 {
		t.Errorf("foo.bar.baz.example.com returned certificate %d, not 0", n)
	}
}
