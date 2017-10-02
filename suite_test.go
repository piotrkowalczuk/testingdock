package testingdock_test

import (
	"os"
	"testing"

	"context"

	"time"

	"github.com/piotrkowalczuk/testingdock"
)

func TestMain(m *testing.M) {
	got := m.Run()

	testingdock.UnregisterAll()

	os.Exit(got)
}

func TestGetOrCreateSuite(t *testing.T) {
	s1 := testingdock.GetOrCreateSuite(t, "TestGetOrCreateSuite", testingdock.SuiteOpts{})
	s2 := testingdock.GetOrCreateSuite(t, "TestGetOrCreateSuite", testingdock.SuiteOpts{})
	s3 := testingdock.GetOrCreateSuite(t, "TestGetOrCreateSuite", testingdock.SuiteOpts{
		Timeout: 10 * time.Second,
	})

	s1.Reset(context.TODO())
	s2.Reset(context.TODO())
	s3.Reset(context.TODO())

	testingdock.UnregisterAll()
}
