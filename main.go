package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitrise-steplib/steps-avd-manager/hardwareconfig"
	"github.com/bitrise-tools/go-steputils/input"
	shellquote "github.com/kballard/go-shellquote"

	"github.com/bitrise-io/depman/pathutil"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
)

// ConfigsModel ...
type ConfigsModel struct {
	Version            string
	Locale             string
	Resolution         string
	Orientation        string
	AndroidHome        string
	Tag                string
	ID                 string
	CustomConfig       string
	Density            string
	Overwrite          string
	CustomCommandFlags string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		Version:            os.Getenv("version"),
		Locale:             os.Getenv("locale"),
		Resolution:         os.Getenv("resolution"),
		Orientation:        os.Getenv("orientation"),
		Tag:                os.Getenv("tag"),
		ID:                 os.Getenv("emulator_id"),
		Density:            os.Getenv("density"),
		Overwrite:          os.Getenv("overwrite"),
		CustomConfig:       os.Getenv("custom_hw_config"),
		AndroidHome:        os.Getenv("ANDROID_HOME"),
		CustomCommandFlags: os.Getenv("custom_command_flags"),
	}
}

func (configs ConfigsModel) print() {
	log.Infof("Configs:")
	log.Printf("- Version: %s", configs.Version)
	log.Printf("- Locale: %s", configs.Locale)
	log.Printf("- Resolution: %s", configs.Resolution)
	log.Printf("- Density: %s", configs.Density)
	log.Printf("- Orientation: %s", configs.Orientation)
	log.Printf("- Tag: %s", configs.Tag)
	log.Printf("- ID: %s", configs.ID)
	log.Printf("- CustomCommandFlags: %s", configs.CustomCommandFlags)
	log.Printf("- Overwrite: %s", configs.Overwrite)
	log.Printf("- CustomConfig: %s", configs.CustomConfig)
}

func (configs ConfigsModel) validate() error {
	if err := input.ValidateIfNotEmpty(configs.Version); err != nil {
		return fmt.Errorf("Version, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.Overwrite); err != nil {
		return fmt.Errorf("Overwrite, %s", err)
	}
	if err := input.ValidateWithOptions(configs.Overwrite, "true", "false"); err != nil {
		return fmt.Errorf("Overwrite, %s", err)
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
		requiredSDKPackages := []string{"emulator", "tools", "platform-tools", fmt.Sprintf("system-images;android-%s;%s;x86", configs.Version, configs.Tag)}

		log.Infof("Ensure sdk packages: %v", requiredSDKPackages)

		out, err := command.New(filepath.Join(configs.AndroidHome, "tools/bin/sdkmanager"), requiredSDKPackages...).RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			log.Errorf("Failed to update emulator sdk package, error: %s, output: %s", err, out)
			os.Exit(1)
		}

		log.Donef("- Done")
	}

	avdPath := filepath.Join(os.Getenv("HOME"), ".android/avd", fmt.Sprintf("%s.avd", configs.ID))
	avdPathExists, err := pathutil.IsPathExists(avdPath)
	if err != nil {
		log.Errorf("Failed to check if path exists: %s", err)
		os.Exit(1)
	}
	iniPath := filepath.Join(os.Getenv("HOME"), ".android/avd", fmt.Sprintf("%s.ini", configs.ID))
	iniPathExists, err := pathutil.IsPathExists(iniPath)
	if err != nil {
		log.Errorf("Failed to check if path exists: %s", err)
		os.Exit(1)
	}

	if configs.Overwrite == "true" {
		if iniPathExists || avdPathExists {
			fmt.Println()
			log.Infof("Delete AVD")
			if avdPathExists {
				if err := os.RemoveAll(avdPath); err != nil {
					log.Errorf("Failed to remove avd dir: %s", err)
					os.Exit(1)
				}
			}
			if iniPathExists {
				if err := os.RemoveAll(iniPath); err != nil {
					log.Errorf("Failed to remove ini file: %s", err)
					os.Exit(1)
				}
			}
			log.Donef("- Done")
		}
	}

	// create emulator
	{
		if (!iniPathExists && !avdPathExists) || configs.Overwrite == "true" {
			fmt.Println()
			log.Infof("Create AVD")

			hwConfig := hardwareconfig.New(configs.ID, configs.Tag, configs.Version, configs.Orientation, configs.Locale, configs.Resolution, configs.Density)

			for _, config := range strings.Split(configs.CustomConfig, "\n") {
				if strings.TrimSpace(config) == "" {
					continue
				}

				configSplit := strings.Split(config, "=")
				if len(configSplit) < 2 {
					continue
				}
				hwConfig.Config.SetProperty(configSplit[0], strings.Join(configSplit[1:], "="))
			}

			if err := hwConfig.Create(); err != nil {
				log.Errorf("Failed to create avd, error: %s", err)
				os.Exit(1)
			}

			log.Donef("- Done")
		} else {
			fmt.Println()
			log.Donef("Using existing AVD")
		}
	}

	fmt.Println()

	// run emulator
	{
		log.Infof("Start emulator")

		customFlags, err := shellquote.Split(configs.CustomCommandFlags)
		if err != nil {
			log.Errorf("Failed to parse commands, error: %s", err)
			os.Exit(1)
		}

		cmdSlice := []string{"-avd", configs.ID}

		cmdSlice = append(cmdSlice, customFlags...)

		cmd := command.New(filepath.Join(configs.AndroidHome, "emulator/emulator"), cmdSlice...)

		cmd.SetStderr(os.Stderr)

		err = cmd.GetCmd().Start()
		if err != nil {
			log.Errorf("Failed to update emulator sdk package, error: %s", err)
			os.Exit(1)
		}

		log.Donef("- Done")
	}
}
