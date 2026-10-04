package mqtt

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"strings"

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
		return protocolError("Q10 map list shape", err)
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

	err = validateQ10MapListEntries(value)
	if err != nil {
		return transportError("Q10 map list", err)
	}

	reply := response{value: value, err: nil}
	reply.err = q10ListRejection(result)

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

// validateQ10MapListEntries checks required identifier presence before decoding
// loses that distinction. JSON tags come from the canonical generated wire model.
func validateQ10MapListEntries(value json.RawMessage) error {
	var object map[string]json.RawMessage

	err := json.Unmarshal(value, &object)
	if err != nil {
		return protocolError("Q10 map list shape", err)
	}

	dataField, _ := reflect.TypeFor[dependencymodels.MapsQ10ListResult]().FieldByName("Data")
	dataKey, _, _ := strings.Cut(dataField.Tag.Get("json"), ",")

	data, present := object[dataKey]
	if !present {
		return nil
	}

	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errInvalidMapListShape
	}

	var entries []map[string]json.RawMessage

	err = json.Unmarshal(data, &entries)
	if err != nil {
		return protocolError("Q10 map list shape", err)
	}

	for _, entry := range entries {
		err = validateQ10MapListEntry(entry)
		if err != nil {
			return err
		}
	}

	return nil
}

func q10ListRejection(result dependencymodels.MapsQ10ListResult) error {
	if result.Result == nil {
		return protocolError("Q10 map list", errInvalidMapListShape)
	}

	if *result.Result == protocol.B01Q10MapListSuccess {
		return nil
	}

	return protocolError("Q10 map list", &RPCError{Code: *result.Result, Message: "device rejected map list request"})
}

func validateQ10MapListEntry(entry map[string]json.RawMessage) error {
	model := reflect.TypeFor[dependencymodels.MapsQ10ListEntry]()
	for index := range model.NumField() {
		tag := model.Field(index).Tag.Get("json")
		key, options, _ := strings.Cut(tag, ",")

		value, present := entry[key]

		if !present && options != "omitempty" {
			return errInvalidMapListShape
		}

		if present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errInvalidMapListShape
		}
	}

	return nil
}
