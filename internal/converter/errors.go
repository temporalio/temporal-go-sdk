package converter

import (
	"errors"
)

var (
	// ErrMetadataIsNotSet is returned when metadata is not set.
	ErrMetadataIsNotSet = errors.New("metadata is not set")
	// ErrEncodingIsNotSet is returned when payload encoding metadata is not set.
	ErrEncodingIsNotSet = errors.New("payload encoding metadata is not set")
	// ErrEncodingIsNotSupported is returned when payload encoding is not supported.
	ErrEncodingIsNotSupported = errors.New("payload encoding is not supported")
	// ErrUnableToEncode is returned when unable to encode.
	ErrUnableToEncode = errors.New("unable to encode")
	// ErrUnableToDecode is returned when unable to decode.
	ErrUnableToDecode = errors.New("unable to decode")
	// ErrUnableToSetValue is returned when unable to set value.
	ErrUnableToSetValue = errors.New("unable to set value")
	// ErrUnableToFindConverter is returned when unable to find converter.
	ErrUnableToFindConverter = errors.New("unable to find converter")
	// ErrValueDoesntImplementProtoMessage is returned when value doesn't implement proto.Message.
	ErrValueDoesntImplementProtoMessage = errors.New("value doesn't implement proto.Message")
	// ErrValueDoesntImplementProtoUnmarshaler is returned when value doesn't implement proto.Unmarshaler.
	ErrValueDoesntImplementProtoUnmarshaler = errors.New("value doesn't implement proto.Unmarshaler")
)
