package main

import (
	"bufio"
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

	ImageSysDir1Key                     = "image.sysdir.1"
	ImageSysDir1PlayStoreTagPathElement = "google_apis_playstore"

	TagDisplayKey             = "tag.display"
	TagDisplayGooglePlayValue = "Google Play"

	TagIDKey                      = "tag.id"
	TagIDGoogleAPIsPlayStoreValue = "google_apis_playstore"
)

func ensureGooglePlay(avdName, tag string) error {
	configIniPth := filepath.Join(pathutil.UserHomeDir(), ".android/avd", avdName+".avd", "config.ini")

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
		log.Warnf("Using %s tag, but PlayStore is disable in config.ini, updating %s...", tag, configIniPth)
		config[PlayStoreEnabledKey] = PlayStoreEnabledTrueValue
		updated = true
	}

	if value := config[ImageSysDir1Key]; getImageSysDir1TagPathElement(value) != ImageSysDir1PlayStoreTagPathElement {
		log.Warnf("Using %s tag, but '%s' system image is used instead of '%s' in config.ini, updating %s...", tag, getImageSysDir1TagPathElement(value), ImageSysDir1PlayStoreTagPathElement, configIniPth)
		config[ImageSysDir1Key] = replaceImageSysDir1TagPathElement(value, ImageSysDir1PlayStoreTagPathElement)
		updated = true
	}

	if value := config[TagDisplayKey]; value != TagDisplayGooglePlayValue {
		log.Warnf("Using %s tag, but '%s' tag.display is used instead of '%s' in config.ini, updating %s...", tag, value, TagDisplayGooglePlayValue, configIniPth)
		config[TagDisplayKey] = TagDisplayGooglePlayValue
		updated = true
	}

	if value := config[TagIDKey]; value != TagIDGoogleAPIsPlayStoreValue {
		log.Warnf("Using %s tag, but '%s' tag.id is used instead of '%s' in config.ini, updating %s...", tag, value, TagIDGoogleAPIsPlayStoreValue, configIniPth)
		config[TagIDKey] = TagIDGoogleAPIsPlayStoreValue
		updated = true
	}

	if updated {
		log.Printf("") // print newline after the warnings

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

func getImageSysDir1TagPathElement(imageSysDirPth string) string {
	// system-images/android-26/google_apis/x86/
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
