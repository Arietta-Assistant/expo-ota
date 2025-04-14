import { Command, Flags } from '@oclif/core';
import fs from 'fs-extra';
import path from 'path';

import { getExpoConfigUpdateUrl, getPrivateExpoConfigAsync, createOrModifyExpoConfigAsync } from '../lib/expoConfig';
import Log from '../lib/log';
import { ora } from '../lib/ora';
import { confirmAsync, promptAsync } from '../lib/prompts';
import { isExpoInstalled } from '../lib/package';

export default class Init extends Command {
  static override args = {};
  static override description = 'Initialize your project with expo-open-ota';
  static override examples = ['<%= config.bin %> <%= command.id %>'];
  static override flags = {
    nonInteractive: Flags.boolean({
      description: 'Run in non-interactive mode',
      default: false,
    }),
  };
  public async run(): Promise<void> {
    const { flags } = await this.parse(Init);
    const { nonInteractive } = flags;

    const projectDir = process.cwd();
    const hasExpo = isExpoInstalled(projectDir);
    if (!hasExpo) {
      Log.error('Expo is not installed in this project. Please install Expo first.');
      process.exit(1);
    }
    const config = await getPrivateExpoConfigAsync(projectDir);
    const currentUrl = getExpoConfigUpdateUrl(config);
    if (!config) {
      Log.error(
        'Could not find Expo config in this project. Please make sure you have an Expo config.'
      );
      return;
    }

    if (!nonInteractive) {
      const confirmed = await confirmAsync({
        message: 'Do you have already generated your certificates for code signing?',
        name: 'certificates',
        type: 'confirm',
      });
      if (!confirmed) {
        Log.fail('You need to generate your certificates first by using npx eoas generate-certs');
        return;
      }
    }

    const manifestEndpoint = await promptAsync({
      message: 'Enter your manifest endpoint (ex: https://your-domain.com)',
      name: 'manifestEndpoint',
      type: 'text',
      initial: currentUrl || 'https://your-domain.com',
    });

    const { codeSigningCertificatePath } = await promptAsync({
      message: 'Enter the path to your code signing certificate (ex: ./certs/certificate.pem)',
      name: 'codeSigningCertificatePath',
      type: 'text',
      initial: './certs/certificate.pem',
      validate: async (v) => {
        try {
          const fullPath = path.resolve(projectDir, v);
          const fileExists = await fs.pathExists(fullPath);
          if (!fileExists) {
            Log.newLine();
            Log.error('File does not exist');
            return false;
          }
          const key = await fs.readFile(fullPath, 'utf8');
          if (!key) {
            Log.error('Empty key');
            return false;
          }
          return true;
        } catch {
          return false;
        }
      },
    });

    const newUpdateConfig = {
      url: manifestEndpoint,
      codeSigningMetadata: {
        keyid: 'main',
        alg: 'rsa-v1_5-sha256' as const,
      },
      codeSigningCertificate: codeSigningCertificatePath,
      enabled: true,
      requestHeaders: {
        'expo-channel-name': 'process.env.RELEASE_CHANNEL',
      },
    };

    const updateConfigSpinner = ora('Updating Expo config').start();
    try {
      await createOrModifyExpoConfigAsync(projectDir, {
        updates: newUpdateConfig,
      });
      updateConfigSpinner.succeed(
        'Expo config successfully updated do not forget to format the file with prettier or eslint'
      );
    } catch (e) {
      updateConfigSpinner.fail('Failed to update Expo config');
      Log.error(e);
    }
  }
}
