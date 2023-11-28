package parser

func New() (*parser, error) {
	return &parser{}, nil
}

type parser struct{}
