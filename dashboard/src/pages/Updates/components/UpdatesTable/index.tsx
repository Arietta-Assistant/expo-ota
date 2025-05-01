import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api.ts';
import { ApiError } from '@/components/APIError';
import { GitBranch, Milestone, Rss, Calendar, Hash, Smartphone, ChevronDown, Tag } from 'lucide-react';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Badge } from '@/components/ui/badge.tsx';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { useState, useEffect } from 'react';
import AppleIcon from '@/assets/apple.svg';
import AndroidIcon from '@/assets/android.svg';

// Define types for the update data
interface Update {
  updateUUID: string;
  createdAt: string;
  updateId: string;
  platform?: string;
  commitHash?: string;
}

export const UpdatesTable = ({
  branch,
  runtimeVersion,
}: {
  branch: string;
  runtimeVersion: string;
}) => {
  const [visibleCount, setVisibleCount] = useState(4);
  
  const { data, isLoading, error } = useQuery({
    queryKey: ['updates'],
    queryFn: () => api.getUpdates(branch, runtimeVersion),
  });

  // Log date values for debugging
  useEffect(() => {
    if (data && data.length > 0) {
      console.log('Date values from API:', data.map(item => ({
        updateId: item.updateId,
        rawDate: item.createdAt
      })));
    }
  }, [data]);

  const handleLoadMore = () => {
    setVisibleCount(prev => prev + 4);
  };
  
  const visibleUpdates = data ? data.slice(0, visibleCount) : [];
  const hasMore = data ? visibleCount < data.length : false;

  // Format date in a robust way that handles various formats
  const formatDate = (dateString: string) => {
    try {
      // Handle extremely large numbers (like 6480000000000000)
      if (/^\d{15,}$/.test(dateString)) {
        // These appear to be malformatted dates, possibly using a non-standard format
        // For now, show a more readable error and the raw value for debugging
        return `Unknown format (${dateString})`;
      }
      
      // Try to parse the date string using Date constructor
      const date = new Date(dateString);
      
      // Check if date is valid
      if (!isNaN(date.getTime())) {
        // Check if year is unreasonable (like 207313)
        if (date.getFullYear() > 3000) {
          // This is likely a timestamp in milliseconds since Unix epoch
          // Try to interpret as seconds instead
          const secondsDate = new Date(parseInt(dateString, 10) * 1000);
          if (!isNaN(secondsDate.getTime()) && secondsDate.getFullYear() < 3000) {
            return secondsDate.toLocaleDateString('en-GB', {
              year: 'numeric',
              month: 'long',
              day: 'numeric',
              hour: '2-digit',
              minute: '2-digit',
            });
          }
          
          // If still invalid, try to use just the date component to debug
          return `Invalid date format: ${dateString}`;
        }
        
        return date.toLocaleDateString('en-GB', {
          year: 'numeric',
          month: 'long',
          day: 'numeric',
          hour: '2-digit',
          minute: '2-digit',
        });
      }
      
      // If it's just a number, it might be a Unix timestamp
      if (/^\d+$/.test(dateString)) {
        const timestampDate = new Date(parseInt(dateString, 10) * 1000);
        if (!isNaN(timestampDate.getTime())) {
          return timestampDate.toLocaleDateString('en-GB', {
            year: 'numeric',
            month: 'long',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
          });
        }
      }
      
      return `Invalid date: ${dateString}`;
    } catch (error) {
      console.error("Date parsing error:", error);
      return `Error parsing date: ${dateString}`;
    }
  };

  // Determine platform(s) from update ID and platform field
  const getPlatformInfo = (update: Update): string[] => {
    const platforms: string[] = [];
    
    // Check explicit platform field
    if (update.platform) {
      const platform = update.platform.toLowerCase();
      if (platform === 'ios') {
        platforms.push('ios');
      }
      if (platform === 'android') {
        platforms.push('android');
      }
    }
    
    // Check update ID for platform hints if no platform explicitly specified
    if (platforms.length === 0 && update.updateId) {
      const updateId = update.updateId.toLowerCase();
      if (updateId.includes('ios')) {
        platforms.push('ios');
      }
      if (updateId.includes('android')) {
        platforms.push('android');
      }
    }
    
    return platforms;
  };

  // Extract build number from update ID
  const extractBuildInfo = (updateId: string) => {
    const buildMatch = updateId.match(/build-(\d+)/i);
    if (buildMatch && buildMatch[1]) {
      return {
        buildNumber: buildMatch[1],
        displayName: `Build ${buildMatch[1]}`
      };
    }
    return {
      buildNumber: null,
      displayName: 'Unknown Build'
    };
  };

  return (
    <div className="w-full flex-1">
      <Breadcrumb className="mb-4">
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink href="/dashboard" className="flex items-center gap-2 underline">
              <GitBranch className="w-4" />
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{branch}</BreadcrumbPage>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbLink
              href={`/dashboard?branch=${branch}`}
              className="flex items-center gap-2 underline">
              <Milestone className="w-4" />
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{runtimeVersion}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      
      {!!error && <ApiError error={error} />}
      
      {isLoading ? (
        <div className="flex justify-center items-center min-h-[200px]">
          <div className="animate-pulse text-muted-foreground">Loading updates...</div>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
            {visibleUpdates.map((update) => {
              const { displayName } = extractBuildInfo(update.updateId || '');
              const platforms = getPlatformInfo(update);
              
              return (
                <Card key={update.updateId} className="overflow-hidden">
                  <CardHeader className="p-4 pb-2 bg-muted/20">
                    <CardTitle className="flex items-center gap-2 text-base">
                      <Tag className="w-4 h-4" />
                      <span className="truncate font-bold">{displayName}</span>
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="p-4 pt-2 space-y-3">
                    <div className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-1 text-sm">
                      <Smartphone className="w-4 h-4 text-muted-foreground" />
                      <div className="flex items-center gap-1 flex-wrap">
                        {platforms.includes('ios') && (
                          <>
                            <img src={AppleIcon} alt="iOS" className="w-4 h-4" />
                            <Badge variant="outline" className="text-xs">iOS</Badge>
                          </>
                        )}
                        {platforms.includes('android') && (
                          <>
                            <img src={AndroidIcon} alt="Android" className="w-4 h-4" />
                            <Badge variant="outline" className="text-xs">Android</Badge>
                          </>
                        )}
                        {platforms.length === 0 && (
                          // If no platform is detected, assume it might be a universal build
                          <>
                            <img src={AppleIcon} alt="iOS" className="w-4 h-4 opacity-50" />
                            <img src={AndroidIcon} alt="Android" className="w-4 h-4 opacity-50" />
                            <Badge variant="outline" className="text-xs">Universal</Badge>
                          </>
                        )}
                      </div>
                      
                      <Rss className="w-4 h-4 text-muted-foreground" />
                      <div className="text-xs text-muted-foreground truncate">
                        <span title={update.updateId}>{update.updateId || 'Unknown'}</span>
                      </div>
                      
                      <Hash className="w-4 h-4 text-muted-foreground" />
                      <div>
                        {update.commitHash ? (
                          <Badge variant="secondary" className="text-xs">
                            {typeof update.commitHash === 'string' ? update.commitHash.slice(0, 7) : update.commitHash}
                          </Badge>
                        ) : (
                          <Badge variant="secondary" className="text-xs">N/A</Badge>
                        )}
                      </div>
                      
                      <Calendar className="w-4 h-4 text-muted-foreground" />
                      <div className="truncate">
                        <span className="text-xs text-muted-foreground">
                          {update.createdAt ? 
                            // Special handling for the known problematic formats
                            /^\d{15,}$/.test(update.createdAt) ? 
                              "Not available" : 
                              formatDate(update.createdAt) 
                            : 'Unknown'}
                        </span>
                      </div>
                      
                      <div className="col-span-2 mt-1 text-xs text-muted-foreground overflow-hidden text-ellipsis">
                        <span className="font-medium">UUID:</span> {update.updateUUID || 'N/A'}
                      </div>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
          
          {hasMore && (
            <div className="flex justify-center mt-4 mb-8">
              <Button 
                variant="outline" 
                onClick={handleLoadMore}
                className="flex items-center gap-2"
              >
                Load More <ChevronDown className="w-4 h-4" />
              </Button>
            </div>
          )}
          
          {visibleUpdates.length === 0 && !isLoading && (
            <div className="text-center py-10 text-muted-foreground">
              No updates found for this runtime version.
            </div>
          )}
        </>
      )}
    </div>
  );
};
