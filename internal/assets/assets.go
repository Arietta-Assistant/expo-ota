package assets

import (
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/cdn"
	"expo-open-ota/internal/types"
	"expo-open-ota/internal/update"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AssetsRequest struct {
	Branch         string
	AssetName      string
	RuntimeVersion string
	Platform       string
	RequestID      string
}

type AssetsResponse struct {
	StatusCode  int
	Headers     map[string]string
	Body        []byte
	ContentType string
	URL         string
	Path        string
	Size        int64
}

func getAssetMetadata(req AssetsRequest, returnAsset bool) (AssetsResponse, *types.BucketFile, string, error) {
	requestID := req.RequestID

	if req.AssetName == "" {
		log.Printf("[RequestID: %s] No asset name provided", requestID)
		return AssetsResponse{StatusCode: http.StatusBadRequest, Body: []byte("No asset name provided")}, nil, "", nil
	}

	if req.Platform == "" || (req.Platform != "ios" && req.Platform != "android" && req.Platform != "all") {
		log.Printf("[RequestID: %s] Invalid platform: %s", requestID, req.Platform)
		return AssetsResponse{StatusCode: http.StatusBadRequest, Body: []byte("Invalid platform")}, nil, "", nil
	}

	if req.RuntimeVersion == "" {
		log.Printf("[RequestID: %s] No runtime version provided", requestID)
		return AssetsResponse{StatusCode: http.StatusBadRequest, Body: []byte("No runtime version provided")}, nil, "", nil
	}

	// Get all updates for this runtime version
	allUpdates, err := update.GetAllUpdatesForRuntimeVersion(req.Branch, req.RuntimeVersion)
	if err != nil || len(allUpdates) == 0 {
		log.Printf("[RequestID: %s] No updates found for runtimeVersion: %s, error: %v", requestID, req.RuntimeVersion, err)
		return AssetsResponse{StatusCode: http.StatusNotFound, Body: []byte("No updates found")}, nil, "", nil
	}

	// Sort updates by build number (descending order - newest first)
	sort.Slice(allUpdates, func(i, j int) bool {
		// Extract build numbers if possible
		buildNumI := 0
		buildNumJ := 0

		// Try to parse build numbers from BuildNumber field
		if allUpdates[i].BuildNumber != "" {
			if num, err := strconv.Atoi(allUpdates[i].BuildNumber); err == nil {
				buildNumI = num
			}
		}

		if allUpdates[j].BuildNumber != "" {
			if num, err := strconv.Atoi(allUpdates[j].BuildNumber); err == nil {
				buildNumJ = num
			}
		}

		// If both have build numbers, compare them
		if buildNumI > 0 && buildNumJ > 0 {
			return buildNumI > buildNumJ // Descending order
		}

		// If build numbers can't be compared, try to extract from UpdateId for "build-X-..." format
		if strings.HasPrefix(allUpdates[i].UpdateId, "build-") && strings.HasPrefix(allUpdates[j].UpdateId, "build-") {
			partsI := strings.SplitN(allUpdates[i].UpdateId, "-", 3)
			partsJ := strings.SplitN(allUpdates[j].UpdateId, "-", 3)

			if len(partsI) >= 2 && len(partsJ) >= 2 {
				if numI, err := strconv.Atoi(partsI[1]); err == nil {
					if numJ, err := strconv.Atoi(partsJ[1]); err == nil {
						return numI > numJ // Descending order
					}
				}
			}
		}

		// Default to comparing UpdateId as strings (less reliable)
		return allUpdates[i].UpdateId > allUpdates[j].UpdateId
	})

	// For debugging purposes, log all available updates in sorted order
	log.Printf("[RequestID: %s] Found %d updates for runtimeVersion: %s (sorted newest first)",
		requestID, len(allUpdates), req.RuntimeVersion)
	for i, update := range allUpdates {
		log.Printf("[RequestID: %s] Sorted update %d: ID=%s, BuildNumber=%s",
			requestID, i+1, update.UpdateId, update.BuildNumber)
	}

	// Use the newest update
	latestUpdate := allUpdates[0]
	log.Printf("[RequestID: %s] Using latest update: ID=%s, BuildNumber=%s",
		requestID, latestUpdate.UpdateId, latestUpdate.BuildNumber)

	// For non-asset return cases (just metadata)
	if !returnAsset {
		headers := map[string]string{
			"expo-protocol-version": "1",
			"expo-sfv-version":      "0",
			"Cache-Control":         "public, max-age=31536000",
		}
		return AssetsResponse{
			StatusCode: http.StatusOK,
			Headers:    headers,
		}, nil, latestUpdate.UpdateId, nil
	}

	// Get the metadata for this update
	metadata, err := update.GetMetadata(latestUpdate)
	if err != nil {
		log.Printf("[RequestID: %s] Error getting metadata: %v", requestID, err)
		return AssetsResponse{StatusCode: http.StatusInternalServerError, Body: []byte("Error getting metadata")}, nil, "", nil
	}

	// Determine which platform metadata to use
	actualPlatform := req.Platform
	if req.Platform == "all" {
		log.Printf("[RequestID: %s] Platform 'all' specified in asset request, using 'ios'", requestID)
		actualPlatform = "ios"
	}

	// Get platform-specific metadata
	var platformMetadata types.PlatformMetadata
	switch actualPlatform {
	case "android":
		platformMetadata = metadata.MetadataJSON.FileMetadata.Android
	case "ios":
		platformMetadata = metadata.MetadataJSON.FileMetadata.IOS
	default:
		return AssetsResponse{StatusCode: http.StatusBadRequest, Body: []byte("Platform not supported")}, nil, "", nil
	}

	// Identify if this is a launch asset (main JS bundle)
	isLaunchAsset := false
	var assetPath string
	var contentType string

	// Log the bundle and requested asset for debugging
	log.Printf("[RequestID: %s] Platform bundle path: %s", requestID, platformMetadata.Bundle)
	log.Printf("[RequestID: %s] Requested asset path: %s", requestID, req.AssetName)

	// Check if this is the launch asset (main JS bundle)
	if req.AssetName == platformMetadata.Bundle {
		log.Printf("[RequestID: %s] Request is for the launch asset (main bundle)", requestID)
		isLaunchAsset = true
		assetPath = platformMetadata.Bundle
		contentType = "application/javascript"
	} else {
		// Look for the asset in the assets list
		found := false
		for _, asset := range platformMetadata.Assets {
			if asset.Path == req.AssetName {
				log.Printf("[RequestID: %s] Found asset in metadata: %s (ext: %s)", requestID, asset.Path, asset.Ext)
				found = true
				contentType = mime.TypeByExtension("." + string(asset.Ext))
				assetPath = asset.Path
				break
			}
		}

		if !found {
			log.Printf("[RequestID: %s] Asset not found in metadata", requestID)
			// Log all available assets for debugging
			log.Printf("[RequestID: %s] Available assets in metadata:", requestID)
			for i, asset := range platformMetadata.Assets {
				log.Printf("[RequestID: %s]   Asset %d: %s", requestID, i+1, asset.Path)
			}
			return AssetsResponse{StatusCode: http.StatusNotFound, Body: []byte("Asset not found in metadata")}, nil, "", nil
		}
	}

	// Get the bucket
	resolvedBucket := bucket.GetBucket()

	// Try to retrieve the asset file
	log.Printf("[RequestID: %s] ASSET-DEBUG: Looking for asset %s in update %s/%s/%s",
		requestID, assetPath, latestUpdate.Branch, latestUpdate.RuntimeVersion, latestUpdate.UpdateId)

	// Create a function to try different asset path variations
	tryAssetPaths := func() (io.ReadCloser, error) {
		// Paths to try in order
		pathsToTry := []string{
			assetPath, // Original path from metadata
		}

		// For launch assets, add additional path variations
		if isLaunchAsset {
			// Remove _expo prefix if present
			if strings.HasPrefix(assetPath, "_expo/") {
				pathsToTry = append(pathsToTry, strings.TrimPrefix(assetPath, "_expo/"))
			}

			// Add variant with bundles/ prefix
			pathsToTry = append(pathsToTry, "bundles/"+assetPath)

			// Extract filename only
			parts := strings.Split(assetPath, "/")
			if len(parts) > 0 {
				pathsToTry = append(pathsToTry, parts[len(parts)-1])
			}
		} else {
			// For regular assets, try without assets/ prefix
			if strings.HasPrefix(assetPath, "assets/") {
				pathsToTry = append(pathsToTry, strings.TrimPrefix(assetPath, "assets/"))
			}

			// Try just the asset hash (last part)
			parts := strings.Split(assetPath, "/")
			if len(parts) > 0 {
				pathsToTry = append(pathsToTry, parts[len(parts)-1])
			}
		}

		// Try each path
		var lastErr error
		for _, pathToTry := range pathsToTry {
			log.Printf("[RequestID: %s] ASSET-DEBUG: Trying path: %s", requestID, pathToTry)
			file, err := resolvedBucket.GetFile(
				latestUpdate.Branch,
				latestUpdate.RuntimeVersion,
				latestUpdate.UpdateId,
				pathToTry)

			if err == nil {
				log.Printf("[RequestID: %s] ASSET-DEBUG: Successfully found asset using path: %s", requestID, pathToTry)
				return file, nil
			}

			lastErr = err
			log.Printf("[RequestID: %s] ASSET-DEBUG: Failed to find asset using path %s: %v", requestID, pathToTry, err)
		}

		return nil, lastErr
	}

	// Try to get the asset
	asset, err := tryAssetPaths()
	if err != nil {
		log.Printf("[RequestID: %s] Error getting asset: %v", requestID, err)

		// Try older updates as fallback
		log.Printf("[RequestID: %s] ASSET-DEBUG: Trying older updates as fallback", requestID)
		var fallbackAsset io.ReadCloser
		var foundInFallback bool

		for i := 1; i < len(allUpdates); i++ {
			fallbackUpdate := allUpdates[i]
			log.Printf("[RequestID: %s] ASSET-DEBUG: Trying fallback update %s for asset %s",
				requestID, fallbackUpdate.UpdateId, assetPath)

			// Try all path variations in this update
			fallbackAsset, err = resolvedBucket.GetFile(
				fallbackUpdate.Branch,
				fallbackUpdate.RuntimeVersion,
				fallbackUpdate.UpdateId,
				assetPath)

			if err == nil {
				log.Printf("[RequestID: %s] ASSET-DEBUG: Found asset in fallback update %s!",
					requestID, fallbackUpdate.UpdateId)
				foundInFallback = true
				latestUpdate = fallbackUpdate
				asset = fallbackAsset
				break
			}
		}

		if !foundInFallback {
			log.Printf("[RequestID: %s] ASSET-DEBUG: Asset not found in any update", requestID)
			return AssetsResponse{StatusCode: http.StatusNotFound, Body: []byte("Asset not found in any update")}, nil, "", nil
		}
	}

	headers := map[string]string{
		"expo-protocol-version": "1",
		"expo-sfv-version":      "0",
		"Cache-Control":         "public, max-age=31536000",
		"Content-Type":          contentType,
	}

	bucketFile := &types.BucketFile{
		Reader:    asset,
		CreatedAt: time.Now(), // Since we don't have the actual creation time
	}

	return AssetsResponse{
		StatusCode:  http.StatusOK,
		Headers:     headers,
		ContentType: contentType,
	}, bucketFile, latestUpdate.UpdateId, nil
}

func HandleAssetsWithFile(req AssetsRequest) (AssetsResponse, error) {
	log.Printf("[RequestID: %s] ASSET-DEBUG: Starting asset lookup for %s (platform: %s, runtimeVersion: %s)",
		req.RequestID, req.AssetName, req.Platform, req.RuntimeVersion)

	resp, bucketFile, _, err := getAssetMetadata(req, true)
	if err != nil {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Error getting asset metadata: %v", req.RequestID, err)
		return resp, err
	}
	if resp.StatusCode != 200 {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Non-200 status code from metadata: %d", req.RequestID, resp.StatusCode)
		return AssetsResponse{
			StatusCode: resp.StatusCode,
			Body:       resp.Body,
		}, nil
	}

	if bucketFile == nil {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Resolved file is nil", req.RequestID)
		return AssetsResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte("Resolved file is nil"),
		}, nil
	}

	log.Printf("[RequestID: %s] ASSET-DEBUG: Successfully found file, reading contents", req.RequestID)
	buffer, err := io.ReadAll(bucketFile.Reader)
	defer bucketFile.Reader.Close()
	if err != nil {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Error converting asset to buffer: %v", req.RequestID, err)
		return AssetsResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte("Error converting asset to buffer"),
		}, err
	}

	log.Printf("[RequestID: %s] ASSET-DEBUG: Successfully read %d bytes from asset", req.RequestID, len(buffer))
	resp.Body = buffer
	return resp, nil
}

func HandleAssetsWithURL(req AssetsRequest, resolvedCDN cdn.CDN) (AssetsResponse, error) {
	resp, _, updateId, err := getAssetMetadata(req, false)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != 200 {
		return AssetsResponse{
			StatusCode: resp.StatusCode,
			Body:       resp.Body,
		}, nil
	}
	resp.URL, err = resolvedCDN.ComputeRedirectionURLForAsset(req.Branch, req.RuntimeVersion, updateId, req.AssetName)
	if err != nil {
		log.Printf("[RequestID: %s] Error computing redirection URL: %v", req.RequestID, err)
		return AssetsResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte("Error computing redirection URL"),
		}, err
	}
	return resp, nil
}

func getAssetMetadataForPath(branch string, runtimeVersion string, updateId string, assetPath string) (*AssetsResponse, error) {
	requestID := uuid.New().String()
	log.Printf("[RequestID: %s] Getting asset metadata for %s/%s/%s/%s", requestID, branch, runtimeVersion, updateId, assetPath)

	// Get the bucket
	bucket := bucket.GetBucket()

	// Try different path variations
	paths := []string{
		assetPath,                          // Original path
		strings.TrimPrefix(assetPath, "/"), // Without leading slash
		path.Base(assetPath),               // Just the filename
	}

	var file io.ReadCloser
	var err error

	for _, p := range paths {
		log.Printf("[RequestID: %s] Trying path: %s", requestID, p)
		file, err = bucket.GetFile(branch, runtimeVersion, updateId, p)
		if err == nil {
			log.Printf("[RequestID: %s] Found file at path: %s", requestID, p)
			break
		}
		log.Printf("[RequestID: %s] Path %s not found: %v", requestID, p, err)
	}

	if err != nil {
		log.Printf("[RequestID: %s] Asset not found in any path variation: %v", requestID, err)
		return nil, fmt.Errorf("asset not found: %v", err)
	}
	defer file.Close()

	// Get file size by reading the content
	var size int64
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := file.Read(buf)
		if n > 0 {
			size += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[RequestID: %s] Error reading file: %v", requestID, err)
			return nil, fmt.Errorf("error reading file: %v", err)
		}
	}

	// Create response
	response := &AssetsResponse{
		Path: assetPath,
		Size: size,
	}

	log.Printf("[RequestID: %s] Successfully retrieved asset metadata: %+v", requestID, response)
	return response, nil
}
