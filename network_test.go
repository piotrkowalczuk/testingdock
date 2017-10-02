package testingdock_test

import (
	"testing"

	"context"

	"github.com/piotrkowalczuk/testingdock"
)

func TestNetwork_Start(t *testing.T) {
	s, ok := testingdock.GetOrCreateSuite(t, "TestNetwork_Start", testingdock.SuiteOpts{})
	if ok {
		t.Fatal("this suite should not exists yet")
	}
	n1 := s.Network(testingdock.NetworkOpts{Name: "TestNetwork_Start_1"})
	n2 := s.Network(testingdock.NetworkOpts{Name: "TestNetwork_Start_2"})

	n1.Start(context.TODO())
	n2.Start(context.TODO())
}
