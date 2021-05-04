package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLine(t *testing.T) {
	status, err := parseLine([]byte("C1.23,068,120,054,0820,1"))
	require.NoError(t, err)

	assert.Equal(t, coffee, status.mode)
	assert.Equal(t, "1.23", status.version)
	assert.Equal(t, uint16(68), status.steamTemp)
	assert.Equal(t, uint16(120), status.steamTargetTemp)
	assert.Equal(t, uint16(54), status.hxTemp)
	assert.Equal(t, uint16(820), status.readyCountdown)
	assert.Equal(t, true, status.heating)
}
