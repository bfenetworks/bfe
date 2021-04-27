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

package semver

import (
	"errors"
	"strconv"
	"strings"
)

var (
	ErrSemverEmpty  = errors.New("version string empty")
	ErrSemverFormat = errors.New("version format error")
)

type Version struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	Blank         string // "-" or "+"
	BuildMetadata string
}

func New(ver string) (v Version, err error) {
	return parse(ver)
}

func parse(ver string) (v Version, err error) {
	v = Version{}

	if ver == "" {
		return v, ErrSemverEmpty
	}

	// major.minor.patch
	version := strings.SplitN(ver, ".", 3)
	if len(version) != 3 {
		return v, ErrSemverFormat
	}

	// major
	v.Major, err = strconv.ParseUint(version[0], 10, 64)
	if err != nil {
		return v, err
	}

	// minor
	v.Minor, err = strconv.ParseUint(version[1], 10, 64)
	if err != nil {
		return v, err
	}

	// patch & blank & buildMetaData
	patchStr := version[2]
	inx := strings.IndexFunc(patchStr, func(r rune) bool {
		return r == '-' || r == '+'
	})

	if inx > 0 {
		patch := patchStr[:inx]
		blank := patchStr[inx : inx+1]
		buildMetaData := patchStr[inx+1:]

		v.Patch, err = strconv.ParseUint(patch, 10, 64)
		if err != nil {
			return v, err
		}

		v.Blank = blank
		v.BuildMetadata = buildMetaData
	} else {
		v.Patch, err = strconv.ParseUint(patchStr, 10, 64)
		if err != nil {
			return v, err
		}
	}

	return
}

func (v Version) String() string {
	b := make([]byte, 0, 5)
	b = strconv.AppendUint(b, v.Major, 10)
	b = append(b, '.')
	b = strconv.AppendUint(b, v.Minor, 10)
	b = append(b, '.')
	b = strconv.AppendUint(b, v.Patch, 10)

	if v.Blank != "" {
		b = append(b, []byte(v.Blank)...)
		b = append(b, []byte(v.BuildMetadata)...)
	}

	return string(b)
}

// Equal compares Versions v to x:
// -1 == v is less than x
// 0 == v is equal to x
// 1 == v is greater than x
func (v Version) Equal(x Version) bool {
	return v.CompareMajor(x) == 0 && v.CompareMinor(x) == 0 && v.ComparePatch(x) == 0
}

func (v Version) CompareMajor(x Version) int {
	if v.Major != x.Major {
		if v.Major > x.Major {
			return 1
		}
		return -1
	}
	return 0
}

func (v Version) CompareMinor(x Version) int {
	if v.Minor != x.Minor {
		if v.Minor > x.Minor {
			return 1
		}
		return -1
	}
	return 0
}

func (v Version) ComparePatch(x Version) int {
	if v.Patch != x.Patch {
		if v.Patch > x.Patch {
			return 1
		}
		return -1
	}
	return 0
}
