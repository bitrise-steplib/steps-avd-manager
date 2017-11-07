package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bitrise-steplib/steps-avd-manager/hardwareconfig"
	"github.com/bitrise-tools/go-steputils/input"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
)

// ConfigsModel ...
type ConfigsModel struct {
	Version     string
	Locale      string
	Resolution  string
	Orientation string
	AndroidHome string
	Tag         string
	Overwrite   string // TODO
	ID          string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		Version:     os.Getenv("version"),
		Locale:      os.Getenv("locale"),
		Resolution:  os.Getenv("resolution"),
		Orientation: os.Getenv("orientation"),
		Tag:         os.Getenv("tag"),
		ID:          os.Getenv("emulator_id"),
		AndroidHome: os.Getenv("ANDROID_HOME"),
	}
}

func (configs ConfigsModel) print() {
	log.Infof("Configs:")
	log.Printf("- Version: %s", configs.Version)
	log.Printf("- Locale: %s", configs.Locale)
	log.Printf("- Resolution: %s", configs.Resolution)
	log.Printf("- Orientation: %s", configs.Orientation)
	log.Printf("- Tag: %s", configs.Tag)
}

func (configs ConfigsModel) validate() error {
	if err := input.ValidateIfNotEmpty(configs.Version); err != nil {
		return fmt.Errorf("Version, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.ID); err != nil {
		return fmt.Errorf("ID, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.Locale); err != nil {
		return fmt.Errorf("Locale, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.Resolution); err != nil {
		return fmt.Errorf("Resolution, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.Orientation); err != nil {
		return fmt.Errorf("Orientation, %s", err)
	}
	if err := input.ValidateWithOptions(configs.Orientation, "portrait", "landscape"); err != nil {
		return fmt.Errorf("Orientation, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.AndroidHome); err != nil {
		return fmt.Errorf("ANDROID_HOME is not set")
	}
	if err := input.ValidateIfPathExists(configs.AndroidHome); err != nil {
		return fmt.Errorf("ANDROID_HOME does not exists")
	}
	if err := input.ValidateIfNotEmpty(configs.Tag); err != nil {
		return fmt.Errorf("Tag, %s", err)
	}
	if err := input.ValidateWithOptions(configs.Tag, "google_apis", "google_apis_playstore", "android-wear", "android-tv", "default"); err != nil {
		return fmt.Errorf("Tag, %s", err)
	}

	return nil
}

func main() {
	// Input validation
	configs := createConfigsModelFromEnvs()

	fmt.Println()
	configs.print()

	if err := configs.validate(); err != nil {
		fmt.Println()
		log.Errorf("Issue with input: %s", err)
		os.Exit(1)
	}

	fmt.Println()

	// update ensure new sdkmanager, avdmanager
	{
		requiredSDKPackages := []string{"emulator", "platform-tools", fmt.Sprintf("system-images;android-%s;%s;x86", configs.Version, configs.Tag)}

		log.Infof("Ensure sdk packages: %v", requiredSDKPackages)

		out, err := command.New(filepath.Join(configs.AndroidHome, "tools/bin/sdkmanager"), requiredSDKPackages...).RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			log.Errorf("Failed to update emulator sdk package, error: %s, output: %s", err, out)
			os.Exit(1)
		}

		log.Donef("- Done")
	}

	fmt.Println()

	// create emulator
	{
		log.Infof("Create AVD")

		hwConfig := hardwareconfig.New(configs.ID, configs.Tag, configs.Version, configs.Orientation, configs.Locale, configs.Resolution, true)
		if err := hwConfig.Create(); err != nil {
			log.Errorf("Failed to create avd, error: %s", err)
			os.Exit(1)
		}

		log.Donef("- Done")
	}

	// run emulator
	{
		log.Infof("Start emulator")

		out, err := command.New(filepath.Join(configs.AndroidHome, "emulator/emulator"), "-avd", configs.ID, "-no-window", "-no-audio", "-accel", "on", "-qemu", "-display", "none").RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			log.Errorf("Failed to update emulator sdk package, error: %s, output: %s", err, out)
			os.Exit(1)
		}

		log.Donef("- Done")
	}
}
