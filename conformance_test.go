package xrr_test

import (
	"os"
	"path/filepath"
	"testing"

	xrr "hop.top/xrr"
	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type manifest struct {
	Interactions []struct {
		Adapter     string `yaml:"adapter"`
		Fingerprint string `yaml:"fingerprint"`
	} `yaml:"interactions"`
}

// TestConformanceFixtures replays spec/fixtures cassettes — proves Go can read
// cassettes produced by any other language port.
func TestConformanceFixtures(t *testing.T) {
	fixtures := filepath.Join("..", "spec", "fixtures")
	entries, err := os.ReadDir(fixtures)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "no fixture dirs found")

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		e := e
		t.Run(e.Name(), func(t *testing.T) {
			dir := filepath.Join(fixtures, e.Name())
			manifestPath := filepath.Join(dir, "manifest.yaml")
			data, err := os.ReadFile(manifestPath)
			require.NoError(t, err, "missing manifest.yaml in %s", e.Name())

			var m manifest
			require.NoError(t, yaml.Unmarshal(data, &m))

			c := xrr.NewFileCassette(dir)
			for _, interaction := range m.Interactions {
				var reqPayload, respPayload map[string]any
				_, err := c.Load(interaction.Adapter, interaction.Fingerprint, &reqPayload, &respPayload)
				assert.NoError(t, err,
					"cassette miss: adapter=%s fp=%s", interaction.Adapter, interaction.Fingerprint)
			}
		})
	}
}
