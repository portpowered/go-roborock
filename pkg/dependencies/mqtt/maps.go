package mqtt

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

type mapQuery struct {
	id     int64
	prefix byte
	reply  chan response
}

// FetchMapV1 retrieves the current V1 map, decrypting and inflating its payload.
func (s *Session) FetchMapV1(ctx context.Context) ([]byte, error) {
	if s.config.Protocol != protocol.MQTTVersionV1 {
		return nil, unsupported("V1 map")
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	query, err := s.beginMap(ctx, 0)
	if err != nil {
		return nil, err
	}

	defer s.endMap()

	s.mu.Lock()
	query.id = int64(s.sequence.Add(1)%protocol.MapsV1RequestIDRange) + 1
	s.mu.Unlock()

	payload, err := s.rpcPayload(query.id, protocol.MapsV1GetMethod, json.RawMessage(protocol.MQTTEmptyParamsJSON))
	if err != nil {
		return nil, err
	}

	err = s.send(ctx, payload)
	if err != nil {
		return nil, transportError("V1 map", err)
	}

	return s.awaitMap(ctx, query)
}

// FetchMapQ7 requests a saved Q7 map by identifier and returns the next observed
// map push while pending. Pushes carry no request ID, so causal freshness cannot
// be established. Serial and product model supply the map key derivation.
func (s *Session) FetchMapQ7(ctx context.Context, mapID int64, serial, model string) ([]byte, error) {
	if s.config.Protocol != protocol.B01Version {
		return nil, unsupported("Q7 map")
	}

	key, err := q7MapKey(serial, model)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	query, err := s.beginMap(ctx, 0)
	if err != nil {
		return nil, err
	}

	defer s.endMap()

	params, err := json.Marshal(dependencymodels.MapsQ7UploadRequest{MapId: mapID})
	if err != nil {
		return nil, transportError("Q7 map parameters", err)
	}

	id := int64(protocol.B01Q7MessageIDBase) + int64(s.sequence.Add(1))

	payload, err := b01RPCPayload(id, dependencymodels.ServiceUploadByMapid, params)
	if err != nil {
		return nil, err
	}

	err = s.send(ctx, payload)
	if err != nil {
		return nil, transportError("Q7 map", err)
	}

	raw, err := s.awaitMap(ctx, query)
	if err != nil {
		return nil, err
	}

	return decodeQ7Map(raw, key)
}

// FetchMapQ10 sends read-only DP102 and returns the next observed current-map
// push while pending. The uncorrelated push cannot establish causal freshness.
// The returned inner payload retains the Q10 framing and compression.
func (s *Session) FetchMapQ10(ctx context.Context) ([]byte, error) {
	return s.fetchQ10(ctx, protocol.MapsQ10CurrentPrefix)
}

// FetchTraceQ10 sends read-only DP102 and returns the next observed trace push
// while pending, without a causal freshness guarantee. Its coordinates and
// timestamp are independent of a separately retrieved current map.
func (s *Session) FetchTraceQ10(ctx context.Context) ([]byte, error) {
	return s.fetchQ10(ctx, protocol.MapsQ10TracePrefix)
}

func (s *Session) fetchQ10(ctx context.Context, prefix byte) ([]byte, error) {
	if s.config.Protocol != protocol.B01Version {
		return nil, unsupported("Q10 map")
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	query, err := s.beginMap(ctx, prefix)
	if err != nil {
		return nil, err
	}

	defer s.endMap()

	err = s.sendB01DPS(ctx, protocol.B01Q10CurrentMapDatapoint, json.RawMessage(protocol.B01EmptyObjectJSON))
	if err != nil {
		return nil, transportError("Q10 map", err)
	}

	return s.awaitMap(ctx, query)
}

func (s *Session) beginMap(ctx context.Context, prefix byte) (*mapQuery, error) {
	select {
	case s.mapGate <- struct{}{}:
	case <-ctx.Done():
		return nil, transportError("map", ctx.Err())
	case <-s.done:
		return nil, s.closedError()
	}

	query := &mapQuery{id: 0, reply: make(chan response, 1), prefix: prefix}

	s.mu.Lock()
	s.mapQuery = query
	s.mu.Unlock()

	return query, nil
}

func (s *Session) endMap() {
	s.mu.Lock()
	s.mapQuery = nil
	s.mu.Unlock()
	<-s.mapGate
}

func (s *Session) awaitMap(ctx context.Context, query *mapQuery) ([]byte, error) {
	select {
	case reply := <-query.reply:
		return reply.value, reply.err
	case <-ctx.Done():
		if s.config.Protocol == protocol.B01Version {
			// An uncorrelated late response must not satisfy another map fetch.
			s.fail(ctx.Err())
		}

		return nil, transportError("map", ctx.Err())
	case <-s.done:
		return nil, s.closedError()
	}
}

func (s *Session) deliverMap(payload []byte) error {
	s.mu.Lock()

	query := s.mapQuery
	if query == nil {
		s.mu.Unlock()

		return nil
	}

	matched, matchErr := s.matchesMap(payload, query)
	if matchErr != nil || !matched {
		s.mu.Unlock()

		return matchErr
	}
	s.mu.Unlock()

	value := bytes.Clone(payload)

	var err error

	if s.config.Protocol == protocol.MQTTVersionV1 {
		value, err = decodeV1Map(payload[protocol.MapsV1HeaderSize:], s.security.Nonce)
	}

	select {
	case query.reply <- response{value: value, err: err}:
	default:
	}

	return nil
}

func (s *Session) matchesMap(payload []byte, query *mapQuery) (bool, error) {
	if s.config.Protocol == protocol.MQTTVersionV1 {
		if len(payload) < protocol.MapsV1HeaderSize {
			return false, invalid("V1 map", "truncated map header")
		}

		endpoint := payload[protocol.MapsV1EndpointOffset:protocol.MapsV1EndpointEnd]
		requestID := binary.LittleEndian.Uint16(payload[protocol.MapsV1RequestIDOffset:protocol.MapsV1RequestIDEnd])

		return bytes.HasPrefix(endpoint, []byte(s.security.Endpoint)) && int64(requestID) == query.id, nil
	}

	if query.prefix == 0 {
		return true, nil
	}

	return len(payload) >= protocol.MapsQ10PrefixSize && payload[0] == query.prefix &&
		payload[1] == protocol.MapsQ10PrefixVersion, nil
}
