package avdmanager

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
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-steplib/steps-avd-manager/avdconfig"
)

// AVD ...
type AVD struct {
	ID                   string
	Tag                  string
	Version              string
	Orientation          string
	Resolution           string
	Locale               string
	Density              string
	ConfigProperties     *avdconfig.Properties
	DescriptorProperties *avdconfig.Properties
}

// New ...
func New(id, tag, version, orientation, locale, resolution, density string) (*AVD, error) {
	avd := &AVD{
		ID:          id,
		Tag:         tag,
		Orientation: orientation,
		Version:     version,
		Locale:      locale,
		Resolution:  resolution,
		Density:     density,
	}

	defaultConfigPropertiesSlice := strings.Split(defaultConfigProperties, "\n")

	props, err := avdconfig.NewProperties(defaultConfigPropertiesSlice)
	if err != nil {
		return nil, err
	}

	avd.ConfigProperties = &props

	avd.ConfigProperties.Apply("AvdId", id)
	avd.ConfigProperties.Apply("tag.id", tag)
	avd.ConfigProperties.Apply("tag.display", tag)
	avd.ConfigProperties.Apply("skin.name", resolution)
	avd.ConfigProperties.Apply("hw.lcd.density", density)
	avd.ConfigProperties.Apply("avd.ini.displayname", id)
	avd.ConfigProperties.Apply("hw.initialOrientation", strings.Title(orientation))
	avd.ConfigProperties.Apply("PlayStore.enabled", fmt.Sprintf("%t", tag == "google_apis_playstore"))
	avd.ConfigProperties.Apply("image.sysdir.1", fmt.Sprintf("system-images/android-%s/%s/x86/", version, tag))

	defaultDescriptorPropertiesSlice := strings.Split(defaultDescriptorProperties, "\n")

	descriptorProps, err := avdconfig.NewProperties(defaultDescriptorPropertiesSlice)
	if err != nil {
		return nil, err
	}

	avd.DescriptorProperties = &descriptorProps

	avd.DescriptorProperties.Apply("path", filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".android/avd/%s.avd", id)))
	avd.DescriptorProperties.Apply("path.rel", fmt.Sprintf("avd/%s.avd", id))
	avd.DescriptorProperties.Apply("target", fmt.Sprintf("android-%s", version))

	return avd, nil
}

// Create ...
func (avd *AVD) Create() error {
	androidHome := os.Getenv("ANDROID_HOME")
	avdPath := avd.DescriptorProperties.Get("path")
	descriptorPath := strings.TrimSuffix(avdPath, ".avd") + ".ini"
	configPath := filepath.Join(avdPath, "config.ini")
	sdcardPath := filepath.Join(avdPath, "sdcard.img")
	sdcardSize := avd.ConfigProperties.Get("sdcard.size")
	encryptionKeyTargetPath := filepath.Join(avdPath, "encryptionkey.img")
	systemTargetPath := filepath.Join(avdPath, "system.img")
	userDataTargetPath := filepath.Join(avdPath, "userdata.img")
	userDataSourcePath := filepath.Join(androidHome, avd.ConfigProperties.Get("image.sysdir.1"), "userdata.img")
	encryptionKeySourcePath := filepath.Join(androidHome, avd.ConfigProperties.Get("image.sysdir.1"), "encryptionkey.img")
	systemSourcePath := filepath.Join(androidHome, avd.ConfigProperties.Get("image.sysdir.1"), "system.img")

	// ensure width and height are matching orientation
	{
		if avd.ConfigProperties.Get("skin.name") != "" {
			res, err := ensureResolutionOrientation(avd.Resolution, avd.Orientation)
			if err != nil {
				return err
			}

			avd.ConfigProperties.Apply("skin.name", res)
		}
	}

	if err := os.MkdirAll(avdPath, 0777); err != nil {
		return err
	}

	if err := fileutil.WriteStringToFile(descriptorPath, avd.DescriptorProperties.String()); err != nil {
		return err
	}

	if err := fileutil.WriteStringToFile(configPath, avd.ConfigProperties.String()); err != nil {
		return err
	}

	if err := copyFile(userDataSourcePath, userDataTargetPath); err != nil {
		return err
	}

	if out, err := command.New(filepath.Join(androidHome, "tools/mksdcard"), "-l", "SDCARD", sdcardSize, sdcardPath).RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("error: %s, output: %s", err, out)
	}

	version, err := strconv.Atoi(avd.Version)
	if err != nil {
		return err
	}

	if avd.Locale != "en-US" && version >= 23 {
		if err := copyFile(systemSourcePath, systemTargetPath); err != nil {
			return err
		}

		if b, err := pathutil.IsPathExists(encryptionKeySourcePath); err == nil && b {
			if err := copyFile(encryptionKeySourcePath, encryptionKeyTargetPath); err != nil {
				return err
			}
		}

		data, err := fileutil.ReadBytesFromFile(systemTargetPath)
		if err != nil {
			return err
		}

		data = bytes.Replace(data, []byte("ro.product.locale=en-US"), []byte(fmt.Sprintf("ro.product.locale=%s", avd.Locale)), 1)

		err = fileutil.WriteBytesToFile(systemTargetPath, data)
		if err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer func() {
		if err := in.Close(); err != nil {
			log.Errorf("Failed to close file, error: %s", err)
		}
	}()
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

const defaultDescriptorProperties = `avd.ini.encoding=UTF-8`

const defaultConfigProperties = `avd.ini.encoding=UTF-8
abi.type=x86
disk.dataPartition.size=800M
hw.accelerometer=yes
hw.audioInput=yes
hw.battery=no
hw.camera.back=emulated
hw.camera.front=emulated
hw.cpu.arch=x86
hw.cpu.ncore=2
hw.dPad=no
hw.gps=yes
hw.keyboard=yes
hw.lcd.density=320
hw.gpu.enabled=true
hw.gpu.mode=swiftshader
hw.mainKeys=no
hw.device.hash2=MD5:bc5032b2a871da511332401af3ac6bb0
hw.device.manufacturer=Google
hw.device.name=Nexus 5X
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
fastboot.forceColdBoot=no
skin.name=720x1280
skin.path=_no_skin
skin.path.backup=_no_skin
PlayStore.enabled=false
tag.display=Google APIs
tag.id=google_apis
vm.heapSize=256`
