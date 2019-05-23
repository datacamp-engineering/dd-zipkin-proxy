package datadog

import (
	"encoding/json"
	"github.com/DataDog/dd-trace-go/tracer"
	"github.com/flachnetz/dd-zipkin-proxy/proxy"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

var log = logrus.WithField("prefix", "datadog")
var logTraces = os.Getenv("DD_LOG_TRACES") == "true"

const flushInterval = 2 * time.Second
const flushSpanCount = 1000

// Create a new default transport.
func DefaultTransport(hostname, port string) tracer.Transport {
	return tracer.NewTransport(hostname, port)
}

func submitTraces(transport tracer.Transport, spansByTrace <-chan map[uint64][]*tracer.Span) {
	for buffer := range spansByTrace {
		count := 0

		// the transport expects a list of list, where each sub-list contains only
		// spans of the same trace.
		var traces [][]*tracer.Span
		for _, spans := range buffer {
			count += len(spans)
			traces = append(traces, spans)
		}

		// if we got traces, send them!
		if len(traces) > 0 {
			log.Infof("Sending %d spans in traces %d traces", count, len(traces))

			if logTraces {
				val, _ := json.MarshalIndent(traces, "", "  ")
				log.Info(string(val))
			} else {
				if _, err := transport.SendTraces(traces); err != nil {
					log.WithError(err).Warn("Error reporting spans to datadog")
				}
			}
		}
	}
}

func Sink(transport tracer.Transport, spans <-chan proxy.Span) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	count := 0
	byTrace := make(map[uint64][]*tracer.Span)

	groupedSpans := make(chan map[uint64][]*tracer.Span, 4)
	defer close(groupedSpans)

	// send the spans in background
	go submitTraces(transport, groupedSpans)

	for {
		var flush bool

		select {
		case span, ok := <-spans:
			if !ok {
				log.Info("Channel closed, stopping sender")
				return
			}

			converted := &tracer.Span{
				Name:     span.Name,
				Resource: span.Tags["dd.resource"],
				Service:  span.Service,

				Start:    span.Timestamp.ToTime().UnixNano(),
				Duration: span.Duration.Nanoseconds(),

				SpanID:   span.Id.Uint64(),
				TraceID:  span.Trace.Uint64(),
				ParentID: span.Parent.Uint64(),

				Meta:    span.Tags,
				Sampled: true,
			}

			count++
			byTrace[converted.TraceID] = append(byTrace[converted.TraceID], converted)
			flush = count >= flushSpanCount

		case <-ticker.C:
			flush = true
		}

		if flush && count > 0 {
			select {
			case groupedSpans <- byTrace:
			default:
				log.Warnf("Could not send %d traces to datadog, sending would block.", len(byTrace))
			}

			// reset collection
			count = 0
			byTrace = make(map[uint64][]*tracer.Span)
		}
	}
}
