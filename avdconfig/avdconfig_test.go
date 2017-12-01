package avdconfig

import (
	"path/filepath"
	"testing"

	"github.com/bitrise-io/go-utils/fileutil"

	"github.com/stretchr/testify/require"

	"github.com/bitrise-io/go-utils/pathutil"
)

func TestParse(t *testing.T) {
	tempDir, err := pathutil.NormalizedOSTempDirPath("_TEST_")
	require.NoError(t, err)

	configPath := filepath.Join(tempDir, "config.ini")

	testContent := "test.key.1=test value 1\ntest.key.2=test value 2\ntest.key.3=test value 3\ntest.key.4=test value 4"

	require.NoError(t, fileutil.WriteStringToFile(configPath, testContent))

	config, err := Parse(configPath)
	require.NoError(t, err)
	require.Equal(t, testContent, config.Properties.String())

	testContent = "test.key.1=test value 1\ntest.key.2=test value test\ntest.key.3=test value 3\ntest.key.4=test value 4"

	config.Properties.Apply("test.key.2", "test value test")
	require.Equal(t, testContent, config.Properties.String())

	testContent = "test.key.2=test value test\ntest.key.3=test value 3\ntest.key.4=test value 4"

	config.Properties.Apply("test.key.1", "")
	require.Equal(t, testContent, config.Properties.String())
}
