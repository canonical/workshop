package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"

	"gopkg.in/check.v1"

	"github.com/canonical/workshop/internal/dirs"
	"github.com/canonical/workshop/internal/testutil"
)

type BaseWorkshopSuite struct {
	testutil.BaseTest
	stdin  *bytes.Buffer
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (s *BaseWorkshopSuite) SetUpTest(c *check.C) {
	dirs.SetRootDir(c.MkDir())

	path := os.Getenv("PATH")
	s.AddCleanup(func() {
		os.Setenv("PATH", path)
	})

	s.stdin = bytes.NewBuffer(nil)
	s.stdout = bytes.NewBuffer(nil)
	s.stderr = bytes.NewBuffer(nil)

	Stdin = s.stdin
	Stdout = s.stdout
	Stderr = s.stderr
}

func (s *BaseWorkshopSuite) TearDownTest(c *check.C) {
	Stdin = os.Stdin
	Stdout = os.Stdout
	Stderr = os.Stderr
}

func (s *BaseWorkshopSuite) RedirectClientToTestServer(handler func(http.ResponseWriter, *http.Request)) {
	server := httptest.NewServer(http.HandlerFunc(handler))
	s.BaseTest.AddCleanup(func() { server.Close() })
	ClientConfig.BaseURL = server.URL
	s.BaseTest.AddCleanup(func() { ClientConfig.BaseURL = "" })
}

func (s *BaseWorkshopSuite) ResetStdStreams() {
	s.stdin.Reset()
	s.stdout.Reset()
	s.stderr.Reset()
}

func (s *BaseWorkshopSuite) Stdout() string {
	return s.stdout.String()
}

func (s *BaseWorkshopSuite) Stderr() string {
	return s.stderr.String()
}

// EncodeResponseBody writes JSON-serialized body to the response writer.
func EncodeResponseBody(c *check.C, w http.ResponseWriter, body interface{}) {
	encoder := json.NewEncoder(w)
	err := encoder.Encode(body)
	c.Assert(err, check.IsNil)
}

// DecodedRequestBody returns the JSON-decoded body of the request.
func DecodedRequestBody(c *check.C, r *http.Request) map[string]interface{} {
	var body map[string]interface{}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	err := decoder.Decode(&body)
	c.Assert(err, check.IsNil)
	return body
}
