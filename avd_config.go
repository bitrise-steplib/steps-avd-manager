package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
)

const (
	PlayStoreEnabledKey       = "PlayStore.enabled"
	PlayStoreEnabledTrueValue = "true"

	ImageSysDir1Key         = "image.sysdir.1"
	PlayStoreTagPathElement = "google_apis_playstore"

	TagDisplayKey             = "tag.display"
	TagDisplayGooglePlayValue = "Google Play"

	TagIDKey                      = "tag.id"
	TagIDGoogleAPIsPlayStoreValue = "google_apis_playstore"
)

func ensureGooglePlay(avdName string) error {
	if err := ensureGooglePlayInConfigIni(avdName); err != nil {
		return err
	}

	if err := ensureGooglePlayInHardwareQemuIni(avdName); err != nil {
		var pthErr *os.PathError
		if errors.As(err, &pthErr) {
			log.Warnf("hardware-qemu.ini doesn't exist")
			return nil
		}
		return err
	}

	return nil
}

func ensureGooglePlayInHardwareQemuIni(avdName string) error {
	configIniPth := filepath.Join(avdConfigsDir(avdName), "hardware-qemu.ini")

	file, err := os.Open(configIniPth)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Warnf("failed to close %s: %s", configIniPth, err)
		}
	}()

	config, err := parseAVDConfiguration(file)
	if err != nil {
		return err
	}

	updated := false

	if value := config[PlayStoreEnabledKey]; value != PlayStoreEnabledTrueValue {
		log.Warnf("PlayStore is disable in config.ini, updating %s...", configIniPth)
		log.Warnf("%s=%s -> %s=%s", PlayStoreEnabledKey, value, PlayStoreEnabledKey, PlayStoreEnabledTrueValue)
		config[PlayStoreEnabledKey] = PlayStoreEnabledTrueValue
		updated = true
	}

	for _, key := range []string{
		"kernel.path",
		"disk.ramdisk.path",
		"disk.systemPartition.initPath",
		"disk.vendorPartition.initPath",
	} {
		// kernel.path = /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//kernel-ranchu
		// disk.ramdisk.path = /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//ramdisk.img
		// disk.systemPartition.initPath = /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//system.img
		// disk.vendorPartition.initPath = /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//vendor.img
		value, ok := config[key]
		if ok {
			if tag := getSystemImageTagPathElement(value); tag != PlayStoreTagPathElement {
				log.Warnf("'%s' system image is used instead of '%s' in hardware-qemu.ini, updating %s...", getSystemImageTagPathElement(value), PlayStoreTagPathElement, configIniPth)
				log.Warnf("%s=%s -> %s=%s", key, value, key, replaceSystemImageTagPathElement(value, PlayStoreTagPathElement))
				config[key] = replaceSystemImageTagPathElement(value, PlayStoreTagPathElement)
			}
		}
	}

	if updated {
		if err := os.Remove(configIniPth); err != nil {
			return err
		}

		newCont := createAVDConfigurationFileContent(config)
		if err := fileutil.WriteStringToFile(configIniPth, newCont); err != nil {
			return err
		}
	}

	return nil
}

func ensureGooglePlayInConfigIni(avdName string) error {
	configIniPth := filepath.Join(avdConfigsDir(avdName), "config.ini")

	file, err := os.Open(configIniPth)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Warnf("failed to close %s: %s", configIniPth, err)
		}
	}()

	config, err := parseAVDConfiguration(file)
	if err != nil {
		return err
	}

	updated := false

	if value := config[PlayStoreEnabledKey]; value != PlayStoreEnabledTrueValue {
		log.Warnf("PlayStore is disable in config.ini, updating %s...", configIniPth)
		log.Warnf("%s=%s -> %s=%s", PlayStoreEnabledKey, value, PlayStoreEnabledKey, PlayStoreEnabledTrueValue)
		config[PlayStoreEnabledKey] = PlayStoreEnabledTrueValue
		updated = true
	}

	if value := config[ImageSysDir1Key]; getImageSysDir1TagPathElement(value) != PlayStoreTagPathElement {
		log.Warnf("'%s' system image is used instead of '%s' in config.ini, updating %s...", getImageSysDir1TagPathElement(value), PlayStoreTagPathElement, configIniPth)
		log.Warnf("%s=%s -> %s=%s", ImageSysDir1Key, value, ImageSysDir1Key, replaceImageSysDir1TagPathElement(value, PlayStoreTagPathElement))
		config[ImageSysDir1Key] = replaceImageSysDir1TagPathElement(value, PlayStoreTagPathElement)
		updated = true
	}

	if value := config[TagDisplayKey]; value != TagDisplayGooglePlayValue {
		log.Warnf("'%s' tag.display is used instead of '%s' in config.ini, updating %s...", value, TagDisplayGooglePlayValue, configIniPth)
		log.Warnf("%s=%s -> %s=%s", TagDisplayKey, value, TagDisplayKey, TagDisplayGooglePlayValue)
		config[TagDisplayKey] = TagDisplayGooglePlayValue
		updated = true
	}

	if value := config[TagIDKey]; value != TagIDGoogleAPIsPlayStoreValue {
		log.Warnf("'%s' tag.id is used instead of '%s' in config.ini, updating %s...", value, TagIDGoogleAPIsPlayStoreValue, configIniPth)
		log.Warnf("%s=%s -> %s=%s", TagIDKey, value, TagIDKey, TagIDGoogleAPIsPlayStoreValue)
		config[TagIDKey] = TagIDGoogleAPIsPlayStoreValue
		updated = true
	}

	if updated {
		if err := os.Remove(configIniPth); err != nil {
			return err
		}

		newCont := createAVDConfigurationFileContent(config)
		if err := fileutil.WriteStringToFile(configIniPth, newCont); err != nil {
			return err
		}
	}

	return nil
}

func avdConfigsDir(avdName string) string {
	return filepath.Join(pathutil.UserHomeDir(), ".android/avd", avdName+".avd")
}

func parseAVDConfiguration(reader io.Reader) (map[string]string, error) {
	config := map[string]string{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		split := strings.Split(line, "=")
		if len(split) < 2 {
			return nil, fmt.Errorf("couldn't parse config line: %s", line)
		}

		key := strings.TrimSpace(split[0])
		value := strings.TrimSpace(strings.Join(split[1:], "="))

		config[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return config, nil
}

func getSystemImageTagPathElement(systemImagePath string) string {
	// /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//kernel-ranchu
	// /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//ramdisk.img
	// /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//system.img
	// /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//vendor.img
	systemImagePathUpToABI := filepath.Dir(systemImagePath)
	systemImagePathUpToTag := filepath.Dir(systemImagePathUpToABI)
	tagElement := filepath.Base(systemImagePathUpToTag)
	return tagElement
}

func replaceSystemImageTagPathElement(systemImagePath, newTagPathElement string) string {
	// // /Users/bitrise/Library/Android/sdk/system-images/android-30/google_apis_playstore/arm64-v8a//kernel-ranchu
	systemImagePathUpToABI := filepath.Dir(systemImagePath)
	systemImagePathUpToTag := filepath.Dir(systemImagePathUpToABI)
	systemImagePathUpToAPILevel := filepath.Dir(systemImagePathUpToTag)

	abiElement := filepath.Base(systemImagePathUpToABI)
	lastElement := filepath.Base(systemImagePath)

	newSystemImagePath := filepath.Join(systemImagePathUpToAPILevel, newTagPathElement, abiElement, lastElement)
	return newSystemImagePath
}

func getImageSysDir1TagPathElement(imageSysDirPth string) string {
	// system-images/android-26/google_apis/x86/
	// system-images/android-30/google_apis_playstore/arm64-v8a/
	imageSysDirPth = strings.TrimSuffix(imageSysDirPth, string(os.PathSeparator))
	tagElement := filepath.Base(filepath.Dir(imageSysDirPth))
	return tagElement
}

func replaceImageSysDir1TagPathElement(imageSysDirPth, newTagPathElement string) string {
	// system-images/android-26/google_apis/x86/
	abiElement := filepath.Base(imageSysDirPth)
	pathUpToAPILevel := filepath.Dir(filepath.Dir(imageSysDirPth))

	newImageSysDirPth := filepath.Join(pathUpToAPILevel, newTagPathElement, abiElement)
	if !strings.HasSuffix(newImageSysDirPth, string(os.PathSeparator)) {
		newImageSysDirPth += string(os.PathSeparator)
	}
	return newImageSysDirPth
}

func createAVDConfigurationFileContent(config map[string]string) string {
	cont := ""
	for key, value := range config {
		cont += key + "=" + value + "\n"
	}
	return cont
}
