package server

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestServer(t *testing.T) {
	Init("rspam-learnd", Version, "testdata/config.yaml")
	s, err := NewServer()
	require.Nil(t, err)
	require.NotNil(t, s)
	require.True(t, s.debug)
	require.True(t, s.verbose)
}
