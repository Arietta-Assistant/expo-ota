// This file is partially copied from eas-cli[https://github.com/expo/eas-cli] to ensure consistent user experience across the CLI.
import { Platform } from '@expo/config';
import fs from 'fs-extra';
import Joi from 'joi';
import fetch from 'node-fetch';
import path from 'path';

import { ExpoCredentials, getAuthExpoHeaders } from './auth';
import { RequestedPlatform } from './expoConfig';
import Log from './log';

const fileMetadataJoi = Joi.object({
  assets: Joi.array()
    .required()
    .items(Joi.object({ path: Joi.string().required(), ext: Joi.string().required() })),
  bundle: Joi.string().required(),
}).optional();
export const MetadataJoi = Joi.object({
  version: Joi.number().required(),
  bundler: Joi.string().required(),
  fileMetadata: Joi.object({
    android: fileMetadataJoi,
    ios: fileMetadataJoi,
    web: fileMetadataJoi,
  }).required(),
}).required();

type Metadata = {
  version: number;
  bundler: 'metro';
  fileMetadata: {
    [key in Platform]: { assets: { path: string; ext: string }[]; bundle: string };
  };
};

interface AssetToUpload {
  path: string;
  name: string;
  ext: string;
}

function loadMetadata(distRoot: string): Metadata {
  // eslint-disable-next-line
  const fileContent = fs.readFileSync(path.join(distRoot, 'metadata.json'), 'utf8');
  let metadata: Metadata;
  try {
    metadata = JSON.parse(fileContent);
  } catch (e: any) {
    Log.error(`Failed to read metadata.json: ${e.message}`);
    throw e;
  }
  const { error } = MetadataJoi.validate(metadata);
  if (error) {
    throw error;
  }
  // Check version and bundler by hand (instead of with Joi) so
  // more informative error messages can be returned.
  if (metadata.version !== 0) {
    throw new Error('Only bundles with metadata version 0 are supported');
  }
  if (metadata.bundler !== 'metro') {
    throw new Error('Only bundles created with Metro are currently supported');
  }
  const platforms = Object.keys(metadata.fileMetadata);
  if (platforms.length === 0) {
    Log.warn('No updates were exported for any platform');
  }
  Log.debug(`Loaded ${platforms.length} platform(s): ${platforms.join(', ')}`);
  return metadata;
}

export function computeFilesRequests(
  projectDir: string,
  requestedPlatform: RequestedPlatform
): AssetToUpload[] {
  // Look for exported files in dist directory
  const distDir = path.join(projectDir, 'dist');
  
  if (!fs.existsSync(distDir)) {
    Log.debug(`Dist directory not found at ${distDir}, project may not be exported yet`);
    throw new Error('Project has not been exported. Run "npx expo export" first or use the publish command.');
  }
  
  // Check for metadata.json in the dist directory
  const metadataPath = path.join(distDir, 'metadata.json');
  if (!fs.existsSync(metadataPath)) {
    Log.debug(`Metadata file not found at ${metadataPath}`);
    throw new Error('Metadata file not found. Make sure export completed successfully.');
  }
  
  const metadata = loadMetadata(distDir);
  Log.debug(`Loaded metadata: Platform keys: ${Object.keys(metadata.fileMetadata).join(', ')}`);
  
  // Initialize assets array with required files
  const assets: AssetToUpload[] = [
    { path: path.join(distDir, 'metadata.json'), name: 'metadata.json', ext: 'json' },
  ];
  
  // Add expo config if available
  const expoConfigPath = path.join(distDir, 'expoConfig.json');
  if (fs.existsSync(expoConfigPath)) {
    assets.push({ path: expoConfigPath, name: 'expoConfig.json', ext: 'json' });
  }
  
  // Process each platform
  for (const platform of Object.keys(metadata.fileMetadata) as Platform[]) {
    // Skip platforms that weren't requested
    if (requestedPlatform !== RequestedPlatform.All && requestedPlatform !== platform) {
      Log.debug(`Skipping platform ${platform} (requested: ${requestedPlatform})`);
      continue;
    }
    
    const platformMetadata = metadata.fileMetadata[platform];
    if (!platformMetadata) {
      Log.debug(`No metadata for platform ${platform}`);
      continue;
    }
    
    // Add the bundle file
    const bundle = platformMetadata.bundle;
    Log.debug(`Adding bundle for ${platform}: ${bundle}`);
    
    // Use the dist directory path for the bundle
    const bundlePath = path.join(distDir, bundle);
    if (fs.existsSync(bundlePath)) {
      assets.push({ path: bundlePath, name: bundle, ext: 'js' });
    } else {
      Log.warn(`Bundle file not found at ${bundlePath}`);
    }
    
    // Add all platform assets
    if (platformMetadata.assets && Array.isArray(platformMetadata.assets)) {
      Log.debug(`Found ${platformMetadata.assets.length} assets for platform ${platform}`);
      
      // Add each asset
      for (const asset of platformMetadata.assets) {
        const assetPath = path.join(distDir, asset.path);
        if (fs.existsSync(assetPath)) {
          assets.push({ path: assetPath, name: asset.path, ext: asset.ext });
        } else {
          Log.warn(`Asset file not found at ${assetPath}`);
        }
      }
    } else {
      Log.debug(`No assets found for platform ${platform}`);
    }
  }
  
  Log.debug(`Total files to upload: ${assets.length}`);
  return assets;
}

export interface RequestUploadUrlItem {
  requestUploadUrl: string;
  fileName: string;
  filePath: string;
}

export async function requestUploadUrls({
  body,
  requestUploadUrl,
  auth,
  runtimeVersion,
  platform,
  commitHash,
  buildNumber,
}: {
  body: { fileNames: string[] };
  requestUploadUrl: string;
  auth?: ExpoCredentials;
  runtimeVersion: string;
  platform: string;
  commitHash?: string;
  buildNumber?: string;
}): Promise<{ uploadRequests: RequestUploadUrlItem[]; updateId: string }> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  // Only add auth headers if auth is provided
  if (auth) {
    const authHeaders = getAuthExpoHeaders(auth);
    Object.assign(headers, authHeaders);
  }

  // Build query params including optional buildNumber as customUpdateId
  const queryParams = new URLSearchParams({
    runtimeVersion,
    platform,
    commitHash: commitHash || '',
  });
  
  // Add buildNumber as customUpdateId if provided
  if (buildNumber) {
    // Let the server generate the UUID but include the build number in the ID
    queryParams.append('buildNumber', buildNumber);
  }

  // Don't strip path information from filenames - send the full paths to ensure
  // the correct directory structure on the server
  const response = await fetch(
    `${requestUploadUrl}?${queryParams.toString()}`,
    {
      method: 'POST',
      headers,
      body: JSON.stringify({ fileNames: body.fileNames }),
    }
  );
  if (!response.ok) {
    throw new Error(`Failed to request upload URL: ${await response.text()}`);
  }
  return await response.json();
}
