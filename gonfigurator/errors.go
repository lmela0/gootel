package gonfigurator

import "errors"

var (
	ErrOptNotSupported error = errors.New("opts not supported")
	ErrStructNotValid  error = errors.New("struct not valid")
	ErrKeyNotFound     error = errors.New("key not found")
)
