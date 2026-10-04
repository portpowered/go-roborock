package mqtt

import (
	"context"
	"encoding/json"
	"maps"
	"strconv"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

type a01Query struct {
	expected map[string]struct{}
	values   map[int]json.RawMessage
	ready    chan struct{}
}

// QueryA01 merges updates until every requested datapoint arrives.
// Since A01 has no request IDs, the session serializes queries and closes on timeout.
func (s *Session) QueryA01(ctx context.Context, datapoints []int) (map[int]json.RawMessage, error) {
	if s.config.Protocol != protocol.MQTTVersionA01 {
		return nil, unsupported("query")
	}

	query, keys, err := newA01Query(datapoints)
	if err != nil {
		return nil, err
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

	s.mu.Lock()
	s.a01 = query

	s.mu.Unlock()

	defer s.endA01Query()

	err = s.sendA01(ctx, map[string]json.RawMessage{protocol.MQTTQueryDatapoint: keys})
	if err != nil {
		return nil, err
	}

	return s.awaitA01(ctx, query)
}

// SetA01 publishes datapoint changes. Success indicates publication, not device acknowledgement or completed action.
func (s *Session) SetA01(ctx context.Context, values map[int]json.RawMessage) error {
	if s.config.Protocol != protocol.MQTTVersionA01 {
		return unsupported("set")
	}

	if len(values) == 0 {
		return invalid("set", "A01 set requires values")
	}

	dps := make(map[string]json.RawMessage, len(values))

	for key, value := range values {
		if key <= 0 || strconv.Itoa(key) == protocol.MQTTQueryDatapoint || !json.Valid(value) {
			return invalid("set", "invalid A01 datapoint")
		}

		dps[strconv.Itoa(key)] = value
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	return s.sendA01(ctx, dps)
}

func newA01Query(datapoints []int) (*a01Query, json.RawMessage, error) {
	if len(datapoints) == 0 {
		return nil, nil, invalid("query", "A01 query requires datapoints")
	}

	query := &a01Query{
		expected: make(map[string]struct{}), values: make(map[int]json.RawMessage), ready: make(chan struct{}, 1),
	}

	for _, key := range datapoints {
		if key <= 0 {
			return nil, nil, invalid("query", "datapoint must be positive")
		}

		query.expected[strconv.Itoa(key)] = struct{}{}
	}

	keys, err := json.Marshal(datapoints)
	if err != nil {
		return nil, nil, transportError("query values", err)
	}

	encoded, err := json.Marshal(string(keys))
	if err != nil {
		return nil, nil, transportError("query string", err)
	}

	return query, encoded, nil
}

func (s *Session) endA01Query() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.a01 = nil
}

func (s *Session) awaitA01(ctx context.Context, query *a01Query) (map[int]json.RawMessage, error) {
	select {
	case <-query.ready:
		return s.queryResult(query), nil
	case <-ctx.Done():
		// Closing prevents a late uncorrelated update from satisfying a later query.
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
	maps.Copy(values, query.values)

	return values
}

func (s *Session) sendA01(ctx context.Context, dps map[string]json.RawMessage) error {
	payload, err := json.Marshal(dependencymodels.MQTTEnvelope{Dps: dps, T: time.Now().Unix()})
	if err != nil {
		return transportError("A01 envelope", err)
	}

	err = s.send(ctx, payload)
	if err != nil {
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
