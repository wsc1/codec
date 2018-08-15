package codec

import (
	"bufio"
	"errors"
	"io"
	"reflect"

	"zikichombo.org/sound"
	"zikichombo.org/sound/ops"
	"zikichombo.org/sound/sample"
)

// ErrUnknownCodec is an error representing that a codec is unknown.
var ErrUnknownCodec = errors.New("unknown codec")

// ErrUnsupportedSampleCodec can be used by codec implementations
// which receive a request for encoding or decoding with a sample codec
// that is unsupported or doesn't make sense.
var ErrUnsupportedSampleCodec = errors.New("unsupported sample codec")

// AnySampleCodec is a sample.Codec which is used as a wildcard sample.Codec
// in this interface.
var AnySampleCodec = sample.Codec(-1)

// Codec represents a way of encoding and decoding sound.
type Codec struct {
	// Extensions lists the filename extensions which this codec claims to support.
	// Examples are .wav, .ogg, .caf.  The extension string includes the leading '.'.
	Extensions []string // Filename extensions

	// Sniff is a function which when provided with a *bufio.Reader r, may
	// call r.Peek(), and only r.Peek().  Sniff should return true only if
	// this codec has a Decoder function for the data based on the data
	// aquired via r.Peek().
	Sniff func(*bufio.Reader) bool

	// DefaultSampleCodec gives a default or preferred sample codec for the codec.
	// Some codecs, such as perception based compressed codecs, don't really have
	// a defined sample.Codec and should use AnySampleCodec for this field. Codecs
	// which have only a decoder should also have AnySampleCodec in this field.
	DefaultSampleCodec sample.Codec

	// Decoder tries to turn an io.ReadCloser into a sound.Source.  In the event
	// the decoder does not use a single defined sample.Codec during the entire
	// decoding process for the resulting sound.Source, then the second return
	// value should be AnySampleCodec (if the error return value is nil).
	Decoder func(io.ReadCloser) (sound.Source, sample.Codec, error)

	// Encoder tries to turn an io.WriteCloser into a sound.Sink.
	// The sample.Codec argument can specify the desired sample Codec.
	// For encodings which don't use a defined sample.Codec, the function
	// should return (nil, ErrUnsupportedSampleCodec) in the event c
	// is not AnySampleCodec.
	Encoder func(w io.WriteCloser, c sample.Codec) (sound.Sink, error)

	// RandomAccess tries to turn an io.ReadWriteSeeker into sound.RandomAccess.
	// If the codec does not make use of a defined sample.Codec and c is
	// not AnySampleCodec, then the function should return (nil, ErrUnsupporteSampleCodec).
	RandomAccess func(ws io.ReadWriteSeeker, c sample.Codec) (sound.RandomAccess, error)

	// PkgPath is the package path of the codec functions above.  It is populated
	// by RegisterCodec().  RegisterCodec() will only succeed if all non-nil codec
	// functions have the same package path.  CodecFor() allows callers to select
	// Codecs by PkgPath in the case of conflicts when there are multiple codecs
	// available.
	PkgPath string
}

var codecs []Codec

// ErrNonUniformCodec is an error indicating a codec has non
// uniform package paths beween the available codec functions.
// (Encoder,Decoder,RandomAccess)
var ErrNonUniformCodec = errors.New("non uniform codec")

func pkgPath(v interface{}) string {
	typ := reflect.ValueOf(v).Type()
	return typ.PkgPath()
}

// RegisterCodec registers a codec so that consumers of this package
// can treat sound I/O generically and switch between codecs.
//
// A package "p" implementing a Codec can register a codec in its init()
// function.
//
// Although c is a pointer, a "deep" copy of c is added to the list of registered codecs.
//
// RegisterCodec returns a nil error upon success and ErrNonUniformCodec
// if any two non-nil functions from c.{Encoder,Decoder,RandomAccess}
// do not have the same package path.
func RegisterCodec(c *Codec) error {

	pkgPaths := make([]string, 0, 3)
	if c.Encoder != nil {
		pkgPaths = append(pkgPaths, pkgPath(c.Encoder))
	}
	if c.Decoder != nil {
		pkgPaths = append(pkgPaths, pkgPath(c.Decoder))
	}
	if c.RandomAccess != nil {
		pkgPaths = append(pkgPaths, pkgPath(c.RandomAccess))
	}
	var thePath string
	if len(pkgPaths) > 1 {
		thePath = pkgPaths[0]
		for i := 1; i < len(pkgPaths); i++ {
			if pkgPaths[i] != thePath {
				return ErrNonUniformCodec
			}
		}
	}

	exts := make([]string, len(c.Extensions))
	for i, ext := range c.Extensions {
		exts[i] = ext
	}

	codecs = append(codecs, Codec{
		PkgPath:            thePath,
		Extensions:         exts,
		Sniff:              c.Sniff,
		DefaultSampleCodec: c.DefaultSampleCodec,
		Decoder:            c.Decoder,
		Encoder:            c.Encoder,
		RandomAccess:       c.RandomAccess})
	return nil
}

// CodecFor tries to find a codec based on a filename extension.
//
// The returned codec, although a pointer to a struct with fields, should be
// treated as read-only.  Not doing so may result in race conditions or worse.
//
// The function pkgSel may be used to filter or select packages implementing
// codecs.  If the supplied value is nil, then by default the behavior
// is as if the function body were "return true".  As multiple codec implementations
// may exist, the first codec whose package path p is such that pkgSel(p) is true
// will be returned.
func CodecFor(ext string, pkgSel func(string) bool) (*Codec, error) {
	for i := range codecs {
		c := &codecs[i]
		for _, codExt := range c.Extensions {
			if ext == codExt {
				if pkgSel == nil || pkgSel(c.PkgPath) {
					return c, nil
				}
			}
		}
	}
	return nil, ErrUnknownCodec
}

// Decoder tries to turn an io.ReadCloser into a sound.Source.  If it fails, it
// returns a non-nil error.
//
// The function pkgSel may be used to filter or select packages implementing
// codecs.  If the supplied value is nil, then by default the behavior is as if
// the function body were "return true".  As multiple codec implementations may
// exist, the first codec whose package path p is such that pkgSel(p) is true
// will be returned.
//
// If it succeeds, it also returns a sample.Codec which may either be:
//
// 1. AnySampleCodec, indicating there is no fixed sample codec for the decoder
// behind sound.Source; or
//
// 2. A sample.Codec which defined the data received in sound.Source.
func Decoder(r io.ReadCloser, pkgSel func(string) bool) (sound.Source, sample.Codec, error) {
	br := bufio.NewReader(r)
	var theCodec *Codec
	for i := range codecs {
		c := &codecs[i]
		if c.Sniff != nil && c.Sniff(br) && (pkgSel == nil || pkgSel(c.PkgPath)) {
			theCodec = c
		}
	}
	if theCodec == nil {
		return nil, AnySampleCodec, ErrUnknownCodec
	}
	return theCodec.Decoder(r)
}

// Encoder tries to turn an io.WriteCloser into a sound.Sink
// given a filename extension.
func Encoder(dst io.WriteCloser, ext string) (sound.Sink, error) {
	co, err := CodecFor(ext, nil)
	if err != nil {
		return nil, err
	}
	return co.Encoder(dst, co.DefaultSampleCodec)
}

// EncoderWith tries to turn an io.WriteCloser into a sound.Sink
// given a filename extension and a sample.Codec.
//
// The sample codec may be AnySampleCodec, which should be used when
// the caller is not sure of the desired sample codec c.
func EncoderWith(dst io.WriteCloser, ext string, c sample.Codec) (sound.Sink, error) {
	co, err := CodecFor(ext, nil)
	if err != nil {
		return nil, err
	}
	return co.Encoder(dst, c)
}

// Encode encodes a sound.Source to an io.WriteCloser, selecting
// the codec based on a filename extension ext. It returns any
// error that may have been encountered in that process.
func Encode(dst io.WriteCloser, src sound.Source, ext string) error {
	return EncodeWith(dst, src, ext, AnySampleCodec)
}

// EncodeWith encodes a sound.Source to an io.WriteCloser, selecting
// the codec based on a filename extension ext and desired sample codec co.
//
// EncodeWith returns any error that may have been encountered in that process.
func EncodeWith(dst io.WriteCloser, src sound.Source, ext string, co sample.Codec) error {
	snk, err := EncoderWith(dst, ext, co)
	if err != nil {
		return err
	}
	return ops.Copy(snk, src)
}