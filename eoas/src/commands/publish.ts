import { Platform, Env } from '@expo/eas-build-job';
import { Command, Flags } from '@oclif/core';
import fs from 'fs-extra';
import https from 'https';
import mime from 'mime';
import url from 'url';
import path from 'path';
import spawnAsync from '@expo/spawn-async';

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

    const runtimeSpinner = ora('🔄 Resolving runtime version...').start();
    const runtimeVersions = [
      ...(!platform || platform === RequestedPlatform.All || platform === RequestedPlatform.Ios
        ? [
            {
              runtimeVersion: (
                await resolveRuntimeVersionAsync({
                  exp: privateConfig,
                  platform: 'ios',
                  workflow: await resolveWorkflowAsync(projectDir, Platform.IOS, vcsClient),
                  projectDir,
                  env: process.env as Env,
                })
              )?.runtimeVersion,
              platform: 'ios',
            },
          ]
        : []),
      ...(!platform || platform === RequestedPlatform.All || platform === RequestedPlatform.Android
        ? [
            {
              runtimeVersion: (
                await resolveRuntimeVersionAsync({
                  exp: privateConfig,
                  platform: 'android',
                  workflow: await resolveWorkflowAsync(projectDir, Platform.ANDROID, vcsClient),
                  projectDir,
                  env: process.env as Env,
                })
              )?.runtimeVersion,
              platform: 'android',
            },
          ]
        : []),
    ].filter(({ runtimeVersion }) => !!runtimeVersion);
    if (!runtimeVersions.length) {
      runtimeSpinner.fail('Could not resolve runtime versions for the requested platforms');
      Log.error('Could not resolve runtime versions for the requested platforms');
      process.exit(1);
    }
    runtimeSpinner.succeed('✅ Runtime versions resolved');

    // 1. Clean up the dist directory
    const cleaningSpinner = ora(`🗑️ Cleaning up output directory...`).start();
    try {
      await spawnAsync('rm', ['-rf', 'dist'], { cwd: projectDir });
      cleaningSpinner.succeed('✅ Cleanup completed');
    } catch (e) {
      cleaningSpinner.fail('❌ Failed to clean up the output directory');
      Log.error(e);
      process.exit(1);
    }

    // 2. Export the project
    const exportSpinner = ora('📦 Exporting project files...').start();
    try {
      // Determine platforms for export
      let platformArgs: string[] = [];
      if (platform === RequestedPlatform.All) {
        platformArgs = ['--platform', 'ios', '--platform', 'android'];
      } else if (platform === RequestedPlatform.Android) {
        platformArgs = ['--platform', 'android'];
      } else {
        platformArgs = ['--platform', 'ios'];
      }
      
      const exportCmd = [
        'expo', 
        'export', 
        ...platformArgs,
        '--dump-sourcemap',
        '--dump-assetmap',
        '--output-dir',
        './dist'
      ];
      
      Log.debug(`Running: npx ${exportCmd.join(' ')}`);
      
      try {
        const { stdout } = await spawnAsync('npx', exportCmd, {
          cwd: projectDir,
          env: {
            ...process.env,
            EXPO_NO_DOTENV: '1',
          }
        });
        exportSpinner.succeed('🚀 Project exported successfully');
        Log.debug(stdout);
      } catch (error: any) {
        // Capture more detailed error information
        exportSpinner.fail(`❌ Export failed`);
        
        if (error.stdout) {
          Log.error(`Export stdout: ${error.stdout}`);
        }
        
        if (error.stderr) {
          Log.error(`Export stderr: ${error.stderr}`);
        }
        
        // Try running a more basic export as a fallback
        exportSpinner.text = '🔄 Trying fallback export method...';
        
        try {
          Log.debug('Attempting fallback export...');
          
          // Basic fallback with minimal options
          let fallbackPlatformArgs: string[] = [];
          if (platform === RequestedPlatform.All) {
            fallbackPlatformArgs = ['--platform', 'ios', '--platform', 'android'];
          } else if (platform === RequestedPlatform.Android) {
            fallbackPlatformArgs = ['--platform', 'android'];
          } else {
            fallbackPlatformArgs = ['--platform', 'ios'];
          }
          
          const fallbackCmd = [
            'expo',
            'export',
            ...fallbackPlatformArgs,
            '--output-dir',
            './dist'
          ];
          
          Log.debug(`Running fallback: npx ${fallbackCmd.join(' ')}`);
          
          const { stdout } = await spawnAsync('npx', fallbackCmd, {
            cwd: projectDir,
            env: {
              ...process.env,
              EXPO_NO_DOTENV: '1',
            }
          });
          exportSpinner.succeed('🚀 Project exported with fallback method');
          Log.debug(stdout);
        } catch (fallbackError: any) {
          exportSpinner.fail(`❌ Fallback export also failed`);
          if (fallbackError.stdout) {
            Log.error(`Fallback stdout: ${fallbackError.stdout}`);
          }
          if (fallbackError.stderr) {
            Log.error(`Fallback stderr: ${fallbackError.stderr}`);
          }
          Log.error(`Failed to export the project: ${error}`);
          process.exit(1);
        }
      }
    } catch (e) {
      exportSpinner.fail(`❌ Failed to export the project: ${e}`);
      process.exit(1);
    }

    // 3. Create expoConfig.json
    const publicConfig = await getPrivateExpoConfigAsync(projectDir);
    if (!publicConfig) {
      Log.error('Could not find Expo config in this project');
      process.exit(1);
    }
    
    fs.writeJsonSync(path.join(projectDir, 'dist', 'expoConfig.json'), publicConfig, {
      spaces: 2,
    });
    Log.debug('expoConfig.json file created in dist directory');

    // 4. Compute file requests and upload
    const uploadFilesSpinner = ora('📤 Uploading files...').start();
    
    let files;
    try {
      files = await computeFilesRequests(projectDir, platform);
      if (!files || files.length === 0) {
        uploadFilesSpinner.fail('No files to upload');
        process.exit(1);
      }
      Log.debug(`Found ${files.length} files to upload`);
    } catch (error) {
      uploadFilesSpinner.fail('Failed to compute file requests');
      Log.error(`Error computing file requests: ${error}`);
      process.exit(1);
    }
    
    try {
      const result = await requestUploadUrls({
        body: { fileNames: files.map(f => f.name) },
        requestUploadUrl: `${baseUrl}/api/update/request-upload-urls/${branch}`,
        runtimeVersion: runtimeVersions[0].runtimeVersion || '',
        platform: runtimeVersions[0].platform,
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
        uploadFilesSpinner.text = `Uploading ${file.name} (${fileContent.length} bytes)`;
        
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

      uploadFilesSpinner.succeed('Update published successfully');
    } catch (error) {
      uploadFilesSpinner.fail('Failed to publish update');
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
