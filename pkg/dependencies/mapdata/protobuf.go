package mapdata

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	maxProtoFields   = 500_000
	maxProtoMessages = 65_536
	maxProtoDepth    = 32
)

type protoBudget struct {
	fields   int
	messages int
}

func (budget *protoBudget) inspect(data []byte, descriptor protoreflect.MessageDescriptor, depth int) error {
	if depth > maxProtoDepth || budget.messages >= maxProtoMessages {
		return fmt.Errorf("protobuf message bound: %w", ErrMalformed)
	}
	budget.messages++
	for len(data) > 0 {
		budget.fields++
		if budget.fields > maxProtoFields {
			return fmt.Errorf("protobuf field bound: %w", ErrMalformed)
		}
		number, wireType, tagLength := protowire.ConsumeTag(data)
		if tagLength < 0 {
			return fmt.Errorf("protobuf tag: %w", ErrMalformed)
		}
		data = data[tagLength:]
		length := protowire.ConsumeFieldValue(number, wireType, data)
		if length < 0 {
			return fmt.Errorf("protobuf field length: %w", ErrMalformed)
		}
		field := descriptor.Fields().ByNumber(number)
		if field != nil && field.Kind() == protoreflect.MessageKind && wireType == protowire.BytesType {
			inner, _ := protowire.ConsumeBytes(data)
			if err := budget.inspect(inner, field.Message(), depth+1); err != nil {
				return err
			}
		}
		data = data[length:]
	}
	return nil
}
