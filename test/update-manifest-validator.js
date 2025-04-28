/**
 * This script tests the update manifest endpoint to ensure it returns valid JSON
 * with the correct structure expected by Expo updates.
 * 
 * Usage:
 * node update-manifest-validator.js
 */

const https = require('https');
const http = require('http');

// Configuration - adjust these values to match your server
const config = {
  baseUrl: 'https://expo-ota-production.up.railway.app', // Your server URL
  branch: 'ota-updates', // The branch/channel name
  runtimeVersion: '1.0.2', // The runtime version to check
  platform: 'ios', // The platform to test (ios or android)
  protocol: '1', // The expo protocol version
  buildNumber: 'build-5', // The build number to test
};

// Function to make HTTP request with proper headers
function fetchUpdateManifest() {
  return new Promise((resolve, reject) => {
    // Determine if using http or https
    const client = config.baseUrl.startsWith('https') ? https : http;
    
    // Construct request options
    const url = `${config.baseUrl}/api/update/manifest/${config.branch}/${config.runtimeVersion}`;
    console.log(`Fetching manifest from: ${url}`);
    
    const options = {
      method: 'GET',
      headers: {
        'expo-platform': config.platform,
        'expo-runtime-version': config.runtimeVersion,
        'expo-channel-name': config.branch,
        'expo-protocol-version': config.protocol,
        'expo-build-number': config.buildNumber,
        'Expo-Extra-Params': `expo-build-number="${config.buildNumber}"`
      }
    };
    
    // Make the request
    const req = client.request(url, options, (res) => {
      const chunks = [];
      
      // Log status code and headers
      console.log(`Status Code: ${res.statusCode}`);
      console.log('Response Headers:', res.headers);
      
      res.on('data', (chunk) => {
        chunks.push(chunk);
      });
      
      res.on('end', () => {
        try {
          const body = Buffer.concat(chunks).toString();
          
          if (res.statusCode >= 400) {
            console.error('Error response:', body);
            reject(new Error(`Request failed with status ${res.statusCode}`));
            return;
          }
          
          // Try to parse as JSON
          try {
            const jsonResponse = JSON.parse(body);
            resolve({ headers: res.headers, body: jsonResponse });
          } catch (jsonError) {
            console.error('Failed to parse response as JSON:', jsonError);
            console.log('Raw response:', body);
            reject(jsonError);
          }
        } catch (error) {
          reject(error);
        }
      });
    });
    
    req.on('error', (error) => {
      reject(error);
    });
    
    req.end();
  });
}

// Function to validate the manifest structure
function validateManifest(manifest) {
  console.log('\n=== Manifest Validation ===');
  
  // Required top-level fields
  const requiredFields = ['id', 'createdAt', 'runtimeVersion', 'assets', 'launchAsset'];
  const missingFields = requiredFields.filter(field => !manifest[field]);
  
  if (missingFields.length > 0) {
    console.error('❌ Missing required fields:', missingFields.join(', '));
  } else {
    console.log('✅ All required top-level fields are present');
  }
  
  // Check LaunchAsset structure
  if (manifest.launchAsset) {
    const launchAsset = manifest.launchAsset;
    console.log('\n--- Launch Asset ---');
    console.log('Hash:', launchAsset.hash);
    console.log('Key:', launchAsset.key);
    console.log('FileExtension:', launchAsset.fileExtension);
    console.log('ContentType:', launchAsset.contentType);
    console.log('URL:', launchAsset.url);
    
    if (launchAsset.contentType !== 'application/javascript') {
      console.error('❌ LaunchAsset should have contentType "application/javascript"');
    } else {
      console.log('✅ LaunchAsset has correct content type');
    }
  }
  
  // Check assets array
  if (Array.isArray(manifest.assets)) {
    console.log(`\n--- Assets (${manifest.assets.length}) ---`);
    
    const assetsByType = {};
    manifest.assets.forEach(asset => {
      const type = asset.contentType || 'unknown';
      assetsByType[type] = (assetsByType[type] || 0) + 1;
    });
    
    console.log('Assets by content type:');
    Object.entries(assetsByType).forEach(([type, count]) => {
      console.log(`- ${type}: ${count}`);
    });
    
    // Check for potential issues
    const assetExtensions = new Set();
    const assetKeys = new Set();
    
    let hasIssues = false;
    manifest.assets.forEach((asset, index) => {
      // Check for duplicate keys
      if (assetKeys.has(asset.key)) {
        console.error(`❌ Duplicate asset key found: ${asset.key} at index ${index}`);
        hasIssues = true;
      }
      assetKeys.add(asset.key);
      
      // Check for consistency in file extensions
      const ext = asset.fileExtension;
      if (ext) {
        assetExtensions.add(ext);
        
        // Check if content type matches extension
        if (ext.includes('.png') && asset.contentType !== 'image/png') {
          console.error(`❌ Asset with .png extension has incorrect content type: ${asset.contentType} (index ${index})`);
          hasIssues = true;
        }
        if ((ext.includes('.jpg') || ext.includes('.jpeg')) && asset.contentType !== 'image/jpeg') {
          console.error(`❌ Asset with .jpg extension has incorrect content type: ${asset.contentType} (index ${index})`);
          hasIssues = true;
        }
      }
    });
    
    if (!hasIssues) {
      console.log('✅ No asset structure issues found');
    }
  }
  
  console.log('\n=== End of Validation ===');
}

// Main function to run the test
async function runTest() {
  try {
    console.log(`Testing update manifest for ${config.branch}/${config.runtimeVersion} (${config.platform})`);
    
    const result = await fetchUpdateManifest();
    
    console.log(`\nSuccessfully fetched manifest (${Object.keys(result.body).length} top-level keys)`);
    validateManifest(result.body);
    
    // Output a small preview of the manifest
    console.log('\nManifest Preview:');
    const { id, createdAt, runtimeVersion } = result.body;
    console.log({ id, createdAt, runtimeVersion });
    console.log('...and more fields');
    
  } catch (error) {
    console.error('Test failed:', error);
  }
}

// Run the test
runTest(); 