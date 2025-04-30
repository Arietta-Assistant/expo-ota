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

		// Check if this update is marked as active
		// By default, updates are considered active if not explicitly marked inactive
		if hasStateFile(update, "active") || !hasStateFile(update, "inactive") {
			update.Active = true
			activeUpdates = append(activeUpdates, update)
			log.Printf("Update %s is active", update.UpdateId)
		} else {
			update.Active = false
			inactiveUpdates = append(inactiveUpdates, update)
			log.Printf("Update %s is inactive, skipping", update.UpdateId)
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

	// Try with dot prefix first
	file, err := resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, "."+stateFileName)
	if err == nil && file != nil {
		file.Close()
		return true
	}

	// Also try without dot prefix for compatibility
	file, err = resolvedBucket.GetFile(update.Branch, update.RuntimeVersion, update.UpdateId, stateFileName)
	if err == nil && file != nil {
		file.Close()
		return true
	}

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
