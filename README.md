# AVD Manager

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/steps-avd-manager?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/steps-avd-manager/releases)

Create and boot an Android emulator used for device testing

<details>
<summary>Description</summary>

Run instrumented and UI tests on a virtual Android device. Once some basic inputs are set, the Step checks the requirements, downloads the selected system image before creating and starting the emulator.

**Warning:** Android emulators can't run on Apple Silicon build machines. Until nested virtualization becomes supported, you should run emulator tests on Linux machines.

### Configuring the Step
1. Add the **AVD Manager** Step to your Workflow as one of the first Steps in your Workflow.
2. Set the **Device Profile** to create a new Android virtual device. To see the complete list of available profiles, use the `avdmanager list device` command and use the `id` value for this input.
3. Set the **Android API Level**. The new virtual device will run with the specified Android version.
4. Select an **OS Tag** to have the required toolset on the new virtual device.

Some system images are pre-installed on the virtual machines. In this case the step won't have to spend time downloading the requested image. To check the list of pre-installed images for each stack, visit the [system reports](https://stacks.bitrise.io).

### Troubleshooting
The emulator needs some time to boot up. The earlier you place the Step in your Workflow, the more tasks, such as cloning or caching, you can complete in your Workflow before the emulator starts working.
We recommend that you also add **Wait for Android emulator** Step to your Workflow as it acts as a shield preventing the AVD Manager to kick in too early. Make sure you add the **Wait for Android emulator** Step BEFORE the Step with which you want to use the **AVD Manager**.

### Useful links
- [Getting started with Android apps](https://devcenter.bitrise.io/getting-started/getting-started-with-android-apps/)
- [Device testing for Android](https://devcenter.bitrise.io/testing/device-testing-for-android/)
- [About Test Reports](https://devcenter.bitrise.io/testing/test-reports/)

### Related Steps
- [Wait for Android emulator](https://www.bitrise.io/integrations/steps/wait-for-android-emulator)
- [Android Build for UI testing](https://www.bitrise.io/integrations/steps/android-build-for-ui-testing)
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://devcenter.bitrise.io/steps-and-workflows/steps-and-workflows-index/).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `profile` | The profile contains parameters of the device, such as screen size and resolution.  To see the complete list of available profiles use the `avdmanager list device` command locally and use the `id` value for this input.  | required | `pixel` |
| `api_level` | The device will run with the specified system image version. | required | `26` |
| `tag` | Select OS tag to have the required toolset on the device. | required | `google_apis` |
| `abi` | Select which ABI to use running the emulator. Availability depends on API level. Please use `sdkmanager --list` command to see the available ABIs. | required | `x86` |
| `disable_animations` | Disable animations on the emulator in order to make tests faster and more stable.  Animations can be enabled/disabled from the test code too, so if your tests do need animations, set this step input to `no` and control the settings yourself. | required | `yes` |
| `emulator_id` | Set the device's ID. (This will be the name under $HOME/.android/avd/) | required | `emulator` |
| `create_command_flags` | Flags used when running the command to create the emulator. |  | `--sdcard 2048M` |
| `start_command_flags` | Flags used when running the command to start the emulator. |  | `-camera-back none -camera-front none` |
| `emulator_build_number` | Allows installing a specific emulator version at runtime. The default value (`preinstalled`) will use the emulator version preinstalled on the Stack, which is updated regularly to the latest stable version.  See available build numbers [here](https://developer.android.com/studio/emulator_archive). You need the last segment of the download URL, for example, build number `12658423` from `emulator-linux_x64-12658423.zip`. Note: this input expects the **build number**, not the **version number**.  When this input set to a specific build number, the `emulator_channel` input should be set to `no update`. |  | `preinstalled` |
| `emulator_channel` | Select which channel to use with `sdkmanager` to fetch *emulator* package. Available options are no update, or channels 0 (Stable), 1 (Beta), 2 (Dev), and 3 (Canary).  - `no update`: The *emulator* preinstalled on the Stack will be used. *system-image* will be updated to the latest Stable version.  To update *emulator* and *system image* to the latest available in a given channel: - `0`: Stable channel - `1`: Beta channel - `2`: Dev channel - `3`: Canary channel  When this input set to a specific channel, the `emulator_build_number` input should be set to `preinstalled`. | required | `no update` |
| `headless_mode` | In headless mode the emulator is not launched in the foreground.  If this input is set, the emulator will not be visible but tests (even the screenshots) will run just like if the emulator ran in the foreground. | required | `yes` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_EMULATOR_SERIAL` | Booted emulator serial |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/steps-avd-manager/pulls) and [issues](https://github.com/bitrise-steplib/steps-avd-manager/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
