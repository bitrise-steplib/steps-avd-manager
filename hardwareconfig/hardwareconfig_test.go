package hardwareconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/bitrise-io/go-utils/log"
	"github.com/stretchr/testify/require"
)

func TestConfigs(t *testing.T) {
	conf := New("test_emulator", "google_apis", "23", "portrait", "en-US", "", true)

	expectedDescriptor := defaultDescriptor

	expectedDescriptor += "\npath=" + filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".android/avd/%s.avd", conf.ID))
	expectedDescriptor += "\npath.rel=" + fmt.Sprintf("avd/%s.avd", conf.ID)
	expectedDescriptor += "\n\ntarget=" + fmt.Sprintf("android-%s", conf.Version)

	log.Infof("%s", conf.Descriptor)
	fmt.Println()
	log.Infof("%s", conf.Config)

	require.Equal(t, expectedDescriptor, conf.Descriptor.String())

}

func TestConfigs2(t *testing.T) {
	conf := New("test_emulator", "google_apis", "26", "portrait", "it-IT", "1080x1920", true)

	require.NoError(t, conf.Create())
	require.FailNow(t, "ok")
}

func TestPropertyList(t *testing.T) {
	proplist := &PropertyList{}
	proplist.SetProperty("key1", "val1")
	proplist.SetProperty("key2", "val2")
	proplist.SetProperty("key3", "val3")

	expected := &PropertyList{"key1=val1", "key2=val2", "key3=val3"}
	require.Equal(t, *expected, *proplist)

	proplist.SetProperty("key2", "valnew")

	expected = &PropertyList{"key1=val1", "key2=valnew", "key3=val3"}
	require.Equal(t, *expected, *proplist)

	proplist.SetProperty("key1", "")

	expected = &PropertyList{"key2=valnew", "key3=val3"}
	require.Equal(t, *expected, *proplist)

	proplist.SetProperty("key4", "test")

	expected = &PropertyList{"key2=valnew", "key3=val3", "key4=test"}
	require.Equal(t, *expected, *proplist)

	proplist.SetProperty("key5", "")

	expected = &PropertyList{"key2=valnew", "key3=val3", "key4=test"}
	require.Equal(t, *expected, *proplist)
}

func TestConfigs3(t *testing.T) {
	res, _ := ensureResolutionOrientation("100x200", "portrait")
	require.Equal(t, "100x200", res)

	res, _ = ensureResolutionOrientation("100x200", "landscape")
	require.Equal(t, "200x100", res)

	res, _ = ensureResolutionOrientation("200x100", "portrait")
	require.Equal(t, "100x200", res)

	res, _ = ensureResolutionOrientation("200x100", "landscape")
	require.Equal(t, "200x100", res)
}
