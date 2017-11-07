package hardwareconfig

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bitrise-io/depman/pathutil"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
)

const defaultConfig = `avd.ini.encoding=UTF-8
abi.type=x86
disk.dataPartition.size=800M
hw.accelerometer=yes
hw.audioInput=yes
hw.battery=yes
hw.camera.back=emulated
hw.camera.front=emulated
hw.cpu.arch=x86
hw.cpu.ncore=2
hw.dPad=no
hw.gps=yes
hw.gpu.enabled=no
hw.gpu.mode=off
hw.keyboard=yes
hw.lcd.density=420
hw.mainKeys=no
hw.ramSize=1536
hw.sdCard=yes
hw.sensors.orientation=yes
hw.sensors.proximity=yes
hw.trackBall=no
runtime.network.latency=none
runtime.network.speed=full
sdcard.size=512M
showDeviceFrame=no
skin.dynamic=yes
skin.name=1080x1920
skin.path=_no_skin
skin.path.backup=_no_skin
vm.heapSize=384`

const defaultDescriptor = `avd.ini.encoding=UTF-8`

// PropertyList ...
type PropertyList []string

// HWConfig ...
type HWConfig struct {
	ID          string
	Tag         string
	Version     string
	Orientation string
	Resolution  string
	Locale      string
	Overwrite   bool
	Config      *PropertyList
	Descriptor  *PropertyList
}

// New ...
func New(id, tag, version, orientation, locale, resolution string, overwrite bool) *HWConfig {
	hwConfig := &HWConfig{
		ID:          id,
		Tag:         tag,
		Orientation: orientation,
		Version:     version,
		Locale:      locale,
		Resolution:  resolution,
		Overwrite:   overwrite,
	}

	defaultConfig := PropertyList(strings.Split(defaultConfig, "\n"))
	hwConfig.Config = &defaultConfig

	hwConfig.Config.SetProperty("AvdId", id)
	hwConfig.Config.SetProperty("avd.ini.displayname", id)
	hwConfig.Config.SetProperty("tag.id", tag)
	hwConfig.Config.SetProperty("tag.display", tag)
	hwConfig.Config.SetProperty("PlayStore.enabled", fmt.Sprintf("%t", tag == "google_apis_playstore"))
	hwConfig.Config.SetProperty("image.sysdir.1", fmt.Sprintf("system-images/android-%s/%s/x86/", version, tag))
	hwConfig.Config.SetProperty("hw.initialOrientation", strings.Title(orientation))
	hwConfig.Config.SetProperty("skin.name", resolution)

	defaultDescriptor := PropertyList(strings.Split(defaultDescriptor, "\n"))
	hwConfig.Descriptor = &defaultDescriptor

	hwConfig.Descriptor.SetProperty("path", filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".android/avd/%s.avd", id)))
	hwConfig.Descriptor.SetProperty("path.rel", fmt.Sprintf("avd/%s.avd", id))
	hwConfig.Descriptor.SetProperty("target", fmt.Sprintf("android-%s", version))

	return hwConfig
}

// String ...
func (props *PropertyList) String() string {
	return strings.Join(*props, "\n")
}

// SetProperty ...
func (props *PropertyList) SetProperty(key, value string) {
	overwritten := false
	for i, line := range *props {
		if !strings.HasPrefix(strings.TrimSpace(line), key) {
			(*props)[i] = line
		} else {
			overwritten = true
			(*props)[i] = fmt.Sprintf("%s=%s", key, value)
		}
	}

	if !overwritten {
		(*props) = append((*props), fmt.Sprintf("%s=%s", key, value))
	}
}

// GetProperty ...
func (props *PropertyList) GetProperty(key string) string {
	for _, line := range *props {
		if strings.HasPrefix(strings.TrimSpace(line), key) {
			return strings.Join(strings.Split(line, "=")[1:], "=")
		}
	}

	return ""
}

// Create ...
func (hwConfig *HWConfig) Create() error {
	androidHome := os.Getenv("ANDROID_HOME")
	avdPath := hwConfig.Descriptor.GetProperty("path")
	descriptorPath := strings.TrimSuffix(avdPath, ".avd") + ".ini"
	configPath := filepath.Join(avdPath, "config.ini")
	sdcardPath := filepath.Join(avdPath, "sdcard.img")
	sdcardSize := hwConfig.Config.GetProperty("sdcard.size")
	encryptionKeyTargetPath := filepath.Join(avdPath, "encryptionkey.img")
	systemTargetPath := filepath.Join(avdPath, "system.img")
	userDataTargetPath := filepath.Join(avdPath, "userdata.img")
	userDataSourcePath := filepath.Join(androidHome, hwConfig.Config.GetProperty("image.sysdir.1"), "userdata.img") //encryptionkey.img
	encryptionKeySourcePath := filepath.Join(androidHome, hwConfig.Config.GetProperty("image.sysdir.1"), "encryptionkey.img")
	systemSourcePath := filepath.Join(androidHome, hwConfig.Config.GetProperty("image.sysdir.1"), "system.img")

	res, err := ensureResolutionOrientation(hwConfig.Resolution, hwConfig.Orientation)
	if err != nil {
		return err
	}

	hwConfig.Config.SetProperty("skin.name", res)

	if err := os.MkdirAll(avdPath, 0777); err != nil {
		return err
	}

	if err := fileutil.WriteStringToFile(descriptorPath, hwConfig.Descriptor.String()); err != nil {
		return err
	}

	if err := fileutil.WriteStringToFile(configPath, hwConfig.Config.String()); err != nil {
		return err
	}

	if err := copyFile(userDataSourcePath, userDataTargetPath); err != nil {
		return err
	}

	if err := copyFile(systemSourcePath, systemTargetPath); err != nil {
		return err
	}

	version, err := strconv.Atoi(hwConfig.Version)
	if err != nil {
		return err
	}

	if hwConfig.Locale != "en-US" {
		if version >= 23 {
			data, err := fileutil.ReadBytesFromFile(systemTargetPath)
			if err != nil {
				return err
			}

			data = bytes.Replace(data, []byte("ro.product.locale=en-US"), []byte(fmt.Sprintf("ro.product.locale=%s", hwConfig.Locale)), -1)

			err = fileutil.WriteBytesToFile(systemTargetPath, data)
			if err != nil {
				return err
			}

			if b, err := pathutil.IsPathExists(encryptionKeySourcePath); err == nil && b {
				if err := copyFile(encryptionKeySourcePath, encryptionKeyTargetPath); err != nil {
					return err
				}
			}
		}
	}

	if out, err := command.New(filepath.Join(androidHome, "tools/mksdcard"), "-l", "SDCARD", sdcardSize, sdcardPath).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("error: %s, output: %s", err, out)
	}

	return nil
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	buf := make([]byte, 4096)

	if _, err = io.CopyBuffer(out, in, buf); err != nil {
		return
	}
	err = out.Sync()
	return
}

func ensureResolutionOrientation(res, orientation string) (string, error) {
	sides := strings.Split(res, "x")

	if len(sides) != 2 {
		return "", fmt.Errorf("Invalid resolution format: %s", res)
	}

	a, err := strconv.Atoi(sides[0])
	if err != nil {
		return "", err
	}

	b, err := strconv.Atoi(sides[1])
	if err != nil {
		return "", err
	}

	if strings.ToLower(orientation) == "portrait" {
		if a < b {
			return fmt.Sprintf("%dx%d", a, b), nil
		}
		return fmt.Sprintf("%dx%d", b, a), nil
	}

	if a > b {
		return fmt.Sprintf("%dx%d", a, b), nil
	}
	return fmt.Sprintf("%dx%d", b, a), nil
}
