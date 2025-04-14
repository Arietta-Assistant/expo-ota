// This file is copied from eas-cli[https://github.com/expo/eas-cli] to ensure consistent user experience across the CLI.
import { ExpoConfig, getConfig, getConfigFilePaths } from '@expo/config';
import spawnAsync from '@expo/spawn-async';
import fs from 'fs-extra';
import Joi from 'joi';

import Log from './log';
import { isExpoInstalled } from './package';

export enum RequestedPlatform {
  Android = 'android',
  Ios = 'ios',
  All = 'all',
}

export type PublicExpoConfig = Omit<
  ExpoConfig,
  '_internal' | 'hooks' | 'ios' | 'android' | 'updates'
> & {
  ios?: Omit<ExpoConfig['ios'], 'config'>;
  android?: Omit<ExpoConfig['android'], 'config'>;
  updates?: Omit<ExpoConfig['updates'], 'codeSigningCertificate' | 'codeSigningMetadata'>;
};

export interface ExpoConfigOptions {
  skipSDKVersionRequirement?: boolean;
  skipPlugins?: boolean;
}

interface ExpoConfigOptionsInternal extends ExpoConfigOptions {
  isPublicConfig?: boolean;
}

let wasExpoConfigWarnPrinted = false;

async function getExpoConfigInternalAsync(
  projectDir: string,
  opts: ExpoConfigOptionsInternal = {}
): Promise<ExpoConfig> {
  const originalProcessEnv: NodeJS.ProcessEnv = process.env;
  try {
    process.env = {
      ...process.env,
    };

    let exp: ExpoConfig;
    if (isExpoInstalled(projectDir)) {
      try {
        const { stdout } = await spawnAsync(
          'npx',
          ['expo', 'config', '--json', ...(opts.isPublicConfig ? ['--type', 'public'] : [])],
          {
            cwd: projectDir,
            env: {
              ...process.env,
              EXPO_NO_DOTENV: '1',
            },
          }
        );
        exp = JSON.parse(stdout);
      } catch (err: any) {
        if (!wasExpoConfigWarnPrinted) {
          Log.warn(
            `Failed to read the app config from the project using "npx expo config" command: ${err.message}.`
          );
          Log.warn('Falling back to the version of "@expo/config" shipped with the CLI.');
          wasExpoConfigWarnPrinted = true;
        }
        exp = getConfig(projectDir, {
          skipSDKVersionRequirement: true,
          ...(opts.isPublicConfig ? { isPublicConfig: true } : {}),
          ...(opts.skipPlugins ? { skipPlugins: true } : {}),
        }).exp;
      }
    } else {
      exp = getConfig(projectDir, {
        skipSDKVersionRequirement: true,
        ...(opts.isPublicConfig ? { isPublicConfig: true } : {}),
        ...(opts.skipPlugins ? { skipPlugins: true } : {}),
      }).exp;
    }

    const { error } = MinimalAppConfigSchema.validate(exp, {
      allowUnknown: true,
      abortEarly: true,
    });
    if (error) {
      throw new Error(`Invalid app config.\n${error.message}`);
    }
    return exp;
  } finally {
    process.env = originalProcessEnv;
  }
}

const MinimalAppConfigSchema = Joi.object({
  slug: Joi.string().required(),
  name: Joi.string().required(),
  version: Joi.string(),
  android: Joi.object({
    versionCode: Joi.number().integer(),
  }),
  ios: Joi.object({
    buildNumber: Joi.string(),
  }),
});

export async function getPrivateExpoConfigAsync(
  projectDir: string,
  opts: ExpoConfigOptions = {}
): Promise<ExpoConfig> {
  ensureExpoConfigExists(projectDir);
  return await getExpoConfigInternalAsync(projectDir, { ...opts, isPublicConfig: false });
}

export async function getPublicExpoConfigAsync(
  projectDir: string,
  opts: ExpoConfigOptions = {}
): Promise<PublicExpoConfig> {
  ensureExpoConfigExists(projectDir);
  return await getExpoConfigInternalAsync(projectDir, { ...opts, isPublicConfig: true });
}

export function getExpoConfigUpdateUrl(config: ExpoConfig): string | null {
  return config.updates?.url ?? null;
}

export function ensureExpoConfigExists(projectDir: string): void {
  const paths = getConfigFilePaths(projectDir);
  if (!paths.dynamicConfigPath && !paths.staticConfigPath) {
    throw new Error(
      'No Expo config found. Please create an app.json or app.config.js in your project directory.'
    );
  }
}

export async function createOrModifyExpoConfigAsync(
  projectDir: string,
  updates: { updates: any }
): Promise<void> {
  const config = await getPrivateExpoConfigAsync(projectDir);
  config.updates = {
    ...config.updates,
    ...updates.updates,
  };
  
  const paths = getConfigFilePaths(projectDir);
  if (paths.dynamicConfigPath) {
    // TODO: Implement dynamic config modification
    throw new Error('Dynamic config modification not implemented yet');
  } else if (paths.staticConfigPath) {
    const appJson = JSON.parse(await fs.readFile(paths.staticConfigPath, 'utf8'));
    appJson.expo.updates = config.updates;
    await fs.writeFile(paths.staticConfigPath, JSON.stringify(appJson, null, 2));
  }
}
