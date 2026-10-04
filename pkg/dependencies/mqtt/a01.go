package mqtt

import (
	"context"
	"encoding/json"
	"github.com/portpowered/go-roborock/internal/protocol"
	"strconv"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

type a01Query struct {
	expected map[string]struct{}
	values   map[int]json.RawMessage
	ready    chan struct{}
}

// QueryA01 merges updates until every requested datapoint has arrived.
// A01 has no request identifier, so one query may be active per device session.
func (s *Session) QueryA01(ctx context.Context, datapoints []int) (map[int]json.RawMessage, error) {
	if s.config.Protocol != protocol.MQTTVersionA01 {
		return nil, unsupported("query")
	}

	if len(datapoints) == 0 {
		return nil, invalid("query", "A01 query requires datapoints")
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	select {
	case s.a01Gate <- struct{}{}:
	case <-ctx.Done():
		return nil, transportError("query", ctx.Err())
	case <-s.done:
		return nil, s.closedError()
	}

	defer func() { <-s.a01Gate }()

	query := &a01Query{expected: make(map[string]struct{}), values: make(map[int]json.RawMessage), ready: make(chan struct{}, 1)}

	for _, key := range datapoints {
		if key <= 0 {
			return nil, invalid("query", "datapoint must be positive")
		}

		query.expected[strconv.Itoa(key)] = struct{}{}
	}

	s.mu.Lock()
	s.a01 = query
	s.mu.Unlock()

	defer func() { s.mu.Lock(); s.a01 = nil; s.mu.Unlock() }()

	keys, err := json.Marshal(datapoints)
	if err != nil {
		return nil, err
	}

	keys, err = json.Marshal(string(keys))
	if err != nil {
		return nil, transportError("query", err)
	}

	if err = s.sendA01(ctx, map[string]json.RawMessage{protocol.MQTTQueryDatapoint: keys}); err != nil {
		return nil, err
	}

	select {
	case <-query.ready:
		return s.queryResult(query), nil
	case <-ctx.Done():
		// No IDs exist in A01. A timed-out query cannot be safely reused: a late
		// update might otherwise satisfy a subsequent query for the same datapoint.
		s.fail(ctx.Err())

		return nil, transportError("query", ctx.Err())
	case <-s.done:
		select {
		case <-query.ready:
			return s.queryResult(query), nil
		default:
		}

		return nil, s.closedError()
	}
}

func (s *Session) queryResult(query *a01Query) map[int]json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	values := make(map[int]json.RawMessage, len(query.values))
	for key, value := range query.values {
		values[key] = value
	}

	return values
}

// SetA01 publishes datapoint changes. Success means publication, not device action completion.
func (s *Session) SetA01(ctx context.Context, values map[int]json.RawMessage) error {
	if s.config.Protocol != protocol.MQTTVersionA01 {
		return unsupported("set")
	}

	if len(values) == 0 {
		return invalid("set", "A01 set requires values")
	}

	dps := make(map[string]json.RawMessage, len(values))

	for key, value := range values {
		if key <= 0 || key == 10000 || !json.Valid(value) {
			return invalid("set", "invalid A01 datapoint")
		}

		dps[strconv.Itoa(key)] = value
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	return s.sendA01(ctx, dps)
}

func (s *Session) sendA01(ctx context.Context, dps map[string]json.RawMessage) error {
	payload, err := json.Marshal(dependencymodels.MQTTEnvelope{Dps: dps, T: time.Now().Unix()})
	if err != nil {
		return err
	}

	if err = s.send(ctx, payload); err != nil {
		return transportError("A01 publish", err)
	}

	return nil
}

func (s *Session) deliverA01(dps map[string]json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.a01 == nil {
		return nil
	}

	for key, value := range dps {
		index, err := strconv.Atoi(key)
		if err != nil {
			return errA01DatapointKeyIsNotNumeric
		}

		if _, expected := s.a01.expected[key]; expected {
			s.a01.values[index] = value
		}
	}

	if len(s.a01.values) == len(s.a01.expected) {
		select {
		case s.a01.ready <- struct{}{}:
		default:
		}
	}

	return nil
}
