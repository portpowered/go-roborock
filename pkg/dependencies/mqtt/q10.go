package mqtt

import (
	"context"
	"encoding/json"
	"maps"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

type q10Query struct{ reply chan response }

// QueryMapListQ10 requests DP61 op:list inside DP101 and returns the next observed
// list reply while pending. This protocol has no request ID or freshness guarantee.
// It never sends the movement-triggering DP61 op:get request.
func (s *Session) QueryMapListQ10(ctx context.Context) (dependencymodels.MapsQ10ListResult, error) {
	var result dependencymodels.MapsQ10ListResult
	if s.config.Protocol != protocol.B01Version {
		return result, unsupported("Q10 maps")
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	_, err := s.beginMap(ctx, 0)
	if err != nil {
		return result, err
	}

	defer s.endMap()

	query := &q10Query{reply: make(chan response, 1)}

	s.mu.Lock()
	s.q10Query = query
	s.mapQuery = nil

	s.mu.Unlock()

	defer s.endQ10()

	err = s.sendB01DPS(ctx, protocol.B01Q10CommonDatapoint, json.RawMessage(protocol.B01Q10MapListRequestJSON))
	if err != nil {
		return result, transportError("Q10 maps", err)
	}

	select {
	case reply := <-query.reply:
		if reply.err != nil {
			return result, reply.err
		}

		err = json.Unmarshal(reply.value, &result)
		if err != nil {
			return result, transportError("Q10 maps", err)
		}

		return result, nil
	case <-ctx.Done():
		s.fail(ctx.Err())

		return result, transportError("Q10 maps", ctx.Err())
	case <-s.done:
		return result, s.closedError()
	}
}

func (s *Session) endQ10() {
	s.mu.Lock()
	s.q10Query = nil
	s.mu.Unlock()
}

func (s *Session) deliverQ10(dps map[string]json.RawMessage) error {
	err := expandQ10DPS(dps)
	if err != nil {
		return err
	}

	return s.deliverQ10List(dps[protocol.B01Q10MapListDatapoint])
}

func expandQ10DPS(dps map[string]json.RawMessage) error {
	if common := dps[protocol.B01Q10CommonDatapoint]; common != nil {
		var values map[string]json.RawMessage

		err := json.Unmarshal(common, &values)
		if err != nil {
			return transportError("Q10 common datapoints", err)
		}

		maps.Copy(dps, values)
	}

	return nil
}

func (s *Session) deliverQ10List(value json.RawMessage) error {
	if value == nil {
		return nil
	}

	var result dependencymodels.MapsQ10ListResult

	err := json.Unmarshal(value, &result)
	if err != nil {
		return transportError("Q10 map list", err)
	}

	if result.Op == nil || *result.Op != protocol.B01Q10MapListOperation {
		return nil
	}

	reply := response{value: value, err: nil}
	if result.Result == nil || *result.Result != protocol.B01Q10MapListSuccess {
		reply.err = invalid("Q10 map list", "device rejected map list request")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if query := s.q10Query; query != nil {
		select {
		case query.reply <- reply:
		default:
		}
	}

	return nil
}
