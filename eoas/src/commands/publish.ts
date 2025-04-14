import { Platform, Env } from '@expo/eas-build-job';
import { Command, Flags } from '@oclif/core';
import FormData from 'form-data';
import fs from 'fs-extra';
import mime from 'mime';
import fetch from 'node-fetch';

import { computeFilesRequests, requestUploadUrls } from '../lib/assets';
import {
  RequestedPlatform,
  getExpoConfigUpdateUrl,
  getPrivateExpoConfigAsync,
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

    const workflow = await resolveWorkflowAsync(
      projectDir,
      platform === RequestedPlatform.All ? Platform.ANDROID : platform === RequestedPlatform.Android ? Platform.ANDROID : Platform.IOS,
      vcsClient
    );
    const runtimeVersionResult = await resolveRuntimeVersionAsync({
      exp: privateConfig,
      platform: platform === RequestedPlatform.All || platform === RequestedPlatform.Android ? 'android' : 'ios',
      workflow,
      projectDir,
      env: process.env as Env,
    });

    if (!runtimeVersionResult?.runtimeVersion) {
      Log.error('Could not resolve runtime version');
      process.exit(1);
    }

    // Remove Firebase token check
    const buildNumber = privateConfig.extra?.buildNumber || privateConfig.extra?.updateCode;
    if (!buildNumber) {
      Log.error('Build number or update code is required in app.config.js extra field');
      process.exit(1);
    }

    const spinner = ora('Publishing update').start();
    try {
      const files = await computeFilesRequests(projectDir, platform);
      const uploadUrls = await requestUploadUrls({
        body: { 
          fileNames: files.map(f => f.name)
        },
        requestUploadUrl: `${baseUrl}/update/request-upload-urls/${branch}`,
        runtimeVersion: runtimeVersionResult.runtimeVersion,
        platform: platform === RequestedPlatform.All ? 'all' : platform.toString().toLowerCase(),
        commitHash,
        auth: undefined,
      });

      for (const file of files) {
        const uploadUrl = uploadUrls.uploadRequests.find(url => url.fileName === file.name);
        if (!uploadUrl) {
          throw new Error(`No upload URL found for file ${file.name}`);
        }

        const form = new FormData();
        form.append('file', fs.createReadStream(file.path), {
          contentType: mime.getType(file.path) || 'application/octet-stream',
          filename: file.name,
        });

        const response = await fetch(uploadUrl.requestUploadUrl, {
          method: 'PUT',
          body: form,
        });

        if (!response.ok) {
          throw new Error(`Failed to upload file ${file.name}: ${await response.text()}`);
        }
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
