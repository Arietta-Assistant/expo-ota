import { Platform } from '@expo/eas-build-job';
import spawnAsync from '@expo/spawn-async';
import { Command, Flags } from '@oclif/core';
import FormData from 'form-data';
import fs from 'fs-extra';
import mime from 'mime';
import fetch from 'node-fetch';
import path from 'path';

import { RequestUploadUrlItem, computeFilesRequests, requestUploadUrls } from '../lib/assets';
import { getAuthExpoHeaders, retrieveExpoCredentials } from '../lib/auth';
import {
  RequestedPlatform,
  getExpoConfigUpdateUrl,
  getPrivateExpoConfigAsync,
  getPublicExpoConfigAsync,
} from '../lib/expoConfig';
import Log from '../lib/log';
import { ora } from '../lib/ora';
import { isExpoInstalled } from '../lib/package';
import { confirmAsync } from '../lib/prompts';
import { ensureRepoIsCleanAsync } from '../lib/repo';
import { resolveRuntimeVersionAsync } from '../lib/runtimeVersion';
import { resolveVcsClient } from '../lib/vcs';
import { resolveWorkflowAsync } from '../lib/workflow';

export default class Publish extends Command {
  static override args = {};

  static override description = 'Publish an update to your self-hosted server';

  static override flags = {
    platform: Flags.string({
      char: 'p',
      description: 'Platform to publish for',
      options: ['all', 'android', 'ios'],
      default: 'all',
    }),
    nonInteractive: Flags.boolean({
      description: 'Run in non-interactive mode',
      default: false,
    }),
    branch: Flags.string({
      char: 'b',
      description: 'Branch to publish to',
    }),
    channel: Flags.string({
      char: 'c',
      description: 'Channel to publish to',
    }),
  };

  public async run(): Promise<void> {
    const { flags } = await this.parse(Publish);
    const { platform, nonInteractive, branch, channel } = this.sanitizeFlags(flags);
    if (!branch) {
      Log.error('Branch name is required');
      process.exit(1);
    }
    if (!channel) {
      Log.error('Channel name is required');
      process.exit(1);
    }

    const vcsClient = resolveVcsClient(true);
    await vcsClient.ensureRepoExistsAsync();
    const commitHash = await vcsClient.getCommitHashAsync();
    await ensureRepoIsCleanAsync(vcsClient, nonInteractive);

    const projectDir = process.cwd();
    const hasExpo = isExpoInstalled(projectDir);
    if (!hasExpo) {
      Log.error('Expo is not installed in this project. Please install Expo first.');
      process.exit(1);
    }

    const privateConfig = await getPrivateExpoConfigAsync(projectDir);
    const updateUrl = getExpoConfigUpdateUrl(privateConfig);
    if (!updateUrl) {
      Log.error(
        "Update url is not setup in your config. Please run 'eoas init' to setup the update url"
      );
      process.exit(1);
    }

    let baseUrl: string;
    try {
      const parsedUrl = new URL(updateUrl);
      baseUrl = parsedUrl.origin;
    } catch (e) {
      Log.error('Invalid URL', e);
      process.exit(1);
    }

    if (!nonInteractive) {
      const confirmed = await confirmAsync({
        message: `Is this the correct URL of your self-hosted update server? ${baseUrl}`,
        name: 'export',
        type: 'confirm',
      });
      if (!confirmed) {
        Log.error('Please run `eoas init` to setup the correct update url');
        process.exit(1);
      }
    }

    const runtimeVersion = await resolveRuntimeVersionAsync(projectDir, platform);
    if (!runtimeVersion) {
      Log.error('Could not resolve runtime version');
      process.exit(1);
    }

    // Get Firebase token
    const firebaseToken = process.env.FIREBASE_TOKEN;
    if (!firebaseToken) {
      Log.error('FIREBASE_TOKEN environment variable is required');
      process.exit(1);
    }

    // Get build number from extra field
    const buildNumber = privateConfig.extra?.buildNumber;
    if (!buildNumber) {
      Log.error('Build number is required in app.config.js extra field');
      process.exit(1);
    }

    const spinner = ora('Publishing update').start();
    try {
      const files = await computeFilesRequests(projectDir, platform);
      const uploadUrls = await requestUploadUrls(baseUrl, files, {
        branch,
        runtimeVersion,
        platform,
        commitHash,
        buildNumber,
        firebaseToken,
      });

      for (const file of files) {
        const uploadUrl = uploadUrls.find(url => url.fileName === file.path);
        if (!uploadUrl) {
          throw new Error(`No upload URL found for file ${file.path}`);
        }

        const form = new FormData();
        form.append('file', fs.createReadStream(file.path), {
          contentType: mime.getType(file.path) || 'application/octet-stream',
          filename: path.basename(file.path),
        });

        await fetch(uploadUrl.url, {
          method: 'PUT',
          body: form,
          headers: {
            Authorization: `Bearer ${firebaseToken}`,
          },
        });
      }

      spinner.succeed('Update published successfully');
    } catch (error) {
      spinner.fail('Failed to publish update');
      Log.error(error);
      process.exit(1);
    }
  }

  private sanitizeFlags(flags: {
    platform?: string;
    nonInteractive?: boolean;
    branch?: string;
    channel?: string;
  }): {
    platform: RequestedPlatform;
    nonInteractive: boolean;
    branch?: string;
    channel?: string;
  } {
    return {
      platform: (flags.platform as RequestedPlatform) || RequestedPlatform.All,
      nonInteractive: flags.nonInteractive || false,
      branch: flags.branch,
      channel: flags.channel,
    };
  }
}
