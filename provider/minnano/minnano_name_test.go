package minnano

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTitleNames(t *testing.T) {
	cases := []struct {
		raw    string
		span   string
		name   string
		alises []string
	}{
		{
			raw:    "柚井初美 / Yui Hatsumi",
			span:   "Yui Hatsumi",
			name:   "柚井初美",
			alises: []string{"Hatsumi Yui"},
		},
		{
			raw:  "柚井初美",
			name: "柚井初美",
		},
		{
			raw:    "柚井初美Yui Hatsumi",
			span:   "Yui Hatsumi",
			name:   "柚井初美",
			alises: []string{"Hatsumi Yui"},
		},
		{
			raw:    "柚井初美 / ゆい はつみ / Yui Hatsumi",
			span:   "ゆい はつみ / Yui Hatsumi",
			name:   "柚井初美",
			alises: []string{"Hatsumi Yui"},
		},
	}

	for _, c := range cases {
		name, aliases := parseTitleNames(c.raw, c.span)
		assert.Equal(t, c.name, name, "raw: %q", c.raw)
		assert.Equal(t, c.alises, aliases, "raw: %q", c.raw)
	}
}

func TestNormalizeRomaji(t *testing.T) {
	assert.Equal(t, "Hatsumi Yui", normalizeRomaji("Yui Hatsumi"))
	assert.Equal(t, "Yui", normalizeRomaji(" yui "))
	assert.Equal(t, "", normalizeRomaji("柚井初美"))
}
