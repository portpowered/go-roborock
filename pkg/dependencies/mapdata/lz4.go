package mapdata

import (
	"encoding/binary"
	"fmt"
	"github.com/portpowered/go-roborock/internal/protocol"
)

func lz4Length(data []byte, index *int, value, limit int) (int, error) {
	if value != protocol.MapLZ4NibbleExtension {
		return value, nil
	}

	for {
		if *index >= len(data) {
			return 0, fmt.Errorf("LZ4 truncated length: %w", ErrMalformed)
		}

		part := int(data[*index])
		*index++

		if value > limit-part {
			return 0, fmt.Errorf("LZ4 expansion bound: %w", ErrMalformed)
		}

		value += part
		if part != protocol.MapLZ4LengthExtension {
			return value, nil
		}
	}
}

func decompressLZ4(data []byte, limit int) ([]byte, error) {
	output := make([]byte, 0)

	index := 0
	for index < len(data) {
		token := data[index]
		index++

		length, err := lz4Length(data, &index, int(token>>protocol.MapLZ4TokenShift), limit)
		if err != nil {
			return nil, err
		}

		if length > len(data)-index || length > limit-len(output) {
			return nil, fmt.Errorf("LZ4 literal bounds: %w", ErrMalformed)
		}

		output = append(output, data[index:index+length]...)

		index += length

		if index == len(data) {
			return output, nil
		}

		output, index, err = lz4Match(data, output, index, limit, token)
		if err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("LZ4 missing final token: %w", ErrMalformed)
}

func lz4Match(data, output []byte, index, limit int, token byte) ([]byte, int, error) {
	if len(data)-index < protocol.MapLZ4OffsetSize {
		return nil, index, fmt.Errorf("LZ4 truncated offset: %w", ErrMalformed)
	}

	offset := int(binary.LittleEndian.Uint16(data[index:]))
	index += protocol.MapLZ4OffsetSize

	if offset == 0 || offset > len(output) {
		return nil, index, fmt.Errorf("LZ4 invalid back-reference: %w", ErrMalformed)
	}

	length, err := lz4Length(data, &index, int(token&protocol.MapLZ4NibbleExtension), limit)
	if err != nil {
		return nil, index, err
	}

	if length > limit-len(output)-protocol.MapLZ4MatchMinimum {
		return nil, index, fmt.Errorf("LZ4 match bounds: %w", ErrMalformed)
	}

	for range length + protocol.MapLZ4MatchMinimum {
		output = append(output, output[len(output)-offset])
	}

	return output, index, nil
}
