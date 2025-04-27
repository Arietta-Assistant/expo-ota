package assets

import (
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/cdn"
	"expo-open-ota/internal/types"
	"expo-open-ota/internal/update"
	"io"
	"log"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
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
		}, nil, allUpdates[0].UpdateId, nil
	}

	resolvedBucket := bucket.GetBucket()

	// Try all updates, starting with the most recent one
	var asset io.ReadCloser
	var successfulUpdateId string
	var assetMetadata types.Asset
	var platformMetadata types.PlatformMetadata
	var contentType string
	var isLaunchAsset bool

	// Try each update, starting with the most recent
	for _, currentUpdate := range allUpdates {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Trying update ID %s for asset %s (branch=%s, runtimeVersion=%s)",
			requestID, currentUpdate.UpdateId, req.AssetName, req.Branch, req.RuntimeVersion)

		// Try to get the metadata for this update
		metadata, metadataErr := update.GetMetadata(currentUpdate)
		if metadataErr != nil {
			log.Printf("[RequestID: %s] Error getting metadata for update %s: %v",
				requestID, currentUpdate.UpdateId, metadataErr)
			continue
		}

		// Determine which platform to use
		actualPlatform := req.Platform
		if req.Platform == "all" {
			log.Printf("[RequestID: %s] Platform 'all' specified in asset request, using 'ios'", requestID)
			actualPlatform = "ios"
		}

		// Get platform-specific metadata
		switch actualPlatform {
		case "android":
			platformMetadata = metadata.MetadataJSON.FileMetadata.Android
		case "ios":
			platformMetadata = metadata.MetadataJSON.FileMetadata.IOS
		default:
			continue
		}

		// Check if this is the bundle (main JavaScript file)
		bundle := platformMetadata.Bundle
		isLaunchAsset = bundle == req.AssetName

		// Look for the requested asset in the assets list
		foundAsset := false
		for _, asset := range platformMetadata.Assets {
			if asset.Path == req.AssetName {
				assetMetadata = asset
				foundAsset = true
				break
			}
		}

		// Set the content type based on asset type
		if isLaunchAsset {
			contentType = "application/javascript"
		} else if foundAsset {
			contentType = mime.TypeByExtension("." + string(assetMetadata.Ext))
		}

		// Try to get the actual asset file
		fullPath := currentUpdate.Branch + "/" + currentUpdate.RuntimeVersion + "/" + currentUpdate.UpdateId + "/" + req.AssetName
		log.Printf("[RequestID: %s] ASSET-DEBUG: Trying to get asset at path: %s", requestID, fullPath)

		assetFile, assetErr := resolvedBucket.GetFile(
			currentUpdate.Branch,
			currentUpdate.RuntimeVersion,
			currentUpdate.UpdateId,
			req.AssetName)

		if assetErr == nil {
			// Found the asset!
			asset = assetFile
			successfulUpdateId = currentUpdate.UpdateId
			log.Printf("[RequestID: %s] ASSET-DEBUG: Successfully found asset in update %s!",
				requestID, currentUpdate.UpdateId)
			break
		} else {
			log.Printf("[RequestID: %s] ASSET-DEBUG: Asset not found in update %s: %v",
				requestID, currentUpdate.UpdateId, assetErr)
		}
	}

	// If we couldn't find the asset in any update
	if asset == nil {
		log.Printf("[RequestID: %s] ASSET-DEBUG: Asset not found in any update: %s", requestID, req.AssetName)
		return AssetsResponse{StatusCode: http.StatusNotFound, Body: []byte("Asset not found")}, nil, "", nil
	}

	// Asset found - return it with the appropriate headers
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
	}, bucketFile, successfulUpdateId, nil
}

func HandleAssetsWithFile(req AssetsRequest) (AssetsResponse, error) {
	resp, bucketFile, _, err := getAssetMetadata(req, true)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != 200 {
		return AssetsResponse{
			StatusCode: resp.StatusCode,
			Body:       resp.Body,
		}, nil
	}

	if bucketFile == nil {
		log.Printf("[RequestID: %s] Resolved file is nil", req.RequestID)
		return AssetsResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte("Resolved file is nil"),
		}, nil
	}

	buffer, err := io.ReadAll(bucketFile.Reader)
	defer bucketFile.Reader.Close()
	if err != nil {
		log.Printf("[RequestID: %s] Error converting asset to buffer: %v", req.RequestID, err)
		return AssetsResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte("Error converting asset to buffer"),
		}, err
	}

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
