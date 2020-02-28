package jwa

import "hash"

type HashFactory func() hash.Hash
