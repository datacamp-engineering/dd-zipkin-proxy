package codec

import (
	"github.com/flachnetz/dd-zipkin-proxy/cache"
	"github.com/flachnetz/dd-zipkin-proxy/codec/hyperjson"
	"github.com/pkg/errors"
	"sync"
	"unsafe"
)

const pooledBufferSliceLength = 8 * 1024

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, pooledBufferSliceLength)
	},
}

func endpointValueDecoder(target unsafe.Pointer, p *hyperjson.Parser) error {
	next, err := p.NextType()
	if err != nil {
		return err
	}

	if next == hyperjson.TypeNull {
		*(*endpoint)(target) = endpoint{
			ServiceName: "",
		}
		return p.Skip()
	}

	var decoder = hyperjson.MakeStructDecoder([]hyperjson.Field{
		{
			JsonName: "serviceName",
			Decoder:  hyperjson.StringValueDecoder,
			Offset:   hyperjson.OffsetOf(endpoint{}, "ServiceName"),
		},
	})
	return decoder(target, p)
}

// Decodes any type of literal to a string
func anyValueDecoder(target unsafe.Pointer, p *hyperjson.Parser) error {
	tok, err := p.ReadLiteral()
	if err != nil {
		return errors.WithMessage(err, "decode value")
	}

	// assign to target
	*(*string)(target) = cache.StringForByteSliceCopy(tok.Value)

	return nil
}
