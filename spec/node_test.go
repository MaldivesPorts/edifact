package spec_test

import (
	"testing"

	"github.com/MaldivesPorts/edifact/spec"
)

func TestTransition(t *testing.T) {

	expected := []string{"UNH", "BGM", "RFF", "RFF", "NAD"}

	var err error
	node := spec.Get("DESADV")
	for i, exp := range expected {
		node, err = node.Transition(exp)
		if err != nil {
			t.Fatal(err)
		}
		if exp != node.Tag {
			t.Errorf("%d: %s expected, was %s", i, exp, node.Tag)
		}
	}
}
