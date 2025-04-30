import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api.ts';
import { ApiError } from '@/components/APIError';
import { DataTable } from '@/components/DataTable';
import { GitBranch, Milestone, Rss, Trash, PowerOff, Power, Download, Users } from 'lucide-react';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Badge } from '@/components/ui/badge.tsx';
import { Button } from '@/components/ui/button';
import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from '@/components/ui/dialog';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';

export const UpdatesTable = ({
  branch,
  runtimeVersion,
}: {
  branch: string;
  runtimeVersion: string;
}) => {
  const queryClient = useQueryClient();
  const [selectedUpdateId, setSelectedUpdateId] = useState<string | null>(null);
  const [showStatsDialog, setShowStatsDialog] = useState(false);

  const { data, isLoading, error } = useQuery({
    queryKey: ['updates', branch, runtimeVersion],
    queryFn: () => api.getUpdates(branch, runtimeVersion),
  });

  const { data: updateStats, isLoading: isLoadingStats } = useQuery({
    queryKey: ['updateStats', branch, runtimeVersion, selectedUpdateId],
    queryFn: () => selectedUpdateId ? api.getUpdateStats(branch, runtimeVersion, selectedUpdateId) : null,
    enabled: !!selectedUpdateId && showStatsDialog,
  });

  const deleteMutation = useMutation({
    mutationFn: (updateId: string) => api.deleteUpdate(branch, runtimeVersion, updateId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', branch, runtimeVersion] });
      alert('Update deleted successfully');
    },
    onError: (error) => {
      alert(`Failed to delete update: ${error instanceof Error ? error.message : 'Unknown error'}`);
    },
  });

  const activateMutation = useMutation({
    mutationFn: (updateId: string) => api.activateUpdate(branch, runtimeVersion, updateId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', branch, runtimeVersion] });
      alert('Update activated successfully');
    },
    onError: (error) => {
      alert(`Failed to activate update: ${error instanceof Error ? error.message : 'Unknown error'}`);
    },
  });

  const deactivateMutation = useMutation({
    mutationFn: (updateId: string) => api.deactivateUpdate(branch, runtimeVersion, updateId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates', branch, runtimeVersion] });
      alert('Update deactivated successfully');
    },
    onError: (error) => {
      alert(`Failed to deactivate update: ${error instanceof Error ? error.message : 'Unknown error'}`);
    },
  });

  const handleViewStats = (updateId: string) => {
    setSelectedUpdateId(updateId);
    setShowStatsDialog(true);
  };

  return (
    <div className="w-full flex-1">
      <Breadcrumb className="mb-2">
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
      <DataTable
        loading={isLoading}
        columns={[
          {
            header: 'ID',
            accessorKey: 'updateId',
            cell: value => {
              const updateId = value.row.original?.updateId || 'Unknown';
              return (
                <span className="flex flex-row gap-2 items-center w-full">
                  <Rss className="w-4" />
                  {updateId}
                </span>
              );
            },
          },
          {
            header: 'UUID',
            accessorKey: 'updateUUID',
            cell: value => {
              return value.row.original?.updateUUID || 'N/A';
            },
          },
          {
            header: 'Platform',
            accessorKey: 'platform',
            cell: value => {
              const platform = value.row.original?.platform || '';
              const isIos = platform === 'ios';
              const isAndroid = platform === 'android';
              return (
                <div className="flex flex-row items-center gap-2">
                  {isIos && <img src="@/assets/apple.svg" className="w-4" alt="apple" />}
                  {isAndroid && <img src="@/assets/android.svg" className="w-4" alt="android" />}
                  {!isIos && !isAndroid && <span>Unknown</span>}
                </div>
              );
            },
          },
          {
            header: 'Commit',
            accessorKey: 'commitHash',
            cell: value => {
              const commitHash = value.row.original?.commitHash;
              if (!commitHash) {
                return <Badge variant="secondary" className="text-xs">N/A</Badge>;
              }
              return (
                <Badge variant="secondary" className="text-xs">
                  {typeof commitHash === 'string' ? commitHash.slice(0, 7) : commitHash}
                </Badge>
              );
            },
          },
          {
            header: 'Published at',
            accessorKey: 'createdAt',
            cell: ({ row }) => {
              const createdAt = row.original?.createdAt;
              if (!createdAt) {
                return <Badge variant="outline">Unknown</Badge>;
              }
              
              try {
                const date = new Date(createdAt);
                if (isNaN(date.getTime())) {
                  return <Badge variant="outline">Invalid date</Badge>;
                }
                
                return (
                  <Badge variant="outline">
                    {date.toLocaleDateString('en-GB', {
                      year: 'numeric',
                      month: 'long',
                      day: 'numeric',
                      hour: 'numeric',
                      minute: 'numeric',
                      second: 'numeric',
                    })}
                  </Badge>
                );
              } catch {
                return <Badge variant="outline">Invalid date</Badge>;
              }
            },
          },
          {
            header: 'Status',
            accessorKey: 'active',
            cell: ({ row }) => {
              const active = row.original?.active !== false; // Default to active if not specified
              return (
                <Badge variant={active ? "default" : "secondary"} className={active ? "bg-green-500" : ""}>
                  {active ? "Active" : "Inactive"}
                </Badge>
              );
            },
          },
          {
            header: 'Downloads',
            accessorKey: 'downloadCount',
            cell: ({ row }) => {
              const downloadCount = row.original?.downloadCount || 0;
              return (
                <div className="flex items-center gap-2">
                  <Download className="w-4 h-4" />
                  <span>{downloadCount}</span>
                </div>
              );
            },
          },
          {
            header: 'Actions',
            cell: ({ row }) => {
              const updateId = row.original?.updateId;
              const active = row.original?.active !== false;
              
              if (!updateId) return null;
              
              return (
                <div className="flex space-x-2">
                  <Button 
                    variant="outline" 
                    size="sm" 
                    onClick={() => handleViewStats(updateId)}
                  >
                    <Users className="w-4 h-4 mr-1" />
                    Users
                  </Button>
                  
                  {active ? (
                    <Button 
                      variant="outline" 
                      size="sm"
                      onClick={() => deactivateMutation.mutate(updateId)}
                    >
                      <PowerOff className="w-4 h-4 mr-1" />
                      Deactivate
                    </Button>
                  ) : (
                    <Button 
                      variant="outline" 
                      size="sm"
                      onClick={() => activateMutation.mutate(updateId)}
                    >
                      <Power className="w-4 h-4 mr-1" />
                      Activate
                    </Button>
                  )}
                  
                  <Button 
                    variant="destructive" 
                    size="sm"
                    onClick={() => {
                      if (confirm("Are you sure you want to delete this update?")) {
                        deleteMutation.mutate(updateId);
                      }
                    }}
                  >
                    <Trash className="w-4 h-4 mr-1" />
                    Delete
                  </Button>
                </div>
              );
            },
          },
        ]}
        data={data ?? []}
      />
      
      <Dialog open={showStatsDialog} onOpenChange={setShowStatsDialog}>
        <DialogContent className="sm:max-w-[600px]">
          <DialogHeader>
            <DialogTitle>Update Stats - {selectedUpdateId}</DialogTitle>
            <DialogDescription>
              Users who have downloaded this update
            </DialogDescription>
          </DialogHeader>
          
          {isLoadingStats ? (
            <div className="py-4 text-center">Loading stats...</div>
          ) : (
            <>
              <div className="mb-4">
                <h4 className="text-sm font-medium">Total Downloads: {updateStats?.downloadCount || 0}</h4>
              </div>
              
              {updateStats?.users && updateStats.users.length > 0 ? (
                <div className="max-h-[400px] overflow-y-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>User ID</TableHead>
                        <TableHead>Device ID</TableHead>
                        <TableHead>Last Downloaded</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {updateStats.users.map((user, index) => (
                        <TableRow key={index}>
                          <TableCell>{user.userId}</TableCell>
                          <TableCell>{user.deviceId}</TableCell>
                          <TableCell>{new Date(user.lastDownloadedAt).toLocaleString()}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              ) : (
                <div className="py-4 text-center text-gray-500">No user data available</div>
              )}
            </>
          )}
          
          <DialogClose asChild>
            <Button variant="outline">Close</Button>
          </DialogClose>
        </DialogContent>
      </Dialog>
    </div>
  );
};
