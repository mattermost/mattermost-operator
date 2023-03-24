package utils

import (
	"bytes"
	"encoding/base32"

	"github.com/pborman/uuid"
)

var encoding = base32.NewEncoding("ybndrfg8ejkmcpqxot1uwisza345h769")

// New16ID is a globally unique identifier.  It is a [A-Z0-9] string 16
// characters long.  It is a UUID version 4 Guid that is zbased32 encoded
// with the padding stripped off.
func New16ID() []byte {
	var b bytes.Buffer
	encoder := base32.NewEncoder(encoding, &b)
	_, _ = encoder.Write(uuid.NewRandom())
	encoder.Close()
	b.Truncate(16) // removes the '==' padding
	return b.Bytes()
}

// New28ID is a globally unique identifier.  It is a [A-Z0-9] string 28
// characters long.  It is a UUID version 4 Guid that is zbased32 encoded
// with the padding stripped off.
func New28ID() []byte {
	var b bytes.Buffer
	encoder := base32.NewEncoder(encoding, &b)
	_, _ = encoder.Write(uuid.NewRandom())
	encoder.Close()
	b.Truncate(28) // removes the '==' padding
	return b.Bytes()
}
