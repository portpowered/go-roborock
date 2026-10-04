package roborock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const legacyTupleFields = 4

var errResultShape = errors.New("invalid device result shape")

// GetStatus reads presence-aware status; zero and absent measurements remain distinct.
func (s *DeviceSession) GetStatus(ctx context.Context, _ EmptyRequest) (Status, error) {
	return readObject[dependencymodels.Status, Status](ctx, s, dependencymodels.RPCGetStatus)
}

// GetConsumables reads work counters in seconds or the documented device counter units.
func (s *DeviceSession) GetConsumables(ctx context.Context, _ EmptyRequest) (Consumables, error) {
	return readObject[dependencymodels.Consumables, Consumables](ctx, s, dependencymodels.RPCGetConsumables)
}

// GetDND reads the device's local-hour do-not-disturb schedule.
func (s *DeviceSession) GetDND(ctx context.Context, _ EmptyRequest) (DND, error) {
	return readObject[dependencymodels.DND, DND](ctx, s, dependencymodels.RPCGetDND)
}

// GetCleaningSummary accepts object, legacy tuple, and single-duration V1 results.
func (s *DeviceSession) GetCleaningSummary(ctx context.Context, _ EmptyRequest) (CleaningSummary, error) {
	raw, err := s.callV1(ctx, dependencymodels.RPCGetCleaningSummary, dependencymodels.NoParameters{})
	if err != nil {
		return CleaningSummary{}, err
	}

	result, err := decodeSummary(bytes.TrimSpace(raw))
	if err != nil {
		return CleaningSummary{}, roborockerrors.New(
			roborockerrors.Protocol, "GetCleaningSummary", "invalid cleaning summary", err,
		)
	}

	return result, nil
}

func decodeSummary(raw json.RawMessage) (CleaningSummary, error) {
	var result CleaningSummary
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return result, errResultShape
	}

	switch raw[0] {
	case '{':
		_, err := objectResult(raw)
		if err != nil {
			return result, err
		}

		return projectResult[dependencymodels.CleaningSummary, CleaningSummary](raw)
	case '[':
		return decodeSummaryTuple(raw)
	default:
		err := json.Unmarshal(raw, &result.CleanTime)
		if err != nil {
			return result, fmt.Errorf("decode summary duration: %w", err)
		}

		return result, nil
	}
}

func decodeSummaryTuple(raw json.RawMessage) (CleaningSummary, error) {
	var (
		result CleaningSummary
		values dependencymodels.CleaningSummaryLegacy
	)

	err := json.Unmarshal(raw, &values)
	if err != nil {
		return result, fmt.Errorf("decode summary tuple: %w", err)
	}

	if len(values) == 1 && len(values[0]) > 0 && values[0][0] == '{' {
		return decodeSummary(values[0])
	}

	if len(values) == 0 || len(values) > legacyTupleFields {
		return result, errResultShape
	}

	fields := []**int64{&result.CleanTime, &result.CleanArea, &result.CleanCount}
	err = decodeIntegerTuple(values, fields)
	if err != nil {
		return result, err
	}

	if len(values) == legacyTupleFields {
		err := json.Unmarshal(values[legacyTupleFields-1], &result.Records)
		if err != nil {
			return result, fmt.Errorf("decode summary records: %w", err)
		}
	}

	return result, nil
}

// GetCleanRecord reads a record ID. Multi-part results preserve every record in order.
func (s *DeviceSession) GetCleanRecord(ctx context.Context, req CleanRecordRequest) (CleanRecordResult, error) {
	if req.RecordId <= 0 {
		return CleanRecordResult{}, roborockerrors.New(
			roborockerrors.InvalidArgument, "GetCleanRecord", "record ID must be positive", nil,
		)
	}

	raw, err := s.callV1(ctx, dependencymodels.RPCGetCleanRecord, dependencymodels.IntegerParameters{req.RecordId})
	if err != nil {
		return CleanRecordResult{}, err
	}

	records, err := decodeRecords(bytes.TrimSpace(raw))
	if err != nil {
		return CleanRecordResult{}, roborockerrors.New(
			roborockerrors.Protocol, "GetCleanRecord", "invalid cleaning record", err,
		)
	}

	return CleanRecordResult{Records: records}, nil
}

func decodeRecords(raw json.RawMessage) ([]CleanRecord, error) {
	if len(raw) == 0 {
		return nil, errResultShape
	}

	if raw[0] == '{' {
		_, err := objectResult(raw)
		if err != nil {
			return nil, err
		}

		record, err := projectResult[dependencymodels.CleanRecord, CleanRecord](raw)

		return []CleanRecord{record}, err
	}

	var values []json.RawMessage

	err := json.Unmarshal(raw, &values)
	if err != nil {
		return nil, fmt.Errorf("decode record list: %w", err)
	}

	if len(values) == 0 {
		return nil, errResultShape
	}

	if len(values[0]) > 0 && (values[0][0] == '{' || values[0][0] == '[') {
		return decodeRecordParts(values)
	}

	return decodeRecordTuple(values)
}

func decodeRecordParts(values []json.RawMessage) ([]CleanRecord, error) {
	var result []CleanRecord

	for _, value := range values {
		records, err := decodeRecords(value)
		if err != nil {
			return nil, err
		}

		result = append(result, records...)
	}

	return result, nil
}

func decodeRecordTuple(values []json.RawMessage) ([]CleanRecord, error) {
	if len(values) < legacyTupleFields {
		return nil, errResultShape
	}

	var record CleanRecord

	fields := []**int64{&record.Begin, &record.End, &record.Duration, &record.Area}
	for index, field := range fields {
		err := json.Unmarshal(values[index], field)
		if err != nil {
			return nil, fmt.Errorf("decode record tuple: %w", err)
		}
	}

	return []CleanRecord{record}, nil
}

func decodeIntegerTuple(values []json.RawMessage, fields []**int64) error {
	for index, field := range fields {
		if index >= len(values) {
			break
		}

		err := json.Unmarshal(values[index], field)
		if err != nil {
			return fmt.Errorf("decode integer tuple: %w", err)
		}
	}

	return nil
}
