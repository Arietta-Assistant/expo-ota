import { Platform, Env } from '@expo/eas-build-job';
import { Command, Flags } from '@oclif/core';
import fs from 'fs-extra';
import https from 'https';
import mime from 'mime';
import url from 'url';

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

  static override examples = ['<%= config.bin %> <%= command.id %>'];

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
    'runtime-version': Flags.string({
      char: 'r',
      description: 'Runtime version to publish for',
      required: true,
    }),
    message: Flags.string({
      char: 'm',
      description: 'Update message',
    }),
    'local-project': Flags.string({
      description: 'Directory containing update source files',
      required: true,
      default: '.',
    }),
    'launch-jsurl': Flags.string({
      description: 'URL to launch the update from',
    }),
    'build-number': Flags.string({
      description: 'Build number to include in the update ID',
    }),
  };

  public async run(): Promise<void> {
    const { flags } = await this.parse(Publish);
    const {
      platform,
      nonInteractive,
      branch,
      channel,
      'runtime-version': _runtimeVersion,
      message: _message,
      'local-project': _expoLocalProject,
      'launch-jsurl': _launchJsUrl,
      'build-number': _buildNumber,
    } = this.sanitizeFlags(flags);
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
    const appBuildNumber = privateConfig.extra?.buildNumber || privateConfig.extra?.updateCode;
    if (!appBuildNumber) {
      Log.error('Build number or update code is required in app.config.js extra field');
      process.exit(1);
    }

    const spinner = ora('Publishing update').start();
    try {
      // First, run 'npx expo export' to generate updated bundles
      spinner.text = 'Exporting the project...';
      
      // Determine which platforms to export
      const platformArg = platform === RequestedPlatform.All 
        ? 'ios,android' 
        : platform === RequestedPlatform.Android 
          ? 'android' 
          : 'ios';
          
      // Build the export command with the runtime version
      const exportCmd = `npx expo export --platform=${platformArg} --dump-sourcemap --asset-manifest --output-dir=./dist`;
      
      try {
        const { execSync } = require('child_process');
        Log.debug(`Running: ${exportCmd}`);
        const result = execSync(exportCmd, { 
          cwd: projectDir,
          stdio: 'pipe', // Capture output
          encoding: 'utf-8'
        });
        Log.debug(`Export completed: ${result}`);
      } catch (error) {
        spinner.fail('Export failed');
        Log.error('Failed to export the project. Please make sure Expo CLI is installed.');
        Log.error(error);
        process.exit(1);
      }
      
      spinner.text = 'Computing file requests...';
      const files = await computeFilesRequests(projectDir, platform);
      const result = await requestUploadUrls({
        body: { fileNames: files.map(f => f.name) },
        requestUploadUrl: `${baseUrl}/api/update/request-upload-urls/${branch}`,
        runtimeVersion: runtimeVersionResult.runtimeVersion,
        platform: platform === RequestedPlatform.All ? 'all' : platform.toString().toLowerCase(),
        commitHash,
        buildNumber: _buildNumber || appBuildNumber,
      });
      const { uploadRequests } = result;

      for (const file of files) {
        const uploadUrl = uploadRequests.find(url => url.fileName === file.name);
        if (!uploadUrl) {
          throw new Error(`No upload URL found for file ${file.name}`);
        }

        // Read the file content
        const fileContent = await fs.readFile(file.path);
        const contentType = mime.getType(file.path) || 'application/octet-stream';
        
        // Log information about file being uploaded
        spinner.text = `Uploading ${file.name} (${fileContent.length} bytes)`;
        
        try {
          // Use the raw https module for better control over the request
          await new Promise<void>((resolve, reject) => {
            const parsedUrl = new url.URL(uploadUrl.requestUploadUrl);
            
            const options = {
              hostname: parsedUrl.hostname,
              path: parsedUrl.pathname + parsedUrl.search,
              method: 'PUT',
              headers: {
                'Content-Type': contentType,
                'Content-Length': fileContent.length
              }
            };
            
            const req = https.request(options, (res) => {
              let responseBody = '';
              
              res.on('data', (chunk) => {
                responseBody += chunk;
              });
              
              res.on('end', () => {
                if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
                  resolve();
                } else {
                  reject(new Error(`Failed to upload file ${file.name}: ${responseBody}`));
                }
              });
            });
            
            req.on('error', (err) => {
              reject(new Error(`Network error while uploading ${file.name}: ${err.message}`));
            });
            
            req.write(fileContent);
            req.end();
          });
        } catch (error) {
          throw error;
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
    'runtime-version'?: string;
    message?: string;
    'local-project'?: string;
    'launch-jsurl'?: string;
    'build-number'?: string;
  }): {
    platform: RequestedPlatform;
    nonInteractive: boolean;
    branch?: string;
    channel?: string;
    'runtime-version'?: string;
    message?: string;
    'local-project': string;
    'launch-jsurl'?: string;
    'build-number'?: string;
  } {
    return {
      platform: this.parsePlatform(flags.platform),
      nonInteractive: Boolean(flags.nonInteractive),
      branch: flags.branch,
      channel: flags.channel,
      'runtime-version': flags['runtime-version'],
      message: flags.message,
      'local-project': flags['local-project'] || '.',
      'launch-jsurl': flags['launch-jsurl'],
      'build-number': flags['build-number'],
    };
  }

  private parsePlatform(platform?: string): RequestedPlatform {
    if (platform === 'all') {
      return RequestedPlatform.All;
    } else if (platform === 'android') {
      return RequestedPlatform.Android;
    } else if (platform === 'ios') {
      return RequestedPlatform.Ios;
    } else {
      throw new Error('Invalid platform');
    }
  }
}
