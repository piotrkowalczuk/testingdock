package testingdock_test

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/piotrkowalczuk/testingdock"
)

func TestMain(m *testing.M) {
	flag.Parse()

	got := m.Run()

	testingdock.UnregisterAll()

	os.Exit(got)
}

func TestGetOrCreateSuite(t *testing.T) {
	s1, ok1 := testingdock.GetOrCreateSuite(t, "TestGetOrCreateSuite", testingdock.SuiteOpts{})
	s2, ok2 := testingdock.GetOrCreateSuite(t, "TestGetOrCreateSuite", testingdock.SuiteOpts{})

	s1.Reset(context.TODO())
	s2.Reset(context.TODO())

	if ok1 {
		t.Error("first call should create new suite")
	}
	if !ok2 {
		t.Error("second call should return suite from registry")
	}

	testingdock.UnregisterAll()
}
