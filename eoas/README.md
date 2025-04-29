# Ariel OTA CLI

The Ariel OTA CLI is a powerful helper package designed to simplify the setup and update publication process for Ariel OTA, a self-hosted over-the-air update server for React Native applications.

## Quick Start

To get started with the Ariel OTA CLI, check out the official documentation:
[Ariel OTA Documentation](https://help.meet-ariel.com/)

## Installation

Install the CLI globally to use it from anywhere:

```
npm install -g ./eoas
```

Or link it during development:

```
cd eoas && npm run build && npm link
```

## Available Flags

The Ariel OTA CLI supports several flags to customize your OTA updates:

### Build Related Flags

- `--build-number <number>`: Specify a custom build number for your update. This allows you to maintain version control independent of Expo EAS.
- `--skipStorageUpload`: Skip uploading assets to storage (useful for testing publishing flow).
- `--storageType <type>`: Specify the storage provider ('s3', 'firebase', 'local', etc).

### Runtime Flags

- `--runtime <env>`: Specify the runtime environment ('production', 'staging', 'development').
- `--runtime-version <version>`: Specify the runtime version of your app.
- `--platform <platform>`: Target specific platform ('ios', 'android', 'all').
- `--branch <branch>`: The branch to publish updates to.
- `--channel <channel>`: The channel to publish updates to (e.g., 'production', 'staging').

## Important: Matching Build Numbers

The `--build-number` flag in your publish command **must match** the `updateCode` or `buildNumber` in your app's `app.config.js` file. For example:

```js
// In app.config.js
export default {
  // ... other config
  extra: {
    updateCode: "build-22", // This should match the --build-number flag
  }
}
```

## Example Usage

Here's an example of publishing an OTA update:

```
eoas publish --branch ota-updates --channel production --platform all --build-number 22 --runtime-version 1.0.2
```

This command:
- Publishes an update from the `ota-updates` branch
- Targets the `production` channel
- Deploys to both iOS and Android platforms (`all`)
- Sets the build number to `22` (must match app.config.js)
- Uses runtime version `1.0.2`

## Build Number Synchronization

The Ariel OTA CLI now includes an automatic build number synchronization feature that:
1. Automatically checks if the build number in your publish command matches app.config.js
2. Updates app.config.js if they don't match
3. Commits the change before publishing the update

This helps prevent mismatches between your publish command and app configuration, ensuring consistent versioning across your application.

## Learn More
For detailed information and to explore the core functionalities of Ariel OTA, visit:
[Ariel OTA Documentation](https://help.meet-ariel.com/)

---

Developed and maintained by [Arietta AB](https://meet-ariel.com).

