# testingdock [![GoDoc](https://godoc.org/github.com/piotrkowalczuk/testingdock?status.svg)](http://godoc.org/github.com/piotrkowalczuk/testingdock)&nbsp;[![Build Status](https://travis-ci.org/piotrkowalczuk/testingdock.svg?branch=master)](https://travis-ci.org/piotrkowalczuk/testingdock)&nbsp;[![codecov.io](https://codecov.io/github/piotrkowalczuk/testingdock/coverage.svg?branch=master)](https://codecov.io/github/piotrkowalczuk/testingdock?branch=master)&nbsp;[![Code Climate](https://codeclimate.com/github/piotrkowalczuk/testingdock/badges/gpa.svg)](https://codeclimate.com/github/piotrkowalczuk/testingdock)&nbsp;[![Go Report Card](https://goreportcard.com/badge/github.com/piotrkowalczuk/testingdock)](https://goreportcard.com/report/github.com/piotrkowalczuk/testingdock)

Simple helper library for integration testing with docker in a programmatical way.

## Example

See the [Container test suite](./container_test.go).

## Notes

This library will create networks and containers under the label `owner=testingdock`.
Containers and networks with this label will be considered to have been started by this library
and may be subject to aggressive manipulation and cleanup.
