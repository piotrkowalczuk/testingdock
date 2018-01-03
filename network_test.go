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
	s.Network(testingdock.NetworkOpts{Name: "TestNetwork_Start_1"})
	s.Network(testingdock.NetworkOpts{Name: "TestNetwork_Start_2"})

	s.Start(context.TODO())

	if err := s.Close(); err != nil {
		t.Fatalf("Failed to close a network: %s", err.Error())
	}
}
