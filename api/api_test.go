package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/snyk/snyk-code-review-exercise/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// review: it's also useful to have tests for different edge cases:
//   - package not found (404)
//   - version not found
//   - invalid version string
//   - circular dependency (if supported)
//
// idea: use table-driven tests to make these cases scalable and organized
// idea: extract fixture loading logic into helper function
// idea: use a mock client instead of hitting real registry (flaky tests)
// review: current test is flaky because dependency versions change on live npm registry

func TestPackageHandler(t *testing.T) {
	// review: this uses real HTTP client; inject mock for deterministic results
	handler := api.New()
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/package/react/16.13.0")
	require.Nil(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.Nil(t, err)

	var data api.NpmPackageVersion
	err = json.Unmarshal(body, &data)
	require.Nil(t, err)

	assert.Equal(t, "react", data.Name)
	assert.Equal(t, "16.13.0", data.Version)

	// review: fixture must stay up-to-date with the actual data returned by registry — fragile
	fixture, err := os.Open(filepath.Join("testdata", "react-16.13.0.json"))
	require.Nil(t, err)
	var fixtureObj api.NpmPackageVersion
	require.Nil(t, json.NewDecoder(fixture).Decode(&fixtureObj))

	// review: assert.Equal is not ideal for deep nested diffs; you can use cmp.Diff for clearer failure output
	assert.Equal(t, fixtureObj, data)
}
