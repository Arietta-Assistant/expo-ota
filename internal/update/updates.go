package update

import (
	"encoding/json"
	"expo-open-ota/config"
	"expo-open-ota/internal/bucket"
	cache2 "expo-open-ota/internal/cache"
	"expo-open-ota/internal/crypto"
	"expo-open-ota/internal/dashboard"
	"expo-open-ota/internal/types"
	"fmt"
	"io"
	"log"
	"mime"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func sortUpdates(updates []types.Update) []types.Update {
	sort.Slice(updates, func(i, j int) bool {
		return updates[i].CreatedAt > updates[j].CreatedAt
	})
	return updates
}

func GetAllUpdatesForRuntimeVersion(branch string, runtimeVersion string) ([]types.Update, error) {
	resolvedBucket := bucket.GetBucket()
	updates, errGetUpdates := resolvedBucket.GetUpdates(branch, runtimeVersion)
	if errGetUpdates != nil {
		if strings.Contains(errGetUpdates.Error(), "no more items in iterator") {
			// This is not a real error, just an empty result
			log.Printf("No updates found for branch %s and runtime version %s (iterator empty)",
				branch, runtimeVersion)
			return []types.Update{}, nil
		}
		return nil, errGetUpdates
	}

	if len(updates) == 0 {
		log.Printf("No updates found for branch %s and runtime version %s",
			branch, runtimeVersion)
	} else {
		log.Printf("Found %d updates for branch %s and runtime version %s",
			len(updates), branch, runtimeVersion)
	}

	updates = sortUpdates(updates)
	return updates, nil
}

func MarkUpdateAsChecked(update types.Update) error {
	cache := cache2.GetCache()
	branchesCacheKey := dashboard.ComputeGetBranchesCacheKey()
	runTimeVersionsCacheKey := dashboard.ComputeGetRuntimeVersionsCacheKey(update.Branch)
	updatesCacheKey := dashboard.ComputeGetUpdatesCacheKey(update.Branch, update.RuntimeVersion)
	cacheKeys := []string{ComputeLastUpdateCacheKey(update.Branch, update.RuntimeVersion), branchesCacheKey, runTimeVersionsCacheKey, updatesCacheKey}
	for _, cacheKey := range cacheKeys {
		cache.Delete(cacheKey)
	}
	resolvedBucket := bucket.GetBucket()
	reader := strings.NewReader(".check")
	_ = resolvedBucket.UploadFileIntoUpdate(update, ".check", reader)
	return nil
}

func IsUpdateValid(Update types.Update) bool {
	resolvedBucket := bucket.GetBucket()
	// Search for metadata.json file in the update instead of .check
	file, err := resolvedBucket.GetFile(Update.Branch, Update.RuntimeVersion, Update.UpdateId, "metadata.json")
	if err == nil && file != nil {
		defer file.Close()
		log.Printf("VALID UPDATE: %s (found metadata.json)", Update.UpdateId)
		return true
	}

	// Try alternate metadata file name
	file, err = resolvedBucket.GetFile(Update.Branch, Update.RuntimeVersion, Update.UpdateId, "update-metadata.json")
	if err == nil && file != nil {
		defer file.Close()
		log.Printf("VALID UPDATE: %s (found update-metadata.json)", Update.UpdateId)
		return true
	}

	// Try bundle.js as another fallback
	file, err = resolvedBucket.GetFile(Update.Branch, Update.RuntimeVersion, Update.UpdateId, "bundle.js")
	if err == nil && file != nil {
		defer file.Close()
		log.Printf("VALID UPDATE: %s (found bundle.js)", Update.UpdateId)
		return true
	}

	// Log detailed error for debugging
	log.Printf("Update %s validation failed: cannot find metadata or bundle files", Update.UpdateId)
	return false
}

func ComputeLastUpdateCacheKey(branch string, runtimeVersion string) string {
	return fmt.Sprintf("lastUpdate:%s:%s", branch, runtimeVersion)
}

func ComputeMetadataCacheKey(branch string, runtimeVersion string, updateId string) string {
	return fmt.Sprintf("metadata:%s:%s:%s", branch, runtimeVersion, updateId)
}

func ComputeUpdataManifestCacheKey(branch string, runtimeVersion string, updateId string, platform string) string {
	return fmt.Sprintf("manifest:%s:%s:%s:%s", branch, runtimeVersion, updateId, platform)
}

func ComputeManifestAssetCacheKey(update types.Update, assetPath string) string {
	return fmt.Sprintf("asset:%s:%s:%s:%s", update.Branch, update.RuntimeVersion, update.UpdateId, assetPath)
}

func VerifyUploadedUpdate(update types.Update) error {
	metadata, errMetadata := GetMetadata(update)
	if errMetadata != nil {
		return errMetadata
	}
	if metadata.MetadataJSON.FileMetadata.IOS.Bundle == "" && metadata.MetadataJSON.FileMetadata.Android.Bundle == "" {
		return fmt.Errorf("missing bundle path in metadata")
	}
	files := []string{}
	if metadata.MetadataJSON.FileMetadata.IOS.Bundle != "" {
		files = append(files, metadata.MetadataJSON.FileMetadata.IOS.Bundle)
		for _, asset := range metadata.MetadataJSON.FileMetadata.IOS.Assets {
			files = append(files, asset.Path)
		}
	}
	if metadata.MetadataJSON.FileMetadata.Android.Bundle != "" {
		files = append(files, metadata.MetadataJSON.FileMetadata.Android.Bundle)
		for _, asset := range metadata.MetadataJSON.FileMetadata.Android.Assets {
			files = append(files, asset.Path)
		}
	}

	resolvedBucket := bucket.GetBucket()
	for _, file := range files {
		_, err := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, file)
		if err != nil {
			return fmt.Errorf("missing file: %s in update", file)
		}
	}
	return nil
}

func GetUpdate(branch string, runtimeVersion string, updateId string) (*types.Update, error) {
	updateIdInt64, err := strconv.ParseInt(updateId, 10, 64)
	if err != nil {
		return nil, err
	}
	return &types.Update{
		Branch:         branch,
		RuntimeVersion: runtimeVersion,
		UpdateId:       updateId,
		CreatedAt:      time.Duration(updateIdInt64) * time.Millisecond,
	}, nil
}

func AreUpdatesIdentical(update1, update2 types.Update, platform string) (bool, error) {
	metadata1, errMetadata1 := GetMetadata(update1)
	if errMetadata1 != nil {
		return false, errMetadata1
	}
	metadata2, errMetadata2 := GetMetadata(update2)
	if errMetadata2 != nil {
		return false, errMetadata2
	}
	update1Manifest, errManifest1 := ComposeUpdateManifest(&metadata1, update1, platform)
	if errManifest1 != nil {
		return false, errManifest1
	}
	update2Manifest, errManifest2 := ComposeUpdateManifest(&metadata2, update2, platform)
	if errManifest2 != nil {
		return false, errManifest2
	}
	if update1Manifest.LaunchAsset.Hash != update2Manifest.LaunchAsset.Hash {
		return false, nil
	}
	if len(update2Manifest.Assets) != len(update1Manifest.Assets) {
		return false, nil
	}
	for i, asset := range update1Manifest.Assets {
		if asset.Hash != update2Manifest.Assets[i].Hash {
			return false, nil
		}
	}
	return true, nil
}

func GetLatestUpdateBundlePathForRuntimeVersion(branch string, runtimeVersion string, buildNumber string) (*types.Update, error) {
	cache := cache2.GetCache()
	cacheKey := fmt.Sprintf(ComputeLastUpdateCacheKey(branch, runtimeVersion))
	if buildNumber != "" {
		cacheKey = fmt.Sprintf("%s:%s", cacheKey, buildNumber)
	}

	log.Printf("Searching for updates in branch=%s, runtimeVersion=%s, buildNumber=%s", branch, runtimeVersion, buildNumber)

	// Get all updates regardless of cache to ensure we find the latest build
	updates, err := GetAllUpdatesForRuntimeVersion(branch, runtimeVersion)
	if err != nil {
		log.Printf("Error getting updates: %v", err)
		return nil, err
	}
	log.Printf("Found %d updates (before validation)", len(updates))

	// Debug - print all updates
	for i, update := range updates {
		log.Printf("Update #%d: ID=%s, CreatedAt=%v", i+1, update.UpdateId, update.CreatedAt)
	}

	// Check if we have any updates with higher build numbers
	highestBuildUpdate := (*types.Update)(nil)
	highestBuildNum := -1

	// Only check cache if we're not forcing a refresh
	if cachedValue := cache.Get(cacheKey); cachedValue != "" {
		var cachedUpdate types.Update
		err := json.Unmarshal([]byte(cachedValue), &cachedUpdate)
		if err == nil {
			log.Printf("Found cached update: %s", cachedUpdate.UpdateId)
			cachedBuildNum := extractBuildNumber(cachedUpdate.UpdateId)
			if cachedBuildNum > 0 {
				highestBuildNum = cachedBuildNum
				highestBuildUpdate = &cachedUpdate
			}
		} else {
			log.Printf("Error parsing cached update: %v", err)
		}
	}

	// Process all updates to find the highest build number
	filteredUpdates := make([]types.Update, 0)
	for _, update := range updates {
		// Only process updates with build-NUMBER format
		buildNum := extractBuildNumber(update.UpdateId)
		log.Printf("Checking update: %s (build number: %d)", update.UpdateId, buildNum)

		if IsUpdateValid(update) {
			filteredUpdates = append(filteredUpdates, update)
			log.Printf("VALID UPDATE: %s", update.UpdateId)

			if buildNum > highestBuildNum {
				log.Printf("Found higher build number: %d > %d", buildNum, highestBuildNum)
				highestBuildNum = buildNum
				highestBuildUpdate = &update
			}
		} else {
			log.Printf("INVALID UPDATE: %s", update.UpdateId)
		}
	}

	// If no updates found, return nil
	if len(filteredUpdates) == 0 {
		log.Printf("No valid updates found for %s/%s", branch, runtimeVersion)
		return nil, nil
	}

	log.Printf("Found %d valid updates", len(filteredUpdates))

	// Use the highest build update if found, otherwise use the first valid update
	var latest *types.Update
	if highestBuildUpdate != nil {
		latest = highestBuildUpdate
		log.Printf("SELECTED UPDATE WITH HIGHEST BUILD NUMBER: %s (build %d)", latest.UpdateId, highestBuildNum)
	} else {
		latest = &filteredUpdates[0]
		log.Printf("SELECTED UPDATE (first in sorted list): %s", latest.UpdateId)
	}

	// Update the cache with the latest update
	cacheValue, _ := json.Marshal(*latest)
	ttl := 1800
	_ = cache.Set(cacheKey, string(cacheValue), &ttl)
	return latest, nil
}

// extractBuildNumber extracts build number from a string like "build-NUMBER-updateid" or just "12"
// Returns the build number as an integer, or -1 if not found
func extractBuildNumber(str string) int {
	// Only extract from build-NUMBER format
	if strings.HasPrefix(str, "build-") {
		parts := strings.SplitN(strings.TrimPrefix(str, "build-"), "-", 2)
		if len(parts) > 0 {
			num, err := strconv.Atoi(parts[0])
			if err == nil {
				return num
			}
		}
	}

	// For direct number format (less common)
	num, err := strconv.Atoi(str)
	if err == nil {
		return num
	}

	// Not a valid build number format
	return -1
}

func GetUpdateType(update types.Update) types.UpdateType {
	resolvedBucket := bucket.GetBucket()
	file, err := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, "rollback")
	if err == nil && file != nil {
		defer file.Close()
		return types.Rollback
	}
	return types.NormalUpdate
}

func GetExpoConfig(update types.Update) (json.RawMessage, error) {
	resolvedBucket := bucket.GetBucket()
	file, err := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, "expoConfig.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var expoConfig json.RawMessage
	err = json.NewDecoder(file).Decode(&expoConfig)
	if err != nil {
		return nil, err
	}
	return expoConfig, nil
}

func GetMetadata(update types.Update) (types.UpdateMetadata, error) {
	metadataCacheKey := ComputeMetadataCacheKey(update.Branch, update.RuntimeVersion, update.UpdateId)
	cache := cache2.GetCache()
	if cachedValue := cache.Get(metadataCacheKey); cachedValue != "" {
		var metadata types.UpdateMetadata
		err := json.Unmarshal([]byte(cachedValue), &metadata)
		if err != nil {
			return types.UpdateMetadata{}, err
		}
		return metadata, nil
	}
	resolvedBucket := bucket.GetBucket()
	file, errFile := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, "metadata.json")
	if errFile != nil {
		return types.UpdateMetadata{}, errFile
	}
	defer file.Close()

	// Read the raw content for debugging
	rawContent, err := io.ReadAll(file)
	if err != nil {
		return types.UpdateMetadata{}, err
	}

	// Log the raw metadata JSON
	log.Printf("DEBUG: Raw metadata for update %s/%s/%s: %s",
		update.Branch, update.RuntimeVersion, update.UpdateId, string(rawContent))

	// Reset reader
	file, errFile = resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, "metadata.json")
	if errFile != nil {
		return types.UpdateMetadata{}, errFile
	}
	defer file.Close()

	var metadata types.UpdateMetadata
	var metadataJson types.MetadataObject
	err = json.NewDecoder(file).Decode(&metadataJson)
	if err != nil {
		fmt.Println("error decoding metadata json:", err)
		return types.UpdateMetadata{}, err
	}

	// Debug log the structure
	extraData, _ := json.MarshalIndent(metadataJson.Extra, "", "  ")
	log.Printf("DEBUG: Extra fields in metadata: %s", string(extraData))

	metadata.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	metadata.MetadataJSON = metadataJson
	stringifiedMetadata, err := json.Marshal(metadata.MetadataJSON)
	if err != nil {
		return types.UpdateMetadata{}, err
	}
	id, errHash := crypto.CreateHash(stringifiedMetadata, "sha256", "hex")

	if errHash != nil {
		return types.UpdateMetadata{}, errHash
	}
	metadata.ID = id
	cacheValue, err := json.Marshal(metadata)
	if err != nil {
		return metadata, nil
	}
	err = cache.Set(metadataCacheKey, string(cacheValue), nil)
	return metadata, nil
}

func BuildFinalManifestAssetUrlURL(baseURL, assetFilePath, runtimeVersion, platform string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Add query parameters to retain compatibility
	query := url.Values{}
	query.Set("asset", assetFilePath)
	query.Set("runtimeVersion", runtimeVersion)
	query.Set("platform", platform)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}

func GetAssetEndpoint() string {
	return config.GetEnv("BASE_URL") + "/api/update/assets"
}

func shapeManifestAsset(update types.Update, asset *types.Asset, isLaunchAsset bool, platform string) (types.ManifestAsset, error) {
	cacheKey := ComputeManifestAssetCacheKey(update, asset.Path)
	cache := cache2.GetCache()
	if cachedValue := cache.Get(cacheKey); cachedValue != "" {
		var manifestAsset types.ManifestAsset
		err := json.Unmarshal([]byte(cachedValue), &manifestAsset)
		if err != nil {
			return types.ManifestAsset{}, err
		}
		return manifestAsset, nil
	}
	resolvedBucket := bucket.GetBucket()

	// Path to try
	assetPath := asset.Path

	// Try to get the file at specified path
	assetFile, err := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, assetPath)

	// If file not found and it's a bundle file, try alternate locations
	if err != nil && isLaunchAsset {
		log.Printf("Bundle file not found at path %s, trying alternative locations", assetPath)

		// Try common alternative locations
		alternativePaths := []string{
			// Custom path for app bundles
			fmt.Sprintf("bundles/%s-bundle.js", platform),
			// Default names
			"bundle.js",
			"index.js",
			fmt.Sprintf("%s-bundle.js", platform),
		}

		// Try extracting filename from the path and looking for it directly
		parts := strings.Split(assetPath, "/")
		if len(parts) > 0 {
			filename := parts[len(parts)-1]
			alternativePaths = append(alternativePaths, filename)
		}

		// Try each alternative path
		for _, altPath := range alternativePaths {
			log.Printf("Trying alternative path for bundle: %s", altPath)
			altFile, altErr := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, altPath)
			if altErr == nil {
				log.Printf("Found bundle at alternative path: %s", altPath)
				assetFile = altFile
				assetPath = altPath // Update the path for URL generation
				err = nil
				break
			}
		}
	}

	if err != nil {
		return types.ManifestAsset{}, err
	}
	defer assetFile.Close()

	// Read the file content
	content, err := io.ReadAll(assetFile)
	if err != nil {
		return types.ManifestAsset{}, err
	}

	assetHash, errHash := crypto.CreateHash(content, "sha256", "base64")
	if errHash != nil {
		return types.ManifestAsset{}, errHash
	}
	urlEncodedHash := crypto.GetBase64URLEncoding(assetHash)
	key, errKey := crypto.CreateHash(content, "md5", "hex")
	if errKey != nil {
		return types.ManifestAsset{}, errKey
	}

	keyExtensionSuffix := asset.Ext
	if isLaunchAsset {
		keyExtensionSuffix = "bundle"
	}
	keyExtensionSuffix = "." + keyExtensionSuffix
	contentType := "application/javascript"
	if isLaunchAsset {
		contentType = mime.TypeByExtension(asset.Ext)
	}
	finalUrl, errUrl := BuildFinalManifestAssetUrlURL(GetAssetEndpoint(), assetPath, update.RuntimeVersion, platform)
	if errUrl != nil {
		return types.ManifestAsset{}, errUrl
	}
	manifestAsset := types.ManifestAsset{
		Hash:          urlEncodedHash,
		Key:           key,
		FileExtension: keyExtensionSuffix,
		ContentType:   contentType,
		Url:           finalUrl,
	}
	cacheValue, err := json.Marshal(manifestAsset)
	if err != nil {
		return manifestAsset, nil
	}
	_ = cache.Set(cacheKey, string(cacheValue), nil)
	return manifestAsset, nil
}

func ComposeUpdateManifest(
	metadata *types.UpdateMetadata,
	update types.Update,
	platform string,
) (types.UpdateManifest, error) {
	log.Printf("MANIFEST-DEBUG: Creating manifest for update ID: %s, platform: %s", update.UpdateId, platform)
	cache := cache2.GetCache()
	cacheKey := ComputeUpdataManifestCacheKey(update.Branch, update.RuntimeVersion, update.UpdateId, platform)
	if cachedValue := cache.Get(cacheKey); cachedValue != "" {
		var manifest types.UpdateManifest
		err := json.Unmarshal([]byte(cachedValue), &manifest)
		if err != nil {
			return types.UpdateManifest{}, err
		}
		return manifest, nil
	}
	expoConfig, errConfig := GetExpoConfig(update)
	if errConfig != nil {
		return types.UpdateManifest{}, errConfig
	}

	var platformSpecificMetadata types.PlatformMetadata
	switch platform {
	case "ios":
		platformSpecificMetadata = metadata.MetadataJSON.FileMetadata.IOS
		log.Printf("DEBUG: iOS bundle path: %s", platformSpecificMetadata.Bundle)
	case "android":
		platformSpecificMetadata = metadata.MetadataJSON.FileMetadata.Android
		log.Printf("DEBUG: Android bundle path: %s", platformSpecificMetadata.Bundle)
	}

	// Debug log all metadata
	metadataBytes, _ := json.MarshalIndent(metadata.MetadataJSON, "", "  ")
	log.Printf("DEBUG: Complete metadata for update %s:\n%s", update.UpdateId, string(metadataBytes))

	if platformSpecificMetadata.Bundle == "" {
		log.Printf("ERROR: Missing bundle path for platform %s in update %s", platform, update.UpdateId)
		return types.UpdateManifest{}, fmt.Errorf("missing bundle path for platform %s", platform)
	}

	var (
		assets = make([]types.ManifestAsset, len(platformSpecificMetadata.Assets))
		errs   = make(chan error, len(platformSpecificMetadata.Assets))
		wg     sync.WaitGroup
	)

	// Process assets in parallel
	for i, asset := range platformSpecificMetadata.Assets {
		wg.Add(1)
		go func(i int, asset types.Asset) {
			defer wg.Done()
			manifestAsset, err := shapeManifestAsset(update, &asset, false, platform)
			if err != nil {
				errs <- err
				return
			}
			assets[i] = manifestAsset
		}(i, asset)
	}

	// Wait for all assets to be processed
	wg.Wait()
	close(errs)

	// Check for any errors
	for err := range errs {
		if err != nil {
			return types.UpdateManifest{}, err
		}
	}

	// Process launch asset
	launchAsset, err := shapeManifestAsset(update, &types.Asset{
		Path: platformSpecificMetadata.Bundle,
		Ext:  "js",
	}, true, platform)
	if err != nil {
		return types.UpdateManifest{}, err
	}

	// Create the manifest with build number in extra
	manifest := types.UpdateManifest{
		Id:             crypto.ConvertSHA256HashToUUID(metadata.ID),
		CreatedAt:      metadata.CreatedAt,
		RunTimeVersion: update.RuntimeVersion,
		Metadata:       json.RawMessage("{}"),
		Extra: types.ExtraManifestData{
			ExpoClient:  expoConfig,
			Branch:      update.Branch,
			BuildNumber: update.BuildNumber, // Include build number in extra
		},
		Assets:      assets,
		LaunchAsset: launchAsset,
	}

	// Cache the manifest
	cacheValue, err := json.Marshal(manifest)
	if err != nil {
		return manifest, nil
	}
	_ = cache.Set(cacheKey, string(cacheValue), nil)

	return manifest, nil
}

func CreateRollbackDirective(update types.Update) (types.RollbackDirective, error) {
	resolvedBucket := bucket.GetBucket()
	object, err := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, "rollback")
	if err != nil {
		return types.RollbackDirective{}, err
	}
	defer object.Close()

	content, err := io.ReadAll(object)
	if err != nil {
		return types.RollbackDirective{}, err
	}

	var rollbackDirective types.RollbackDirective
	err = json.Unmarshal(content, &rollbackDirective)
	if err != nil {
		return types.RollbackDirective{}, err
	}

	return rollbackDirective, nil
}

func CreateNoUpdateAvailableDirective() types.NoUpdateAvailableDirective {
	return types.NoUpdateAvailableDirective{
		Type: "noUpdateAvailable",
	}
}

func RetrieveUpdateCommitHashAndPlatform(update types.Update) (string, string, error) {
	metadata, err := GetMetadata(update)
	if err != nil {
		return "", "", err
	}

	commitHash, ok := metadata.MetadataJSON.Extra["commitHash"].(string)
	if !ok {
		return "", "", fmt.Errorf("commitHash not found in metadata")
	}

	platform, ok := metadata.MetadataJSON.Extra["platform"].(string)
	if !ok {
		return "", "", fmt.Errorf("platform not found in metadata")
	}

	return commitHash, platform, nil
}

func CreateUpdate(update types.Update) error {
	resolvedBucket := bucket.GetBucket()
	metadata := types.MetadataObject{
		Version: 0,
		Bundler: "metro",
		FileMetadata: types.FileMetadata{
			Android: types.PlatformMetadata{
				Bundle: "",
				Assets: []types.Asset{},
			},
			IOS: types.PlatformMetadata{
				Bundle: "",
				Assets: []types.Asset{},
			},
		},
		Extra: map[string]interface{}{
			"commitHash": update.CommitHash,
			"updateCode": update.BuildNumber,
			"platform":   update.Platform,
		},
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("error marshalling metadata: %w", err)
	}

	reader := strings.NewReader(string(metadataBytes))
	return resolvedBucket.UploadFileIntoUpdate(update, "metadata.json", reader)
}

// DumpSpecificUpdateMetadata dumps the complete metadata for the specific update ID
func DumpSpecificUpdateMetadata() {
	specificUpdateId := "95dd7166-1c74-4251-ae37-5fab5eafa74c"
	log.Printf("Attempting to dump complete metadata for update ID: %s", specificUpdateId)

	// First try with the direct method that doesn't rely on branch discovery
	resolvedBucket := bucket.GetBucket()

	// Use type assertion to check if the bucket is a FirebaseBucket
	if firebaseBucket, ok := resolvedBucket.(*bucket.FirebaseBucket); ok {
		log.Printf("Using direct Firebase bucket access method")
		firebaseBucket.GetDirectMetadata(specificUpdateId)
		return
	}

	log.Printf("Bucket does not support direct access, falling back to branch discovery method")

	// Fallback to the original method if the bucket doesn't support direct access
	// Get all branches
	branches, err := resolvedBucket.GetBranches()
	if err != nil {
		log.Printf("Error getting branches: %v", err)
		log.Printf("Consider checking Firebase credentials and permissions")
		return
	}

	log.Printf("Checking for update %s in branches: %v", specificUpdateId, branches)

	for _, branch := range branches {
		// Get runtime versions for each branch
		runtimeVersions, err := resolvedBucket.GetRuntimeVersions(branch)
		if err != nil {
			log.Printf("Error getting runtime versions for branch %s: %v", branch, err)
			continue
		}

		log.Printf("Checking branch %s with runtime versions: %v", branch, runtimeVersions)

		for _, rv := range runtimeVersions {
			// Get updates for this branch and runtime version
			updates, err := resolvedBucket.GetUpdates(branch, rv.RuntimeVersion)
			if err != nil {
				log.Printf("Error getting updates for %s/%s: %v", branch, rv.RuntimeVersion, err)
				continue
			}

			// Check if our specific update ID is in the list
			for _, update := range updates {
				if update.UpdateId == specificUpdateId {
					log.Printf("Found update %s in branch %s, runtime version %s", specificUpdateId, branch, rv.RuntimeVersion)

					// Get and dump the metadata
					metadata, err := GetMetadata(update)
					if err != nil {
						log.Printf("Error getting metadata: %v", err)
						return
					}

					// Marshal the metadata to pretty JSON
					jsonData, err := json.MarshalIndent(metadata, "", "  ")
					if err != nil {
						log.Printf("Error marshalling metadata: %v", err)
						return
					}

					log.Printf("Complete metadata for update %s:\n%s", specificUpdateId, string(jsonData))

					// Also dump the MetadataJSON field which has the actual content
					jsonData2, err := json.MarshalIndent(metadata.MetadataJSON, "", "  ")
					if err != nil {
						log.Printf("Error marshalling metadata.MetadataJSON: %v", err)
						return
					}

					log.Printf("MetadataJSON for update %s:\n%s", specificUpdateId, string(jsonData2))

					// Dump Extra field specifically
					jsonData3, err := json.MarshalIndent(metadata.MetadataJSON.Extra, "", "  ")
					if err != nil {
						log.Printf("Error marshalling metadata.MetadataJSON.Extra: %v", err)
						return
					}

					log.Printf("Extra fields for update %s:\n%s", specificUpdateId, string(jsonData3))

					return
				}
			}
		}
	}

	log.Printf("Could not find update %s in any branch or runtime version", specificUpdateId)
}
