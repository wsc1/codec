package codec

import (
	"testing"
)

func TestRegister(t *testing.T) {
	RegisterCodec(NullCodec{})
}

// don't know how to easily do this without actual codecs,
// perhap we test more thoroughly in other packages?
//
// For boot strapping some stuff is tested out of the repo.

/*
func TestDecoder(t *testing.T) {
}

func TestSeekingDecoder(t *testing.T) {
}

func TestEncoder(t *testing.T) {
}

func TestRandomAccess(t *testing.T) {
}
*/
