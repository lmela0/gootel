package gonfigurator

// Options abstract settings to choose the data fetcher
type Options interface {
	From() string
}

// EnvOption loads from os env
type EnvOption struct{}

// JsonOption loads from file
type JsonOption struct {
	Path string
}

func (o EnvOption) From() string {
	return "ENV"
}

func (o JsonOption) From() string {
	return "JSON"
}
