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

package mod_compress

import (
	"compress/gzip"
	"errors"
	"fmt"
)

import (
	"github.com/andybalholm/brotli"
)

const (
	ActionGzip   = "GZIP"
	ActionBrotli = "BROTLI"
)

type ActionFile struct {
	Cmd       *string
	Quality   *int
	FlushSize *int
}

type Action struct {
	Cmd       string
	Quality   int
	FlushSize int
}

func ActionFileCheck(conf *ActionFile) error {
	if conf.Cmd == nil {
		return errors.New("no Cmd")
	}

	switch *conf.Cmd {
	case ActionGzip:
		if *conf.Quality < gzip.HuffmanOnly || *conf.Quality > gzip.BestCompression {
			return fmt.Errorf("Quality should be [%d, %d]",
				gzip.HuffmanOnly, gzip.BestCompression)
		}
	case ActionBrotli:
		if *conf.Quality < brotli.BestSpeed || *conf.Quality > brotli.BestCompression {
			return fmt.Errorf("Quality should be [%d, %d]",
				brotli.BestSpeed, brotli.BestCompression)
		}
	default:
		return fmt.Errorf("invalid cmd: %s", *conf.Cmd)
	}

	if *conf.FlushSize < 64 || *conf.FlushSize > 4096 {
		return fmt.Errorf("FlushSize should be [64, 4096]")
	}

	return nil
}

func actionConvert(actionFile ActionFile) Action {
	action := Action{}
	action.Cmd = *actionFile.Cmd
	action.Quality = *actionFile.Quality
	action.FlushSize = *actionFile.FlushSize
	return action
}
