package cache

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/vmihailenco/msgpack/v5"
)

type Serializer interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
	Name() string
}

type JSONSerializer struct{}

func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

func (s *JSONSerializer) Marshal(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("cannot marshal nil value")
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal failed: %w", err)
	}

	return data, nil
}

func (s *JSONSerializer) Unmarshal(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("cannot unmarshal empty data")
	}

	if v == nil {
		return fmt.Errorf("cannot unmarshal into nil")
	}

	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("json unmarshal failed: %w", err)
	}

	return nil
}

func (s *JSONSerializer) Name() string {
	return "json"
}

type GobSerializer struct{}

func NewGobSerializer() *GobSerializer {
	return &GobSerializer{}
}

func (s *GobSerializer) Marshal(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("cannot marshal nil value")
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("gob encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (s *GobSerializer) Unmarshal(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("cannot unmarshal empty data")
	}

	if v == nil {
		return fmt.Errorf("cannot unmarshal into nil")
	}

	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)

	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("gob decode failed: %w", err)
	}

	return nil
}

func (s *GobSerializer) Name() string {
	return "gob"
}

type MsgpackSerializer struct{}

func NewMsgpackSerializer() *MsgpackSerializer {
	return &MsgpackSerializer{}
}

func (s *MsgpackSerializer) Marshal(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("cannot marshal nil value")
	}

	data, err := msgpack.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("msgpack marshal failed: %w", err)
	}

	return data, nil
}

func (s *MsgpackSerializer) Unmarshal(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("cannot unmarshal empty data")
	}

	if v == nil {
		return fmt.Errorf("cannot unmarshal into nil")
	}

	if err := msgpack.Unmarshal(data, v); err != nil {
		return fmt.Errorf("msgpack unmarshal failed: %w", err)
	}

	return nil
}

func (s *MsgpackSerializer) Name() string {
	return "msgpack"
}

func MarshalString(s string, serializer Serializer) ([]byte, error) {
	return []byte(s), nil
}

func UnmarshalString(data []byte) string {
	return string(data)
}

func MarshalInt(i int64, serializer Serializer) ([]byte, error) {
	return []byte(strconv.FormatInt(i, 10)), nil
}

func UnmarshalInt(data []byte) (int64, error) {
	return strconv.ParseInt(string(data), 10, 64)
}

func MarshalBool(b bool, serializer Serializer) ([]byte, error) {
	if b {
		return []byte("1"), nil
	}
	return []byte("0"), nil
}

func UnmarshalBool(data []byte) bool {
	return len(data) > 0 && data[0] == '1'
}

func MarshalStruct(v interface{}, serializer Serializer) ([]byte, error) {
	return serializer.Marshal(v)
}

func UnmarshalStruct(data []byte, v interface{}, serializer Serializer) error {
	return serializer.Unmarshal(data, v)
}

func GetSerializer(name string) Serializer {
	switch name {
	case "json":
		return NewJSONSerializer()
	case "gob":
		return NewGobSerializer()
	case "msgpack":
		return NewMsgpackSerializer()
	default:
		return NewJSONSerializer()
	}
}

func IsSerializable(v interface{}) bool {
	if v == nil {
		return false
	}

	t := reflect.TypeOf(v)
	k := t.Kind()

	switch k {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return false
	case reflect.Ptr:
		return IsSerializable(reflect.ValueOf(v).Elem().Interface())
	default:
		return true
	}
}

func SerializeValue(v interface{}, serializer Serializer) ([]byte, error) {
	if !IsSerializable(v) {
		return nil, fmt.Errorf("value is not serializable")
	}

	return serializer.Marshal(v)
}

func DeserializeValue(data []byte, target interface{}, serializer Serializer) error {
	return serializer.Unmarshal(data, target)
}
