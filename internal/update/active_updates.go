package update

import (
	"expo-open-ota/internal/bucket"
	"expo-open-ota/internal/cache"
	"expo-open-ota/internal/types"
	"fmt"
	"log"
	"sort"
)

// GetLatestActiveUpdateForRuntimeVersion prioritizes updates marked as active
func GetLatestActiveUpdateForRuntimeVersion(branch string, runtimeVersion string, buildNumber string) (*types.Update, error) {
	// Get all updates for this branch and runtime version
	updates, err := GetAllUpdatesForRuntimeVersion(branch, runtimeVersion)
	if err != nil {
		return nil, fmt.Errorf("error getting updates: %w", err)
	}

	log.Printf("Found %d updates for branch=%s, runtimeVersion=%s", len(updates), branch, runtimeVersion)

	// Filter out invalid updates and check for active flag
	var activeUpdates []types.Update
	var inactiveUpdates []types.Update

	for _, update := range updates {
		if !IsUpdateValid(update) {
			log.Printf("Update %s is invalid, skipping", update.UpdateId)
			continue
		}

		// Check if this update is explicitly marked as inactive
		if hasStateFile(update, "inactive") {
			update.Active = false
			inactiveUpdates = append(inactiveUpdates, update)
			log.Printf("Update %s is explicitly marked inactive, skipping", update.UpdateId)
			continue
		}

		// Either it has an active marker or no marker at all, which makes it active by default
		update.Active = true
		activeUpdates = append(activeUpdates, update)

		// If it has an explicit active marker, log it
		if hasStateFile(update, "active") {
			log.Printf("Update %s is explicitly marked active", update.UpdateId)
		} else {
			log.Printf("Update %s has no active/inactive markers, treating as active by default", update.UpdateId)
		}
	}

	// If we have no active updates, check if we should fall back to any update
	if len(activeUpdates) == 0 {
		log.Printf("No active updates found for branch=%s, runtimeVersion=%s", branch, runtimeVersion)
		if len(inactiveUpdates) > 0 {
			log.Printf("Found %d inactive updates, but all updates are marked inactive", len(inactiveUpdates))
		}
		return nil, fmt.Errorf("no active updates found")
	}

	// Sort active updates by creation time (newest first)
	sort.Slice(activeUpdates, func(i, j int) bool {
		return activeUpdates[i].CreatedAt > activeUpdates[j].CreatedAt
	})

	// If buildNumber is specified, find the highest active build that's not greater than requested
	if buildNumber != "" {
		requestedBuildNum := extractBuildNumber(buildNumber)
		if requestedBuildNum > 0 {
			// Find highest build number that doesn't exceed requested build
			var highestCompatibleUpdate *types.Update
			highestBuildNumFound := -1

			for i := range activeUpdates {
				currentBuildNum := extractBuildNumber(activeUpdates[i].UpdateId)
				if currentBuildNum > 0 && currentBuildNum <= requestedBuildNum && currentBuildNum > highestBuildNumFound {
					highestBuildNumFound = currentBuildNum
					highestCompatibleUpdate = &activeUpdates[i]
				}
			}

			if highestCompatibleUpdate != nil {
				log.Printf("Found compatible active update with build number %d for requested build %d: %s",
					highestBuildNumFound, requestedBuildNum, highestCompatibleUpdate.UpdateId)
				return highestCompatibleUpdate, nil
			}
		}
	}

	// If we get here, either no buildNumber was specified or we couldn't find a compatible update
	// Return the newest active update
	newestUpdate := &activeUpdates[0]
	log.Printf("Selected newest active update: %s", newestUpdate.UpdateId)
	return newestUpdate, nil
}

// hasStateFile checks if an update has a specific state file like "inactive" or "active"
func hasStateFile(update types.Update, stateFileName string) bool {
	resolvedBucket := bucket.GetBucket()

	// List of possible locations for the state marker file
	pathsToCheck := []string{
		// With dot prefix in root dir
		"." + stateFileName,
		// No dot prefix in root dir
		stateFileName,
		// In assets directory
		"assets/" + stateFileName,
		// In assets directory without dot
		"assets/" + stateFileName,
	}

	// Check each potential path
	log.Printf("ACTIVE-DEBUG: Checking for %s state markers for update %s in %s/%s",
		stateFileName, update.UpdateId, update.Branch, update.RuntimeVersion)

	for _, path := range pathsToCheck {
		log.Printf("ACTIVE-DEBUG: Looking for %s marker at path: %s/%s/%s/%s",
			stateFileName, update.Branch, update.RuntimeVersion, update.UpdateId, path)

		file, err := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, path)
		if err == nil && file != nil {
			file.Close()
			log.Printf("ACTIVE-DEBUG: Found %s marker at path: %s/%s/%s/%s",
				stateFileName, update.Branch, update.RuntimeVersion, update.UpdateId, path)
			return true
		} else if err != nil {
			log.Printf("ACTIVE-DEBUG: Error checking for %s marker at %s/%s/%s/%s: %v",
				stateFileName, update.Branch, update.RuntimeVersion, update.UpdateId, path, err)
		}
	}

	log.Printf("ACTIVE-DEBUG: No %s marker found for update %s", stateFileName, update.UpdateId)
	return false
}

// ActivateUpdate marks an update as active
func ActivateUpdate(branch string, runtimeVersion string, updateId string) error {
	// Use the bucket's implementation directly
	resolvedBucket := bucket.GetBucket()
	err := resolvedBucket.ActivateUpdate(branch, runtimeVersion, updateId)
	if err != nil {
		return fmt.Errorf("error activating update: %w", err)
	}

	// Invalidate cache
	invalidateUpdateCache(branch, runtimeVersion)

	log.Printf("Successfully activated update %s for branch=%s, runtimeVersion=%s",
		updateId, branch, runtimeVersion)
	return nil
}

// DeactivateUpdate marks an update as inactive
func DeactivateUpdate(branch string, runtimeVersion string, updateId string) error {
	// Use the bucket's implementation directly
	resolvedBucket := bucket.GetBucket()
	err := resolvedBucket.DeactivateUpdate(branch, runtimeVersion, updateId)
	if err != nil {
		return fmt.Errorf("error deactivating update: %w", err)
	}

	// Invalidate cache
	invalidateUpdateCache(branch, runtimeVersion)

	log.Printf("Successfully deactivated update %s for branch=%s, runtimeVersion=%s",
		updateId, branch, runtimeVersion)
	return nil
}

// invalidateUpdateCache clears any cached data for the branch and runtime version
func invalidateUpdateCache(branch string, runtimeVersion string) {
	cacheInstance := cache.GetCache()
	cacheKey := ComputeLastUpdateCacheKey(branch, runtimeVersion)
	cacheInstance.Delete(cacheKey)
}
