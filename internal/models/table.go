package models

type Graph struct {
	Name  string
	Nodes []Node
	Edges []Edge
}

type Node struct {
	Name   string
	Host   string
	Inputs []string
	Output string
}

type Edge struct {
	Source      string
	Destination string
	Conditions  []string
}
