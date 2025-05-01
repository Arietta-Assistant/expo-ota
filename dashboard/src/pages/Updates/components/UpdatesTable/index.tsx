import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api.ts';
import { ApiError } from '@/components/APIError';
import { GitBranch, Milestone, Rss, Calendar, Hash, Smartphone, ChevronDown } from 'lucide-react';
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
import { useState } from 'react';
import AppleIcon from '@/assets/apple.svg';
import AndroidIcon from '@/assets/android.svg';

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

  const handleLoadMore = () => {
    setVisibleCount(prev => prev + 4);
  };
  
  const visibleUpdates = data ? data.slice(0, visibleCount) : [];
  const hasMore = data ? visibleCount < data.length : false;

  const formatDate = (dateString: string) => {
    try {
      const date = new Date(dateString);
      if (isNaN(date.getTime())) {
        return 'Invalid date';
      }
      
      return date.toLocaleDateString('en-GB', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
        hour: 'numeric',
        minute: 'numeric',
      });
    } catch {
      return 'Invalid date';
    }
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
            {visibleUpdates.map((update) => (
              <Card key={update.updateId} className="overflow-hidden">
                <CardHeader className="p-4 pb-2 bg-muted/20">
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Rss className="w-4 h-4" />
                    <span className="truncate">{update.updateId || 'Unknown'}</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="p-4 pt-2 space-y-3">
                  <div className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-1 text-sm">
                    <Smartphone className="w-4 h-4 text-muted-foreground" />
                    <div className="flex items-center gap-1">
                      {update.platform === 'ios' && (
                        <>
                          <img src={AppleIcon} alt="iOS" className="w-4 h-4" />
                          <Badge variant="outline" className="text-xs">iOS</Badge>
                        </>
                      )}
                      {update.platform === 'android' && (
                        <>
                          <img src={AndroidIcon} alt="Android" className="w-4 h-4" />
                          <Badge variant="outline" className="text-xs">Android</Badge>
                        </>
                      )}
                      {!update.platform && (
                        <Badge variant="outline" className="text-xs">Unknown</Badge>
                      )}
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
                        {update.createdAt ? formatDate(update.createdAt) : 'Unknown'}
                      </span>
                    </div>
                    
                    <div className="col-span-2 mt-1 text-xs text-muted-foreground overflow-hidden text-ellipsis">
                      <span className="font-medium">UUID:</span> {update.updateUUID || 'N/A'}
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
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
