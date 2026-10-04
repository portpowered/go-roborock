package mapdata

import "fmt"

func lz4Length(data []byte, index *int, value, limit int) (int, error) {
	if value != 15 {
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
		if part != 255 {
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
		length, err := lz4Length(data, &index, int(token>>4), limit)
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
		if len(data)-index < 2 {
			return nil, fmt.Errorf("LZ4 truncated offset: %w", ErrMalformed)
		}
		offset := int(data[index]) | int(data[index+1])<<8
		index += 2
		if offset == 0 || offset > len(output) {
			return nil, fmt.Errorf("LZ4 invalid back-reference: %w", ErrMalformed)
		}
		length, err = lz4Length(data, &index, int(token&15), limit)
		if err != nil {
			return nil, err
		}
		if length > limit-len(output)-4 {
			return nil, fmt.Errorf("LZ4 match bounds: %w", ErrMalformed)
		}
		for range length + 4 {
			output = append(output, output[len(output)-offset])
		}
	}
	return nil, fmt.Errorf("LZ4 missing final token: %w", ErrMalformed)
}
