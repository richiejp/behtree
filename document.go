package behtree

import (
	"encoding/json"
	"fmt"
	"os"
)

type Document struct {
	Objects    []ObjectDef    `json:"objects,omitempty"`
	Interfaces []InterfaceDef `json:"interfaces,omitempty"`
	Behaviours []BehaviourDef `json:"behaviours,omitempty"`
	Tree       *Node          `json:"tree,omitempty"`
}

func ParseDocument(data []byte) (*Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse document: %w", err)
	}
	return &doc, nil
}

func LoadDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load document %s: %w", path, err)
	}
	return ParseDocument(data)
}

func MergeDocuments(docs ...*Document) *Environment {
	env := &Environment{}
	for _, doc := range docs {
		env.Objects = append(env.Objects, doc.Objects...)
		env.Interfaces = append(env.Interfaces, doc.Interfaces...)
		env.Behaviours = append(env.Behaviours, doc.Behaviours...)
		if doc.Tree != nil {
			env.Trees = append(env.Trees, doc.Tree)
		}
	}
	return env
}

func LoadEnvironment(paths ...string) (*Environment, error) {
	var docs []*Document
	for _, p := range paths {
		doc, err := LoadDocument(p)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return MergeDocuments(docs...), nil
}
