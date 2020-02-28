package jwa

// algorithm calculating signature
type SignAlg interface {
	Update(msg []byte) (n int, err error) // update msg
	Sign() (sig []byte)                   // get signature
	Verify(sig []byte) bool               // verify signature
}

// algorithm use to encrypt & decrypt
type EncAlg interface {
	Encrypt(msg []byte) (cipher []byte)
	Decrypt(cipher []byte) (msg []byte)
}
