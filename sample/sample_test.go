package sample

import (
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

const TEST_MESSAGE = `To: recipient@example.org
From: sender@example.org
Subject: test message

body line 1
body line 2
`

func TestSample(t *testing.T) {
	Init("rspam_learnd", Version, "testdata/config.yaml")
	class := "sample_class"
	username := os.Getenv("USER")
	domains := []string{"example.org"}
	filename := "testdata/sample"
	err := os.WriteFile(filename, []byte(TEST_MESSAGE), 0600)
	require.Nil(t, err)
	s, err := NewSample(class, username, domains, filename)
	require.Nil(t, err)
	require.NotNil(t, s)
}
