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

	lastUpdate, err := update.GetLatestUpdateBundlePathForRuntimeVersion(req.Branch, req.RuntimeVersion, "")
	if err != nil || lastUpdate == nil {
		log.Printf("[RequestID: %s] No update found for runtimeVersion: %s", requestID, req.RuntimeVersion)
		return AssetsResponse{StatusCode: http.StatusNotFound, Body: []byte("No update found")}, nil, "", nil
	}

	log.Printf("[RequestID: %s] ASSET-DEBUG: Using update ID %s for asset request (branch=%s, runtimeVersion=%s)",
		requestID, lastUpdate.UpdateId, req.Branch, req.RuntimeVersion)

	if !returnAsset {
		headers := map[string]string{
			"expo-protocol-version": "1",
			"expo-sfv-version":      "0",
			"Cache-Control":         "public, max-age=31536000",
		}
		return AssetsResponse{
			StatusCode: http.StatusOK,
			Headers:    headers,
		}, nil, lastUpdate.UpdateId, nil
	}

	metadata, err := update.GetMetadata(*lastUpdate)
	if err != nil {
		log.Printf("[RequestID: %s] Error getting metadata: %v", requestID, err)
		return AssetsResponse{StatusCode: http.StatusInternalServerError, Body: []byte("Error getting metadata")}, nil, "", nil
	}

	actualPlatform := req.Platform
	if req.Platform == "all" {
		log.Printf("[RequestID: %s] Platform 'all' specified in asset request, using 'ios'", requestID)
		actualPlatform = "ios"
	}

	var platformMetadata types.PlatformMetadata
	switch actualPlatform {
	case "android":
		platformMetadata = metadata.MetadataJSON.FileMetadata.Android
	case "ios":
		platformMetadata = metadata.MetadataJSON.FileMetadata.IOS
	default:
		return AssetsResponse{StatusCode: http.StatusBadRequest, Body: []byte("Platform not supported")}, nil, "", nil
	}

	bundle := platformMetadata.Bundle
	isLaunchAsset := bundle == req.AssetName

	var assetMetadata types.Asset
	for _, asset := range platformMetadata.Assets {
		if asset.Path == req.AssetName {
			assetMetadata = asset
		}
	}

	resolvedBucket := bucket.GetBucket()
	log.Printf("[RequestID: %s] ASSET-DEBUG: Looking for asset %s in update %s/%s/%s (bucket type: %T)",
		requestID, req.AssetName, lastUpdate.Branch, lastUpdate.RuntimeVersion, lastUpdate.UpdateId, resolvedBucket)
	asset, err := resolvedBucket.GetFile(lastUpdate.Branch, lastUpdate.RuntimeVersion, lastUpdate.UpdateId, req.AssetName)
	if err != nil {
		log.Printf("[RequestID: %s] Error getting asset: %v", requestID, err)

		// Try other potential update directories by getting all updates for this runtime version
		allUpdates, listErr := update.GetAllUpdatesForRuntimeVersion(req.Branch, req.RuntimeVersion)
		if listErr == nil && len(allUpdates) > 0 {
			log.Printf("[RequestID: %s] ASSET-DEBUG: Found %d other updates for runtime %s, attempting fallbacks",
				requestID, len(allUpdates), req.RuntimeVersion)

			// Try each update directory
			for _, otherUpdate := range allUpdates {
				if otherUpdate.UpdateId == lastUpdate.UpdateId {
					continue // Skip the one we already tried
				}

				log.Printf("[RequestID: %s] ASSET-DEBUG: Trying fallback update %s for asset %s",
					requestID, otherUpdate.UpdateId, req.AssetName)
				fallbackAsset, fallbackErr := resolvedBucket.GetFile(otherUpdate.Branch, otherUpdate.RuntimeVersion, otherUpdate.UpdateId, req.AssetName)
				if fallbackErr == nil {
					log.Printf("[RequestID: %s] ASSET-DEBUG: Found asset in fallback update %s!",
						requestID, otherUpdate.UpdateId)
					asset = fallbackAsset
					err = nil
					break
				}
			}
		}

		if err != nil {
			return AssetsResponse{StatusCode: http.StatusInternalServerError, Body: []byte("Error getting asset")}, nil, "", nil
		}
	}

	var contentType string
	if isLaunchAsset {
		contentType = "application/javascript"
	} else {
		contentType = mime.TypeByExtension("." + string(assetMetadata.Ext))
	}

	headers := map[string]string{
		"expo-protocol-version": "1",
		"expo-sfv-version":      "0",
		"Cache-Control":         "public, max-age=31536000",
		"Content-Type":          contentType,
	}

	bucketFile := &types.BucketFile{
		Reader:    asset,
		CreatedAt: time.Now(), // Since we don't have the actual creation time from Firebase Storage
	}

	return AssetsResponse{
		StatusCode:  http.StatusOK,
		Headers:     headers,
		ContentType: contentType,
	}, bucketFile, lastUpdate.UpdateId, nil
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
